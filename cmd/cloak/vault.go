package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var fromStdin bool
	c := &cobra.Command{
		Use:   "init",
		Short: "Initialise a new vault (one-time setup)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, _, ctx, cancel, err := dialBackground(false)
			if err != nil {
				return err
			}
			defer cancel()
			defer cli.Close()

			pw, err := readPassword("Set vault master password: ", fromStdin)
			if err != nil {
				return err
			}
			if !fromStdin {
				confirm, err := readPassword("Confirm vault master password: ", false)
				if err != nil {
					return err
				}
				if pw != confirm {
					return fmt.Errorf("passwords do not match")
				}
			}
			if err := cli.VaultInit(ctx, pw); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "Vault initialised. Run `cloak unlock` to begin.")
			return nil
		},
	}
	c.Flags().BoolVar(&fromStdin, "from-stdin", false, "read password from stdin (single line)")
	return c
}

func newUnlockCmd() *cobra.Command {
	var fromStdin bool
	c := &cobra.Command{
		Use:   "unlock",
		Short: "Unlock the vault",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, _, ctx, cancel, err := dialBackground(false)
			if err != nil {
				return err
			}
			defer cancel()
			defer cli.Close()

			pw, err := readPassword("Vault password: ", fromStdin)
			if err != nil {
				return err
			}
			if err := cli.VaultUnlock(ctx, pw); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "Vault unlocked.")
			return nil
		},
	}
	c.Flags().BoolVar(&fromStdin, "from-stdin", false, "read password from stdin (single line)")
	return c
}

func newLockCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lock",
		Short: "Lock the vault, closing all endpoints",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, _, ctx, cancel, err := dialBackground(false)
			if err != nil {
				return err
			}
			defer cancel()
			defer cli.Close()
			if err := cli.VaultLock(ctx); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "Vault locked.")
			return nil
		},
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show vault status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, _, ctx, cancel, err := dialBackground(false)
			if err != nil {
				return err
			}
			defer cancel()
			defer cli.Close()
			s, err := cli.VaultStatus(ctx)
			if err != nil {
				return err
			}
			emit(s, func() {
				fmt.Printf("State:           %s\n", s.State)
				fmt.Printf("Idle timeout:    %ds\n", s.IdleTimeoutSec)
				if !s.ExpiresAt.IsZero() {
					fmt.Printf("Auto-lock at:    %s\n", s.ExpiresAt.Local().Format("2006-01-02 15:04:05"))
				}
				fmt.Printf("Open endpoints:  %d\n", s.EndpointsOpen)
			})
			return nil
		},
	}
}
