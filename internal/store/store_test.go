package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Chekunin/cloak/internal/secrets"
	"github.com/Chekunin/cloak/internal/vault"
)

func setupVault(t *testing.T) *vault.Manager {
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
	t.Cleanup(v.Shutdown)
	return v
}

func TestSecretCRUD(t *testing.T) {
	v := setupVault(t)
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "vault.db"), v)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	rec, err := s.CreateSecret("prod", TypePostgres, "",
		map[string]any{"host": "h", "port": 5432, "user": "u", "database": "d", "tls_mode": "require"},
		EndpointConfig{Mode: ModePersistent, PersistentPort: 5440, RequireLocalAuth: true},
		[]byte(`{"password":"secret"}`))
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID == "" || rec.Name != "prod" {
		t.Fatalf("bad record: %+v", rec)
	}

	got, err := s.GetSecret("prod")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != rec.ID {
		t.Fatal("id mismatch on get")
	}

	mat, err := s.DecryptSecret("prod")
	if err != nil {
		t.Fatal(err)
	}
	defer mat.Payload.Zero()
	if string(mat.Payload.Bytes()) != `{"password":"secret"}` {
		t.Fatalf("decrypted payload mismatch: %s", string(mat.Payload.Bytes()))
	}

	// Duplicate name → name_conflict.
	if _, err := s.CreateSecret("prod", TypePostgres, "", nil, EndpointConfig{}, []byte(`{}`)); err == nil {
		t.Fatal("expected name conflict")
	}

	if err := s.DeleteSecret("prod"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSecret("prod"); err == nil {
		t.Fatal("expected not_found after delete")
	}
}

func TestTokens(t *testing.T) {
	v := setupVault(t)
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "vault.db"), v)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	id, plain, err := s.CreateToken("dev")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" || plain == "" {
		t.Fatal("missing id or token")
	}
	if _, err := s.VerifyToken(plain); err != nil {
		t.Fatal(err)
	}
	if _, err := s.VerifyToken(plain + "junk"); err == nil {
		t.Fatal("expected verify to fail on corrupted token")
	}
	if err := s.RevokeToken(id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.VerifyToken(plain); err == nil {
		t.Fatal("expected verify to fail after revoke")
	}
}
