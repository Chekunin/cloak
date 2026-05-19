//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// detach configures cmd to run in its own session, detached from the parent's
// controlling terminal.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// signalTerminate sends SIGTERM to the daemon process.
func signalTerminate(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGTERM)
}

// processAlive reports whether pid corresponds to a running process.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// daemonBinaryName returns the executable name to look for.
func daemonBinaryName() string { return "cloakd" }
