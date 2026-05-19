//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package secrets

import "golang.org/x/sys/unix"

func mlock(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	if err := unix.Mlock(b); err != nil {
		return false
	}
	return true
}

func munlock(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	return unix.Munlock(b)
}
