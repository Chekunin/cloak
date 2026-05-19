package audit

import (
	"path/filepath"
	"testing"
)

func TestHashChainAndReplay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	l1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := l1.Write(Entry{Event: EventVaultUnlocked, Details: map[string]any{"i": i}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := l1.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen and append. New entries should continue the seq from 5.
	l2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	if err := l2.Write(Entry{Event: EventVaultLocked}); err != nil {
		t.Fatal(err)
	}

	entries, err := l2.Tail(0)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(entries), 6; got != want {
		t.Fatalf("entry count = %d, want %d", got, want)
	}
	for i := 1; i < len(entries); i++ {
		if entries[i].Seq != entries[i-1].Seq+1 {
			t.Fatalf("seq gap at %d: %d -> %d", i, entries[i-1].Seq, entries[i].Seq)
		}
	}
	if entries[5].Seq != 6 {
		t.Fatalf("last seq = %d, want 6", entries[5].Seq)
	}
}
