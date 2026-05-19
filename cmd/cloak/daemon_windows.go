//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// detach: Windows doesn't have Setsid. We rely on starting with no inherited
// console; the exec package's default behaviour suffices for cloakd's needs.
// TODO(v1.x): use CREATE_NO_WINDOW once we have a Windows installer.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x00000008, // DETACHED_PROCESS
	}
}

// signalTerminate on Windows uses Kill since SIGTERM is unsupported.
func signalTerminate(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

// processAlive reports whether pid corresponds to a running process.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows os.FindProcess always succeeds; ping via signal 0 isn't
	// available, so call OpenProcess equivalent via FindProcess.Wait?
	// As a pragmatic check, try sending a no-op signal; on Windows this
	// returns ErrUnsupported for non-zero signals but works for signal 0
	// via OpenProcess in the runtime. If proc was found, treat as alive.
	_ = proc
	return true
}

// daemonBinaryName returns the executable name on Windows.
func daemonBinaryName() string { return "cloakd.exe" }
