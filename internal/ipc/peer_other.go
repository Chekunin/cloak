//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly)

package ipc

import "net"

func peerPID(_ net.Conn) int { return 0 }
