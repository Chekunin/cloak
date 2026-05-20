// Package endpoints implements the Endpoint Manager (Section 3.3).
//
// The manager owns the lifecycle of every local listener. On vault unlock it
// auto-opens persistent endpoints; on vault lock it closes everything and
// zeros transient secret material.
package endpoints

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/Chekunin/cloak/internal/adapters"
	"github.com/Chekunin/cloak/internal/audit"
	"github.com/Chekunin/cloak/internal/errs"
	"github.com/Chekunin/cloak/internal/secrets"
	"github.com/Chekunin/cloak/internal/store"
	"github.com/Chekunin/cloak/internal/vault"
)

// Manager is the endpoint manager.
type Manager struct {
	mu        sync.RWMutex
	active    map[string]*ActiveEndpoint // keyed by endpoint id
	bySecret  map[string]string          // secretID → endpointID
	openLocks map[string]*sync.Mutex     // per-secret serialization for Open

	// closeGen is bumped on every CloseAll. Open captures it before doing the
	// slow listener+credential build and re-checks it before publishing the
	// new endpoint — that, combined with a vault.State() check held under
	// mu, prevents a listener leak if CloseAll runs concurrently.
	closeGen uint64

	registry         *adapters.Registry
	vault            *vault.Manager
	store            *store.Store
	audit            *audit.Logger
	defaultPortStart int
	runDir           string
}

// NewManager constructs a Manager. defaultPortStart is the lowest port used
// for persistent endpoints whose secret config does not specify a port.
// runDir is the directory under which materialized endpoints render files.
func NewManager(reg *adapters.Registry, v *vault.Manager, s *store.Store, a *audit.Logger, defaultPortStart int, runDir string) *Manager {
	return &Manager{
		active:           map[string]*ActiveEndpoint{},
		bySecret:         map[string]string{},
		openLocks:        map[string]*sync.Mutex{},
		registry:         reg,
		vault:            v,
		store:            s,
		audit:            a,
		defaultPortStart: defaultPortStart,
		runDir:           runDir,
	}
}

// OpenAllPersistent opens every persistent-mode secret. Logs errors on the
// audit log but continues — one bad secret should not block the rest.
func (m *Manager) OpenAllPersistent(ctx context.Context) error {
	recs, err := m.store.ListSecrets()
	if err != nil {
		return err
	}
	for _, r := range recs {
		if r.EndpointConfig.Mode != store.ModePersistent {
			continue
		}
		if _, err := m.Open(ctx, r.ID, 0); err != nil {
			_ = m.audit.Write(audit.Entry{
				Event:      audit.EventEndpointClosed,
				SecretID:   r.ID,
				SecretName: r.Name,
				Details:    map[string]any{"error": err.Error(), "phase": "auto_open"},
			})
		}
	}
	return nil
}

// CloseAll closes every open endpoint, releasing listener resources and
// scrubbing ephemeral local credentials.
func (m *Manager) CloseAll(reason string) {
	m.mu.Lock()
	m.closeGen++
	eps := make([]*ActiveEndpoint, 0, len(m.active))
	for _, e := range m.active {
		eps = append(eps, e)
	}
	m.active = map[string]*ActiveEndpoint{}
	m.bySecret = map[string]string{}
	m.mu.Unlock()
	for _, e := range eps {
		m.shutdownEndpoint(e, reason)
	}
}

// Open opens an endpoint for the given secret. ttlOverrideSeconds, if > 0,
// overrides the secret's configured session TTL (ignored for persistent mode).
//
// Concurrent Opens for the same secret are serialised by a per-secret mutex;
// the second caller sees the first call's endpoint via the idempotent path
// and reuses it.
func (m *Manager) Open(ctx context.Context, secretIDOrName string, ttlOverrideSeconds int) (EndpointSnapshot, error) {
	rec, err := m.store.GetSecret(secretIDOrName)
	if err != nil {
		return EndpointSnapshot{}, err
	}

	// Capture the close generation now; if CloseAll happens before we insert,
	// we discard the listener we just built rather than leaking it.
	openMu := m.acquireOpenLock(rec.ID)
	openMu.Lock()
	defer openMu.Unlock()

	// Idempotency: if the secret already has an open endpoint, return it.
	m.mu.RLock()
	if existing, ok := m.bySecret[rec.ID]; ok {
		ep := m.active[existing]
		m.mu.RUnlock()
		if ep != nil {
			return ep.Snapshot(), nil
		}
	} else {
		m.mu.RUnlock()
	}

	adapter, err := m.registry.Get(rec.Type)
	if err != nil {
		return EndpointSnapshot{}, errs.Wrap(errs.CodeAdapterError, err)
	}

	startGen := m.snapshotCloseGen()

	if adapter.Kind() == adapters.KindMaterialized {
		return m.openMaterialized(ctx, rec, adapter, ttlOverrideSeconds, startGen)
	}
	proxy, ok := adapter.(adapters.ProxyAdapter)
	if !ok {
		return EndpointSnapshot{}, errs.New(errs.CodeAdapterError, "adapter does not support proxying")
	}

	port := 0
	if rec.EndpointConfig.Mode == store.ModePersistent {
		port = rec.EndpointConfig.PersistentPort
		if port == 0 {
			port = m.allocatePort()
		}
	}
	ln, actualPort, err := listenLocal(port)
	if err != nil {
		return EndpointSnapshot{}, errs.Wrap(errs.CodeEndpointError, err)
	}

	creds, err := generateLocalCreds(rec.EndpointConfig.RequireLocalAuth)
	if err != nil {
		_ = ln.Close()
		return EndpointSnapshot{}, errs.Wrap(errs.CodeInternalError, err)
	}

	endpointID := ulid.Make().String()
	openedAt := time.Now().UTC()
	var expiresAt time.Time
	if rec.EndpointConfig.Mode == store.ModeSession {
		ttl := rec.EndpointConfig.SessionTTLSeconds
		if ttlOverrideSeconds > 0 {
			ttl = ttlOverrideSeconds
		}
		expiresAt = openedAt.Add(time.Duration(ttl) * time.Second)
	}

	ctx, cancel := context.WithCancel(ctx)
	ep := &ActiveEndpoint{
		EndpointID:  endpointID,
		SecretID:    rec.ID,
		SecretName:  rec.Name,
		Type:        rec.Type,
		Kind:        EndpointListener,
		Mode:        rec.EndpointConfig.Mode,
		LocalAddr:   ln.Addr().String(),
		LocalCreds:  creds,
		OpenedAt:    openedAt,
		expiresAt:   expiresAt,
		expiryReset: make(chan struct{}, 1),
		Stats:       &EndpointStats{},
		listener:    ln,
		ctx:         ctx,
		cancel:      cancel,
	}

	dec := adapters.DecryptedSecret{
		ID:     rec.ID,
		Name:   rec.Name,
		Type:   rec.Type,
		Config: rec.Config,
	}
	aCreds := toAdapterCreds(creds)
	ep.ConnectionString = proxy.ConnectionString(ep.LocalAddr, dec, aCreds)
	envPrefix := normalizeEnvPrefix(rec.Name)
	ep.EnvVars = proxy.EnvVars(ep.LocalAddr, dec, aCreds, envPrefix)

	// Atomic insert under write lock. Two failure modes we must avoid:
	//  1. A CloseAll fired after we captured startGen and cleared `active`.
	//     If we still insert, the listener leaks.
	//  2. The vault locked entirely (and the CloseAll hook has already drained
	//     the map). Same leak. Checking vault.State() under em.mu is safe
	//     because the lock-hook contends for em.mu before clearing.
	m.mu.Lock()
	if m.snapshotCloseGenLocked() != startGen || m.vault.State() != vault.StateUnlocked {
		m.mu.Unlock()
		ep.cancel()
		_ = ln.Close()
		creds.Zero()
		return EndpointSnapshot{}, errs.New(errs.CodeVaultLocked, "vault was locked during Open")
	}
	// Re-check idempotency in case another goroutine raced through with a
	// different per-secret openLock instance (shouldn't happen, but defensive).
	if existing, ok := m.bySecret[rec.ID]; ok {
		ep2 := m.active[existing]
		m.mu.Unlock()
		ep.cancel()
		_ = ln.Close()
		creds.Zero()
		if ep2 != nil {
			return ep2.Snapshot(), nil
		}
		return EndpointSnapshot{}, errs.New(errs.CodeEndpointError, "concurrent open race")
	}
	m.active[endpointID] = ep
	m.bySecret[rec.ID] = endpointID
	m.mu.Unlock()

	go m.acceptLoop(ep, proxy, rec)
	if !expiresAt.IsZero() {
		go m.sessionExpiryWatcher(ep)
	}

	_ = m.audit.Write(audit.Entry{
		Event:      audit.EventEndpointOpened,
		SecretID:   rec.ID,
		SecretName: rec.Name,
		EndpointID: endpointID,
		Details: map[string]any{
			"mode":       string(rec.EndpointConfig.Mode),
			"local_addr": ep.LocalAddr,
			"port":       actualPort,
		},
	})
	return ep.Snapshot(), nil
}

// Close closes an endpoint by id (or by secret id/name fallback).
func (m *Manager) Close(endpointIDOrSecret string) error {
	m.mu.Lock()
	ep, ok := m.active[endpointIDOrSecret]
	if !ok {
		if id, ok2 := m.bySecret[endpointIDOrSecret]; ok2 {
			ep = m.active[id]
		}
	}
	if ep == nil {
		for _, e := range m.active {
			if e.SecretName == endpointIDOrSecret {
				ep = e
				break
			}
		}
	}
	if ep == nil {
		m.mu.Unlock()
		return errs.Newf(errs.CodeNotFound, "no active endpoint %q", endpointIDOrSecret)
	}
	delete(m.active, ep.EndpointID)
	delete(m.bySecret, ep.SecretID)
	m.mu.Unlock()
	m.shutdownEndpoint(ep, "explicit")
	return nil
}

// Refresh extends a session endpoint's expiry. Persistent endpoints have no
// expiry and refreshing them is a no-op.
func (m *Manager) Refresh(endpointID string, ttlOverrideSeconds int) (EndpointSnapshot, error) {
	m.mu.RLock()
	ep, ok := m.active[endpointID]
	m.mu.RUnlock()
	if !ok {
		return EndpointSnapshot{}, errs.Newf(errs.CodeNotFound, "no active endpoint %q", endpointID)
	}
	if ep.Mode != store.ModeSession {
		return ep.Snapshot(), nil
	}
	rec, err := m.store.GetSecret(ep.SecretID)
	if err != nil {
		return EndpointSnapshot{}, err
	}
	ttl := rec.EndpointConfig.SessionTTLSeconds
	if ttlOverrideSeconds > 0 {
		ttl = ttlOverrideSeconds
	}
	ep.setExpiresAt(time.Now().UTC().Add(time.Duration(ttl) * time.Second))
	return ep.Snapshot(), nil
}

// List returns snapshots of all open endpoints, sorted deterministically by
// secret name (then endpoint id) so polled UIs don't see rows shuffle every
// tick. Map iteration order in Go is randomised; without a sort the same
// data round-tripped through this method would arrive at the client in a
// different order each call.
func (m *Manager) List() []EndpointSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]EndpointSnapshot, 0, len(m.active))
	for _, e := range m.active {
		out = append(out, e.Snapshot())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SecretName != out[j].SecretName {
			return out[i].SecretName < out[j].SecretName
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// --- internal ---

func (m *Manager) acquireOpenLock(secretID string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	mu, ok := m.openLocks[secretID]
	if !ok {
		mu = &sync.Mutex{}
		m.openLocks[secretID] = mu
	}
	return mu
}

func (m *Manager) snapshotCloseGen() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closeGen
}

func (m *Manager) snapshotCloseGenLocked() uint64 { return m.closeGen }

func (m *Manager) acceptLoop(ep *ActiveEndpoint, adapter adapters.ProxyAdapter, rec *store.SecretRecord) {
	maxConns := rec.EndpointConfig.MaxConcurrentConnections
	if maxConns <= 0 {
		maxConns = 16
	}
	semaphore := make(chan struct{}, maxConns)
	for {
		conn, err := ep.listener.Accept()
		if err != nil {
			if ep.ctx.Err() != nil {
				return
			}
			if isClosedError(err) {
				return
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}
		select {
		case semaphore <- struct{}{}:
		default:
			_ = conn.Close()
			_ = m.audit.Write(audit.Entry{
				Event:      audit.EventConnUpstreamFail,
				EndpointID: ep.EndpointID,
				SecretID:   ep.SecretID,
				SecretName: ep.SecretName,
				Details:    map[string]any{"reason": "max_concurrent_connections_exceeded", "limit": maxConns},
			})
			continue
		}
		ep.Stats.ConnectionsOpen.Add(1)
		ep.Stats.ConnectionsTotal.Add(1)
		ep.Stats.LastActivity.Store(time.Now().UnixNano())
		_ = m.audit.Write(audit.Entry{
			Event:      audit.EventConnOpened,
			EndpointID: ep.EndpointID,
			SecretID:   ep.SecretID,
			SecretName: ep.SecretName,
			RemoteAddr: conn.RemoteAddr().String(),
		})
		go func(c net.Conn) {
			defer func() {
				<-semaphore
				ep.Stats.ConnectionsOpen.Add(-1)
			}()
			m.handleConn(ep, adapter, c)
		}(conn)
	}
}

func (m *Manager) handleConn(ep *ActiveEndpoint, adapter adapters.ProxyAdapter, conn net.Conn) {
	defer conn.Close()
	material, err := m.store.DecryptSecret(ep.SecretID)
	if err != nil {
		_ = m.audit.Write(audit.Entry{
			Event:      audit.EventConnUpstreamFail,
			EndpointID: ep.EndpointID,
			SecretID:   ep.SecretID,
			SecretName: ep.SecretName,
			RemoteAddr: conn.RemoteAddr().String(),
			Details:    map[string]any{"error": "decrypt failed: " + err.Error()},
		})
		return
	}
	defer material.Payload.Zero()
	m.store.MarkUsed(ep.SecretID)
	m.vault.Touch()

	dec := adapters.DecryptedSecret{
		ID:      ep.SecretID,
		Name:    ep.SecretName,
		Type:    ep.Type,
		Config:  material.Record.Config,
		Payload: material.Payload,
	}
	creds := toAdapterCreds(ep.LocalCreds)

	wrapped := wrapStats(conn, ep.Stats)
	err = adapter.ServeConnection(ep.ctx, wrapped, dec, creds)
	statsSnapshot := ep.Stats.Snapshot()
	ev := audit.Entry{
		Event:      audit.EventConnClosed,
		EndpointID: ep.EndpointID,
		SecretID:   ep.SecretID,
		SecretName: ep.SecretName,
		RemoteAddr: conn.RemoteAddr().String(),
		Details: map[string]any{
			"bytes_in":  statsSnapshot.BytesIn,
			"bytes_out": statsSnapshot.BytesOut,
		},
	}
	if err != nil {
		if errors.Is(err, adapters.ErrLocalAuth) {
			ev.Event = audit.EventConnUpstreamFail
			ev.Details["error"] = "local_auth_failed"
		} else if !errors.Is(err, context.Canceled) {
			ev.Details["error"] = err.Error()
		}
	}
	_ = m.audit.Write(ev)
}

// sessionExpiryWatcher is a re-armable timer that respects Refresh updates.
// It exits when the endpoint context is cancelled or when the expiry has
// genuinely passed (after re-checking on each wake-up).
func (m *Manager) sessionExpiryWatcher(ep *ActiveEndpoint) {
	for {
		wait := time.Until(ep.ExpiresAt())
		if wait <= 0 {
			break
		}
		t := time.NewTimer(wait)
		select {
		case <-ep.ctx.Done():
			t.Stop()
			return
		case <-ep.expiryReset:
			t.Stop()
			// Loop and recompute the wait based on the new ExpiresAt.
		case <-t.C:
			// Expired (modulo a Refresh that moved ExpiresAt forward — the
			// next loop iteration checks and re-arms if so).
		}
	}
	// Truly expired. Detach and shut down.
	m.mu.Lock()
	if _, ok := m.active[ep.EndpointID]; !ok {
		m.mu.Unlock()
		return
	}
	delete(m.active, ep.EndpointID)
	delete(m.bySecret, ep.SecretID)
	m.mu.Unlock()
	_ = m.audit.Write(audit.Entry{
		Event:      audit.EventEndpointExpired,
		EndpointID: ep.EndpointID,
		SecretID:   ep.SecretID,
		SecretName: ep.SecretName,
	})
	m.shutdownEndpoint(ep, "expired")
}

func (m *Manager) shutdownEndpoint(ep *ActiveEndpoint, reason string) {
	ep.cancel()
	if ep.listener != nil {
		_ = ep.listener.Close()
	}
	if ep.LocalCreds != nil {
		ep.LocalCreds.Zero()
	}
	if ep.Kind == EndpointMaterialized {
		shredFiles(ep)
		_ = m.audit.Write(audit.Entry{
			Event:      audit.EventSecretUnmaterialized,
			EndpointID: ep.EndpointID,
			SecretID:   ep.SecretID,
			SecretName: ep.SecretName,
			Details:    map[string]any{"reason": reason},
		})
		return
	}
	_ = m.audit.Write(audit.Entry{
		Event:      audit.EventEndpointClosed,
		EndpointID: ep.EndpointID,
		SecretID:   ep.SecretID,
		SecretName: ep.SecretName,
		Details: map[string]any{
			"reason":            reason,
			"bytes_in":          ep.Stats.BytesIn.Load(),
			"bytes_out":         ep.Stats.BytesOut.Load(),
			"connections_total": ep.Stats.ConnectionsTotal.Load(),
		},
	})
}

// openMaterialized opens an endpoint for a KindMaterialized secret (Section
// 16.4.2). It decrypts the secret, asks the adapter to materialize it, writes
// any rendered files, and registers a listener-less ActiveEndpoint.
func (m *Manager) openMaterialized(ctx context.Context, rec *store.SecretRecord, adapter adapters.Adapter, ttlOverrideSeconds int, startGen uint64) (EndpointSnapshot, error) {
	madapter, ok := adapter.(adapters.MaterializingAdapter)
	if !ok {
		return EndpointSnapshot{}, errs.New(errs.CodeAdapterError, "adapter does not support materialization")
	}

	material, err := m.store.DecryptSecret(rec.ID)
	if err != nil {
		return EndpointSnapshot{}, err
	}
	mat, err := madapter.Materialize(adapters.DecryptedSecret{
		ID:      rec.ID,
		Name:    rec.Name,
		Type:    rec.Type,
		Config:  rec.Config,
		Payload: material.Payload,
	})
	material.Payload.Zero()
	if err != nil {
		return EndpointSnapshot{}, errs.Wrap(errs.CodeAdapterError, err)
	}

	endpointID := ulid.Make().String()
	openedAt := time.Now().UTC()
	ttl := rec.EndpointConfig.SessionTTLSeconds
	if ttlOverrideSeconds > 0 {
		ttl = ttlOverrideSeconds
	}
	if ttl <= 0 {
		ttl = 3600
	}
	expiresAt := openedAt.Add(time.Duration(ttl) * time.Second)

	env := mat.Env
	if env == nil {
		env = map[string]string{}
	}
	epCtx, cancel := context.WithCancel(ctx)
	ep := &ActiveEndpoint{
		EndpointID:  endpointID,
		SecretID:    rec.ID,
		SecretName:  rec.Name,
		Type:        rec.Type,
		Kind:        EndpointMaterialized,
		Mode:        store.ModeSession,
		EnvVars:     env,
		OpenedAt:    openedAt,
		expiresAt:   expiresAt,
		expiryReset: make(chan struct{}, 1),
		Stats:       &EndpointStats{},
		ctx:         epCtx,
		cancel:      cancel,
	}

	// Write rendered files to a per-materialization 0700 run directory; the
	// manager owns their lifecycle and shreds them on close (Section 16.4.4).
	if len(mat.Files) > 0 {
		ep.RunDir = filepath.Join(m.runDir, endpointID)
		if err := os.MkdirAll(ep.RunDir, 0o700); err != nil {
			cancel()
			zeroFiles(mat.Files)
			return EndpointSnapshot{}, errs.Wrap(errs.CodeInternalError, err)
		}
		for _, f := range mat.Files {
			abs := filepath.Join(ep.RunDir, f.Basename)
			werr := os.WriteFile(abs, f.Content.Bytes(), 0o600)
			f.Content.Zero()
			if werr != nil {
				cancel()
				zeroFiles(mat.Files)
				shredFiles(ep)
				return EndpointSnapshot{}, errs.Wrap(errs.CodeInternalError, werr)
			}
			ep.FilePaths = append(ep.FilePaths, abs)
			if f.PathEnv != "" {
				env[f.PathEnv] = abs
			}
		}
	}

	// Atomic insert with the same close-generation / vault-state guard the
	// listener path uses, so a concurrent CloseAll does not leave files behind.
	m.mu.Lock()
	if m.snapshotCloseGenLocked() != startGen || m.vault.State() != vault.StateUnlocked {
		m.mu.Unlock()
		cancel()
		shredFiles(ep)
		return EndpointSnapshot{}, errs.New(errs.CodeVaultLocked, "vault was locked during Open")
	}
	if existing, ok := m.bySecret[rec.ID]; ok {
		ep2 := m.active[existing]
		m.mu.Unlock()
		cancel()
		shredFiles(ep)
		if ep2 != nil {
			return ep2.Snapshot(), nil
		}
		return EndpointSnapshot{}, errs.New(errs.CodeEndpointError, "concurrent open race")
	}
	m.active[endpointID] = ep
	m.bySecret[rec.ID] = endpointID
	m.mu.Unlock()

	m.store.MarkUsed(rec.ID)
	m.vault.Touch()
	go m.sessionExpiryWatcher(ep)

	_ = m.audit.Write(audit.Entry{
		Event:      audit.EventSecretMaterialized,
		SecretID:   rec.ID,
		SecretName: rec.Name,
		EndpointID: endpointID,
		Details: map[string]any{
			"env_var_names": sortedKeys(env),
			"files":         ep.FilePaths,
			"ttl_seconds":   ttl,
		},
	})
	return ep.Snapshot(), nil
}

// shredFiles overwrites and removes an endpoint's rendered files and its run
// directory. Safe to call on an endpoint that rendered no files.
func shredFiles(ep *ActiveEndpoint) {
	for _, p := range ep.FilePaths {
		if fi, err := os.Stat(p); err == nil && fi.Size() > 0 {
			if f, err := os.OpenFile(p, os.O_WRONLY, 0); err == nil {
				_, _ = f.Write(make([]byte, fi.Size()))
				_ = f.Sync()
				_ = f.Close()
			}
		}
		_ = os.Remove(p)
	}
	if ep.RunDir != "" {
		_ = os.RemoveAll(ep.RunDir)
	}
}

// zeroFiles scrubs the in-memory content of rendered files not yet written.
func zeroFiles(files []adapters.RenderedFile) {
	for _, f := range files {
		if f.Content != nil {
			f.Content.Zero()
		}
	}
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (m *Manager) allocatePort() int {
	m.mu.RLock()
	used := make(map[string]struct{}, len(m.active))
	for _, e := range m.active {
		used[e.LocalAddr] = struct{}{}
	}
	m.mu.RUnlock()
	for p := m.defaultPortStart; p < m.defaultPortStart+1000; p++ {
		addr := fmt.Sprintf("127.0.0.1:%d", p)
		if _, ok := used[addr]; ok {
			continue
		}
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			_ = ln.Close()
			return p
		}
	}
	return 0 // fall through to ephemeral
}

func listenLocal(port int) (net.Listener, int, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, 0, err
	}
	actual := ln.Addr().(*net.TCPAddr).Port
	return ln, actual, nil
}

func generateLocalCreds(require bool) (*LocalCredentials, error) {
	if !require {
		return &LocalCredentials{Username: "cloak", Password: nil}, nil
	}
	username := "cloak_" + ulid.Make().String()[:10]
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	pw := secrets.NewFromString(base64.RawURLEncoding.EncodeToString(raw))
	return &LocalCredentials{Username: strings.ToLower(username), Password: pw}, nil
}

func toAdapterCreds(c *LocalCredentials) adapters.LocalCredentials {
	if c == nil {
		return adapters.LocalCredentials{}
	}
	return adapters.LocalCredentials{Username: c.Username, Password: c.Password}
}

func normalizeEnvPrefix(name string) string {
	out := strings.ToUpper(name)
	out = strings.Map(func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		default:
			return '_'
		}
	}, out)
	return out
}

func isClosedError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, net.ErrClosed) || strings.Contains(err.Error(), "use of closed network connection")
}

// statsConn wraps a net.Conn and accumulates byte counters.
type statsConn struct {
	net.Conn
	stats *EndpointStats
}

func wrapStats(c net.Conn, stats *EndpointStats) net.Conn {
	return &statsConn{Conn: c, stats: stats}
}

func (s *statsConn) Read(p []byte) (int, error) {
	n, err := s.Conn.Read(p)
	if n > 0 {
		s.stats.BytesIn.Add(int64(n))
		s.stats.LastActivity.Store(time.Now().UnixNano())
	}
	return n, err
}

func (s *statsConn) Write(p []byte) (int, error) {
	n, err := s.Conn.Write(p)
	if n > 0 {
		s.stats.BytesOut.Add(int64(n))
		s.stats.LastActivity.Store(time.Now().UnixNano())
	}
	return n, err
}
