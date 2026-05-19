//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package ipc

import (
	"net"

	"golang.org/x/sys/unix"
)

// LOCAL_PEERPID is darwin-only; level is SOL_LOCAL == 0.
const localPeerPID = 2

func peerPID(c net.Conn) int {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return 0
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return 0
	}
	var pid int
	_ = raw.Control(func(fd uintptr) {
		p, err := unix.GetsockoptInt(int(fd), 0, localPeerPID)
		if err == nil {
			pid = p
		}
	})
	return pid
}
