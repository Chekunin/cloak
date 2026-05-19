package vault

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Chekunin/cloak/internal/secrets"
)

func TestInitUnlockLock(t *testing.T) {
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "vault.meta.json")
	m, err := New(metaPath, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if m.State() != StateUninitialized {
		t.Fatalf("initial state = %v", m.State())
	}

	pw := secrets.NewFromString("hunter2")
	if err := m.Init(pw); err != nil {
		t.Fatal(err)
	}
	pw.Zero()
	if m.State() != StateLocked {
		t.Fatalf("after Init state = %v", m.State())
	}

	// Wrong password fails.
	bad := secrets.NewFromString("nope")
	if err := m.Unlock(bad); err == nil {
		t.Fatal("Unlock with wrong password should fail")
	}
	bad.Zero()
	if m.State() != StateLocked {
		t.Fatal("state should remain Locked after failed unlock")
	}

	good := secrets.NewFromString("hunter2")
	if err := m.Unlock(good); err != nil {
		t.Fatal(err)
	}
	good.Zero()
	if m.State() != StateUnlocked {
		t.Fatalf("state = %v, want Unlocked", m.State())
	}

	// Encrypt/decrypt round trip with AAD.
	ct, err := m.Encrypt([]byte("payload"), []byte("aad.v1"))
	if err != nil {
		t.Fatal(err)
	}
	pt, err := m.Decrypt(ct, []byte("aad.v1"))
	if err != nil {
		t.Fatal(err)
	}
	if string(pt.Bytes()) != "payload" {
		t.Fatal("plaintext mismatch")
	}
	pt.Zero()

	// AAD mismatch rejects.
	if _, err := m.Decrypt(ct, []byte("other")); err == nil {
		t.Fatal("expected AAD-mismatch failure")
	}

	m.Lock(LockReasonExplicit)
	if m.State() != StateLocked {
		t.Fatalf("after Lock state = %v", m.State())
	}
}

func TestReopenPersistsMeta(t *testing.T) {
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "vault.meta.json")
	m, err := New(metaPath, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	pw := secrets.NewFromString("p4ss")
	if err := m.Init(pw); err != nil {
		t.Fatal(err)
	}
	pw.Zero()

	m2, err := New(metaPath, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if m2.State() != StateLocked {
		t.Fatalf("reopened state = %v", m2.State())
	}
	good := secrets.NewFromString("p4ss")
	if err := m2.Unlock(good); err != nil {
		t.Fatal(err)
	}
	good.Zero()
}
