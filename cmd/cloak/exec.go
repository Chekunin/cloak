package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

func newExecCmd() *cobra.Command {
	var with []string
	c := &cobra.Command{
		Use:   "exec --with <name>[,<name>...] -- <command...>",
		Short: "Run a command with endpoint env vars injected for each named secret",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("missing command after --")
			}
			names := splitCSV(with)
			if len(names) == 0 {
				return errors.New("--with is required")
			}
			cli, _, ctx, cancel, err := dialBackground(true)
			if err != nil {
				return err
			}
			defer cancel()
			defer cli.Close()

			// Open all endpoints up front.
			env := map[string]string{}
			openedIDs := make([]string, 0, len(names))
			for _, n := range names {
				ep, err := cli.OpenEndpoint(ctx, n, 0)
				if err != nil {
					return fmt.Errorf("open endpoint %q: %w", n, err)
				}
				openedIDs = append(openedIDs, ep.ID)
				for k, v := range ep.EnvVars {
					env[k] = v
				}
			}

			child := exec.Command(args[0], args[1:]...)
			child.Stdin = os.Stdin
			child.Stdout = os.Stdout
			child.Stderr = os.Stderr
			extra := os.Environ()
			for k, v := range env {
				extra = append(extra, k+"="+v)
			}
			child.Env = extra

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			defer signal.Stop(sigCh)

			if err := child.Start(); err != nil {
				_ = closeAll(openedIDs)
				return err
			}

			exitCode := 0
			doneCh := make(chan error, 1)
			go func() { doneCh <- child.Wait() }()
			select {
			case sig := <-sigCh:
				_ = child.Process.Signal(sig)
				err := <-doneCh
				if ee, ok := err.(*exec.ExitError); ok {
					exitCode = ee.ExitCode()
				}
			case err := <-doneCh:
				if err != nil {
					if ee, ok := err.(*exec.ExitError); ok {
						exitCode = ee.ExitCode()
					} else {
						_ = closeAll(openedIDs)
						return err
					}
				}
			}

			_ = closeAll(openedIDs)
			if exitCode != 0 {
				os.Exit(exitCode)
			}
			return nil
		},
	}
	c.Flags().StringSliceVar(&with, "with", nil, "comma-separated secret names whose endpoints to inject")
	c.Flags().SetInterspersed(false)
	return c
}

func splitCSV(s []string) []string {
	var out []string
	for _, item := range s {
		for _, piece := range strings.Split(item, ",") {
			piece = strings.TrimSpace(piece)
			if piece != "" {
				out = append(out, piece)
			}
		}
	}
	return out
}

func closeAll(endpointIDs []string) error {
	if len(endpointIDs) == 0 {
		return nil
	}
	cli, _, ctx, cancel, err := dialBackground(true)
	if err != nil {
		return err
	}
	defer cancel()
	defer cli.Close()
	for _, id := range endpointIDs {
		_ = cli.CloseEndpoint(ctx, id)
	}
	return nil
}

var _ = context.Background
