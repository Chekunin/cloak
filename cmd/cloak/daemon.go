package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Chekunin/cloak/internal/paths"
)

func newDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "daemon", Short: "Manage the cloakd daemon"}
	cmd.AddCommand(newDaemonStartCmd())
	cmd.AddCommand(newDaemonStopCmd())
	cmd.AddCommand(newDaemonStatusCmd())
	return cmd
}

func newDaemonStartCmd() *cobra.Command {
	var foreground bool
	c := &cobra.Command{
		Use:   "start",
		Short: "Start the cloakd daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := paths.Default()
			if err != nil {
				return err
			}
			if err := p.EnsureHome(); err != nil {
				return err
			}
			if running, _ := daemonRunning(p); running {
				fmt.Fprintln(os.Stderr, "cloakd is already running")
				return nil
			}
			bin, err := findDaemonBinary()
			if err != nil {
				return err
			}
			if foreground {
				ec := exec.Command(bin, "-foreground")
				ec.Stdout = os.Stdout
				ec.Stderr = os.Stderr
				return ec.Run()
			}
			ec := exec.Command(bin, "-foreground")
			ec.Stdout = nil
			ec.Stderr = nil
			detach(ec)
			if err := ec.Start(); err != nil {
				return err
			}
			// Wait briefly for the socket to appear.
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if _, err := os.Stat(p.SocketPath()); err == nil {
					fmt.Fprintln(os.Stderr, "cloakd started.")
					return nil
				}
				time.Sleep(100 * time.Millisecond)
			}
			return fmt.Errorf("daemon did not become ready within 5s")
		},
	}
	c.Flags().BoolVar(&foreground, "foreground", false, "run in the foreground")
	return c
}

func newDaemonStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the cloakd daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := paths.Default()
			if err != nil {
				return err
			}
			pid, err := readPID(p.PIDFile())
			if err != nil {
				return err
			}
			if err := signalTerminate(pid); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "stop signal sent")
			return nil
		},
	}
}

func newDaemonStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether cloakd is running",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := paths.Default()
			if err != nil {
				return err
			}
			running, pid := daemonRunning(p)
			emit(map[string]any{"running": running, "pid": pid, "socket": p.SocketPath()},
				func() {
					if running {
						fmt.Printf("running (pid %d, socket %s)\n", pid, p.SocketPath())
						return
					}
					fmt.Println("not running")
				})
			return nil
		},
	}
}

func daemonRunning(p paths.Paths) (bool, int) {
	pid, err := readPID(p.PIDFile())
	if err != nil {
		return false, 0
	}
	return processAlive(pid), pid
}

func readPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("no PID file at %s (daemon not running?)", path)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}
	return pid, nil
}

// findDaemonBinary locates the cloakd binary. Resolution order:
//  1. $CLOAKD_BIN
//  2. cloakd next to the running `cloak`
//  3. cloakd on PATH
func findDaemonBinary() (string, error) {
	if env := os.Getenv("CLOAKD_BIN"); env != "" {
		return env, nil
	}
	if exePath, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exePath), daemonBinaryName())
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	if path, err := exec.LookPath(daemonBinaryName()); err == nil {
		return path, nil
	}
	return "", errors.New("cloakd binary not found; set CLOAKD_BIN or place cloakd next to cloak")
}
