// Package secrets provides SecretBytes, a small wrapper around []byte intended
// for in-memory secret material (decrypted credentials, the DEK, ephemeral
// local-endpoint passwords, etc.).
//
// SecretBytes obtains memory via mlock where supported so the buffer does not
// get paged to disk. Callers are expected to call Zero() on every SecretBytes
// they own, typically from a defer at the point of allocation.
package secrets

import (
	"crypto/rand"
	"crypto/subtle"
)

// SecretBytes holds sensitive in-memory bytes. The zero value is unusable;
// always construct with New, NewFromBytes, or Random.
type SecretBytes struct {
	buf    []byte
	locked bool
}

// New allocates a new SecretBytes of the given length, zero-initialised, with
// the underlying buffer mlock'd where the platform supports it.
func New(n int) *SecretBytes {
	if n < 0 {
		panic("secrets: negative length")
	}
	buf := make([]byte, n)
	locked := mlock(buf)
	return &SecretBytes{buf: buf, locked: locked}
}

// NewFromBytes copies src into a new SecretBytes and returns it. The caller
// remains responsible for zeroing src.
func NewFromBytes(src []byte) *SecretBytes {
	sb := New(len(src))
	copy(sb.buf, src)
	return sb
}

// NewFromString is a convenience wrapper around NewFromBytes for callers that
// already hold a string (e.g. password prompts).
func NewFromString(s string) *SecretBytes {
	return NewFromBytes([]byte(s))
}

// Random fills a new SecretBytes of length n with cryptographically random bytes.
func Random(n int) (*SecretBytes, error) {
	sb := New(n)
	if _, err := rand.Read(sb.buf); err != nil {
		sb.Zero()
		return nil, err
	}
	return sb, nil
}

// Bytes returns the underlying slice. Callers must not retain references past
// the lifetime of the SecretBytes and must not call Zero while a reader still
// holds the slice.
func (s *SecretBytes) Bytes() []byte {
	if s == nil {
		return nil
	}
	return s.buf
}

// String returns the contents as a Go string. This necessarily copies into the
// GC heap; use sparingly and prefer Bytes when feasible.
func (s *SecretBytes) String() string {
	if s == nil {
		return ""
	}
	return string(s.buf)
}

// Len reports the length of the underlying buffer.
func (s *SecretBytes) Len() int {
	if s == nil {
		return 0
	}
	return len(s.buf)
}

// Equal compares the buffer to other in constant time.
func (s *SecretBytes) Equal(other []byte) bool {
	if s == nil {
		return len(other) == 0
	}
	return subtle.ConstantTimeCompare(s.buf, other) == 1
}

// Clone returns an independent copy. Useful when handing a credential to a
// component that owns its own lifetime.
func (s *SecretBytes) Clone() *SecretBytes {
	if s == nil {
		return nil
	}
	return NewFromBytes(s.buf)
}

// Zero overwrites the buffer with zeros and releases any mlock. After Zero,
// Bytes() returns an all-zero slice of the original length; do not reuse.
func (s *SecretBytes) Zero() {
	if s == nil || s.buf == nil {
		return
	}
	for i := range s.buf {
		s.buf[i] = 0
	}
	if s.locked {
		_ = munlock(s.buf)
		s.locked = false
	}
	s.buf = nil
}
