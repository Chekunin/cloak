//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly)

package secrets

// On platforms without a working mlock equivalent we fall back to plain memory.
// Best-effort: Zero() still scrubs the buffer.
func mlock(b []byte) bool   { return false }
func munlock(b []byte) error { return nil }
