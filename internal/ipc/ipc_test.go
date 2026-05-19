package ipc_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/Chekunin/cloak/internal/adapters"
	"github.com/Chekunin/cloak/internal/adapters/httpadapter"
	"github.com/Chekunin/cloak/internal/audit"
	"github.com/Chekunin/cloak/internal/endpoints"
	"github.com/Chekunin/cloak/internal/ipc"
	"github.com/Chekunin/cloak/internal/secrets"
	"github.com/Chekunin/cloak/internal/store"
	"github.com/Chekunin/cloak/internal/vault"
	pkgclient "github.com/Chekunin/cloak/pkg/client"
)

// TestE2EHTTP exercises the full bootstrap → unlock → create secret →
// open endpoint → HTTP request flow against a real httptest upstream.
func TestE2EHTTP(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "cloakd.sock")

	// Upstream that echoes back the Authorization header so we can assert that
	// Cloak strips the local-auth bearer and inserts the injected one.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(r.Header.Get("Authorization")))
	}))
	defer upstream.Close()

	auditLog, err := audit.Open(filepath.Join(dir, "audit.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer auditLog.Close()

	v, err := vault.New(filepath.Join(dir, "vault.meta.json"), 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Shutdown()

	st, err := store.Open(filepath.Join(dir, "vault.db"), v)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	reg := adapters.NewRegistry()
	reg.Register(httpadapter.New())

	em := endpoints.NewManager(reg, v, st, auditLog, 0)
	v.RegisterLockHook(func(reason vault.LockReason) { em.CloseAll(string(reason)) })

	server := ipc.New(socketPath, st, auditLog, zerolog.Nop())
	ipc.RegisterAll(server, ipc.Deps{
		Vault: v, Store: st, Endpoints: em, Audit: auditLog, Adapters: reg,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := server.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	c, err := pkgclient.Dial(ctx, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// vault.init → unlock.
	if err := c.VaultInit(ctx, "pw"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := c.VaultUnlock(ctx, "pw"); err != nil {
		t.Fatalf("unlock: %v", err)
	}

	// bootstrap token.
	tok, err := c.CreateToken(ctx, "test")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	if err := c.Authenticate(ctx, tok.Token); err != nil {
		t.Fatalf("hello: %v", err)
	}

	// create an HTTP secret.
	rec, err := c.CreateSecret(ctx, pkgclient.CreateSecretRequest{
		Name: "test-api",
		Type: pkgclient.TypeHTTP,
		Config: map[string]any{
			"upstream":         upstream.URL,
			"follow_redirects": true,
		},
		Secret: map[string]any{
			"inject": map[string]any{
				"headers": map[string]string{"Authorization": "Bearer {{ .key }}"},
			},
			"values": map[string]string{"key": "sk_test_abc"},
		},
		EndpointConfig: &pkgclient.EndpointConfig{
			Mode:             pkgclient.ModeSession,
			RequireLocalAuth: true,
		},
	})
	if err != nil {
		t.Fatalf("create secret: %v", err)
	}

	// open endpoint.
	ep, err := c.OpenEndpoint(ctx, rec.Name, 60)
	if err != nil {
		t.Fatalf("open endpoint: %v", err)
	}

	// Hit the local endpoint with the injected bearer token Cloak gave us.
	req, _ := http.NewRequest("GET", ep.ConnectionString+"/", nil)
	req.Header.Set("Authorization", "Bearer "+ep.EnvVars[envFirstKey(ep.EnvVars, "_TOKEN")])
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	got, _ := readAll(resp.Body)
	if got != "Bearer sk_test_abc" {
		t.Fatalf("upstream saw Authorization %q, want Bearer sk_test_abc", got)
	}

	// 401 when local auth fails.
	req2, _ := http.NewRequest("GET", ep.ConnectionString+"/", nil)
	req2.Header.Set("Authorization", "Bearer wrong")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("http2: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 401 {
		t.Fatalf("expected 401 with bad local auth, got %d", resp2.StatusCode)
	}

	// vault.lock closes endpoints.
	if err := c.VaultLock(ctx); err != nil {
		t.Fatal(err)
	}
	eps, _ := c.ListEndpoints(ctx)
	if len(eps) != 0 {
		t.Fatalf("endpoints not closed: %d remain", len(eps))
	}

	// Suppress unused warning for secrets package import.
	_ = secrets.NewFromString
}

func envFirstKey(m map[string]string, suffix string) string {
	for k := range m {
		if len(k) >= len(suffix) && k[len(k)-len(suffix):] == suffix {
			return k
		}
	}
	return ""
}

func readAll(rc interface {
	Read(p []byte) (int, error)
}) (string, error) {
	buf := make([]byte, 0, 256)
	tmp := make([]byte, 256)
	for {
		n, err := rc.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			if err.Error() == "EOF" {
				return string(buf), nil
			}
			return string(buf), err
		}
	}
}
