package endpoints_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Chekunin/cloak/internal/adapters"
	"github.com/Chekunin/cloak/internal/adapters/httpadapter"
	"github.com/Chekunin/cloak/internal/audit"
	"github.com/Chekunin/cloak/internal/endpoints"
	"github.com/Chekunin/cloak/internal/secrets"
	"github.com/Chekunin/cloak/internal/store"
	"github.com/Chekunin/cloak/internal/vault"
)

// buildEnv constructs a manager-ready tuple (vault, store, audit, registry, mgr)
// for use in tests. The vault is unlocked and the registry has only the HTTP
// adapter — which doesn't need to actually talk to an upstream during these
// tests because we never open a connection.
func buildEnv(t *testing.T) (*vault.Manager, *store.Store, *audit.Logger, *endpoints.Manager, func()) {
	t.Helper()
	dir := t.TempDir()

	v, err := vault.New(filepath.Join(dir, "vault.meta.json"), 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	pw := secrets.NewFromString("pw")
	if err := v.Init(pw); err != nil {
		t.Fatal(err)
	}
	if err := v.Unlock(pw); err != nil {
		t.Fatal(err)
	}
	pw.Zero()

	st, err := store.Open(filepath.Join(dir, "vault.db"), v)
	if err != nil {
		t.Fatal(err)
	}
	au, err := audit.Open(filepath.Join(dir, "audit.log"))
	if err != nil {
		t.Fatal(err)
	}
	reg := adapters.NewRegistry()
	reg.Register(httpadapter.New())

	em := endpoints.NewManager(reg, v, st, au, 0)
	v.RegisterLockHook(func(reason vault.LockReason) { em.CloseAll(string(reason)) })

	cleanup := func() {
		v.Shutdown()
		_ = st.Close()
		_ = au.Close()
	}
	return v, st, au, em, cleanup
}

// TestOpenIdempotentUnderConcurrency verifies that 50 goroutines opening the
// same secret produce one endpoint, not 50 listeners.
func TestOpenIdempotentUnderConcurrency(t *testing.T) {
	_, st, _, em, cleanup := buildEnv(t)
	defer cleanup()

	rec, err := st.CreateSecret("api", store.TypeHTTP, "",
		map[string]any{"upstream": "http://127.0.0.1:1"},
		store.EndpointConfig{Mode: store.ModeSession, RequireLocalAuth: false},
		[]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}

	const N = 50
	var wg sync.WaitGroup
	ids := make([]string, N)
	errs := make([]error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			snap, err := em.Open(context.Background(), rec.Name, 60)
			errs[i] = err
			if err == nil {
				ids[i] = snap.ID
			}
		}(i)
	}
	wg.Wait()

	unique := map[string]bool{}
	for i, e := range errs {
		if e != nil {
			t.Errorf("Open[%d]: %v", i, e)
			continue
		}
		unique[ids[i]] = true
	}
	if len(unique) != 1 {
		t.Fatalf("got %d unique endpoint ids for one secret, want 1", len(unique))
	}

	open := em.List()
	if len(open) != 1 {
		t.Fatalf("manager has %d active endpoints, want 1", len(open))
	}
}

// TestRefreshExtendsExpiry verifies that Refresh actually moves the expiry
// forward and that the watcher honours it instead of expiring on the original
// schedule.
func TestRefreshExtendsExpiry(t *testing.T) {
	_, st, _, em, cleanup := buildEnv(t)
	defer cleanup()

	rec, err := st.CreateSecret("short", store.TypeHTTP, "",
		map[string]any{"upstream": "http://127.0.0.1:1"},
		store.EndpointConfig{Mode: store.ModeSession, SessionTTLSeconds: 1, RequireLocalAuth: false},
		[]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}

	snap, err := em.Open(context.Background(), rec.Name, 1)
	if err != nil {
		t.Fatal(err)
	}
	originalExpiry := snap.ExpiresAt

	// Refresh with 60s TTL well before the 1s expiry fires.
	time.Sleep(200 * time.Millisecond)
	extended, err := em.Refresh(snap.ID, 60)
	if err != nil {
		t.Fatal(err)
	}
	if !extended.ExpiresAt.After(originalExpiry) {
		t.Fatalf("expiry not extended: was %v, now %v", originalExpiry, extended.ExpiresAt)
	}

	// Wait past the *original* TTL; the endpoint should still be open.
	time.Sleep(1200 * time.Millisecond)
	if got := em.List(); len(got) != 1 {
		t.Fatalf("endpoint expired despite refresh: list=%+v", got)
	}
}

// TestCloseAllDuringConcurrentOpen verifies that no listener is leaked when
// CloseAll fires while concurrent Open calls are in flight.
func TestCloseAllDuringConcurrentOpen(t *testing.T) {
	v, st, _, em, cleanup := buildEnv(t)
	defer cleanup()

	for i := 0; i < 10; i++ {
		_, err := st.CreateSecret("s"+string(rune('0'+i)), store.TypeHTTP, "",
			map[string]any{"upstream": "http://127.0.0.1:1"},
			store.EndpointConfig{Mode: store.ModeSession, RequireLocalAuth: false},
			[]byte(`{}`))
		if err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = em.Open(context.Background(), "s"+string(rune('0'+i)), 60)
		}()
	}
	// While Opens are in flight, lock the vault (which triggers CloseAll).
	time.Sleep(2 * time.Millisecond)
	v.Lock(vault.LockReasonExplicit)
	wg.Wait()

	// All endpoints should be closed.
	if got := em.List(); len(got) != 0 {
		t.Fatalf("endpoints survived CloseAll: %+v", got)
	}
}
