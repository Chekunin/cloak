package secrets

import "testing"

func TestNewFromBytesAndZero(t *testing.T) {
	src := []byte("hello")
	s := NewFromBytes(src)
	if s.Len() != len(src) {
		t.Fatalf("len = %d, want %d", s.Len(), len(src))
	}
	if string(s.Bytes()) != "hello" {
		t.Fatalf("contents mismatch")
	}
	s.Zero()
	if s.Bytes() != nil {
		t.Fatalf("Zero did not reset buffer")
	}
}

func TestRandom(t *testing.T) {
	s, err := Random(16)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Zero()
	if s.Len() != 16 {
		t.Fatalf("len = %d, want 16", s.Len())
	}
	// Two consecutive randoms should not match.
	s2, _ := Random(16)
	defer s2.Zero()
	if s.Equal(s2.Bytes()) {
		t.Fatal("two random secrets matched")
	}
}

func TestEqualConstantTime(t *testing.T) {
	a := NewFromString("hunter2")
	defer a.Zero()
	if !a.Equal([]byte("hunter2")) {
		t.Fatal("equal mismatch")
	}
	if a.Equal([]byte("hunter3")) {
		t.Fatal("equal returned true for mismatched bytes")
	}
}
