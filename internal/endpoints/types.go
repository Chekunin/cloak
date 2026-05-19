package endpoints

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Chekunin/cloak/internal/secrets"
	"github.com/Chekunin/cloak/internal/store"
)

// LocalCredentials are the ephemeral username/password a client uses to
// authenticate against the listener.
type LocalCredentials struct {
	Username string
	Password *secrets.SecretBytes
}

// Zero scrubs the password and releases the username.
func (l *LocalCredentials) Zero() {
	if l == nil {
		return
	}
	if l.Password != nil {
		l.Password.Zero()
		l.Password = nil
	}
	l.Username = ""
}

// EndpointStats accumulates per-endpoint counters. The fields are read with
// atomic.LoadInt64; writers also use atomics. v1 uses cumulative counters.
// TODO(v1.x): switch to a rolling window when the GUI needs it.
type EndpointStats struct {
	BytesIn          atomic.Int64
	BytesOut         atomic.Int64
	ConnectionsOpen  atomic.Int64
	ConnectionsTotal atomic.Int64
	LastActivity     atomic.Int64 // unix nano
}

// StatsSnapshot is the JSON-friendly value type for IPC and audit consumption.
type StatsSnapshot struct {
	BytesIn          int64     `json:"bytes_in"`
	BytesOut         int64     `json:"bytes_out"`
	ConnectionsOpen  int64     `json:"connections_open"`
	ConnectionsTotal int64     `json:"connections_total"`
	LastActivity     time.Time `json:"last_activity,omitempty"`
}

// Snapshot copies the atomic counters.
func (s *EndpointStats) Snapshot() StatsSnapshot {
	la := s.LastActivity.Load()
	var t time.Time
	if la > 0 {
		t = time.Unix(0, la).UTC()
	}
	return StatsSnapshot{
		BytesIn:          s.BytesIn.Load(),
		BytesOut:         s.BytesOut.Load(),
		ConnectionsOpen:  s.ConnectionsOpen.Load(),
		ConnectionsTotal: s.ConnectionsTotal.Load(),
		LastActivity:     t,
	}
}

// ActiveEndpoint is the in-memory state for one open listener.
//
// Fields accessed concurrently are documented inline; the rest are written
// once at construction and read-only afterwards.
type ActiveEndpoint struct {
	EndpointID       string
	SecretID         string
	SecretName       string
	Type             store.SecretType
	Mode             store.EndpointMode
	LocalAddr        string
	ConnectionString string
	EnvVars          map[string]string
	LocalCreds       *LocalCredentials
	OpenedAt         time.Time
	Stats            *EndpointStats

	// expiresAt and the expiry reset signal are read by sessionExpiryWatcher
	// and written under Manager.mu. Refresh sends a non-blocking notify to
	// expiryReset so the watcher re-arms its timer.
	expiresAtMu sync.RWMutex
	expiresAt   time.Time
	expiryReset chan struct{}

	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc
}

// ExpiresAt returns the current expiry (or zero time for persistent endpoints).
func (e *ActiveEndpoint) ExpiresAt() time.Time {
	e.expiresAtMu.RLock()
	defer e.expiresAtMu.RUnlock()
	return e.expiresAt
}

// setExpiresAt updates the expiry and pokes the watcher.
func (e *ActiveEndpoint) setExpiresAt(t time.Time) {
	e.expiresAtMu.Lock()
	e.expiresAt = t
	e.expiresAtMu.Unlock()
	if e.expiryReset != nil {
		select {
		case e.expiryReset <- struct{}{}:
		default:
		}
	}
}

// EndpointSnapshot is a JSON-friendly view of an ActiveEndpoint for IPC.
type EndpointSnapshot struct {
	ID               string             `json:"id"`
	SecretID         string             `json:"secret_id"`
	SecretName       string             `json:"secret_name"`
	Type             store.SecretType   `json:"type"`
	Mode             store.EndpointMode `json:"mode"`
	LocalAddr        string             `json:"local_addr"`
	ConnectionString string             `json:"connection_string"`
	EnvVars          map[string]string  `json:"env_vars,omitempty"`
	OpenedAt         time.Time          `json:"opened_at"`
	ExpiresAt        time.Time          `json:"expires_at,omitempty"`
	Stats            StatsSnapshot      `json:"stats"`
}

// Snapshot returns an EndpointSnapshot for IPC.
func (e *ActiveEndpoint) Snapshot() EndpointSnapshot {
	return EndpointSnapshot{
		ID:               e.EndpointID,
		SecretID:         e.SecretID,
		SecretName:       e.SecretName,
		Type:             e.Type,
		Mode:             e.Mode,
		LocalAddr:        e.LocalAddr,
		ConnectionString: e.ConnectionString,
		EnvVars:          e.EnvVars,
		OpenedAt:         e.OpenedAt,
		ExpiresAt:        e.ExpiresAt(),
		Stats:            e.Stats.Snapshot(),
	}
}
