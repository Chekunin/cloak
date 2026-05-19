package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newTokenCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "token", Short: "Manage client tokens"}
	cmd.AddCommand(newTokenCreateCmd())
	cmd.AddCommand(newTokenListCmd())
	cmd.AddCommand(newTokenRevokeCmd())
	return cmd
}

func newTokenCreateCmd() *cobra.Command {
	var name string
	var save bool
	c := &cobra.Command{
		Use:   "create",
		Short: "Issue a new client token (printed once)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			// Try authenticated first; fall back to unauthenticated for the
			// bootstrap case where no client token exists yet.
			cli, p, ctx, cancel, err := dialBackground(true)
			if err != nil {
				if cancel != nil {
					cancel()
				}
				cli, p, ctx, cancel, err = dialBackground(false)
				if err != nil {
					return err
				}
			}
			defer cancel()
			defer cli.Close()
			t, err := cli.CreateToken(ctx, name)
			if err != nil {
				return err
			}
			emit(t, func() {
				fmt.Println("Token:")
				fmt.Println(t.Token)
				if save {
					_ = saveToken(p, t.Token)
					fmt.Fprintf(os.Stderr, "Saved to %s/cli_token\n", p.Home)
				} else {
					fmt.Fprintf(os.Stderr, "\nSave this — it will not be shown again.\n")
					fmt.Fprintf(os.Stderr, "Use `--save` to write it to %s/cli_token.\n", p.Home)
				}
			})
			return nil
		},
	}
	c.Flags().StringVar(&name, "name", "", "human-readable name for this token")
	c.Flags().BoolVar(&save, "save", false, "write the token to ~/.cloak/cli_token")
	return c
}

func newTokenListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List client tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, _, ctx, cancel, err := dialBackground(true)
			if err != nil {
				return err
			}
			defer cancel()
			defer cli.Close()
			toks, err := cli.ListTokens(ctx)
			if err != nil {
				return err
			}
			emit(toks, func() {
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "ID\tNAME\tCREATED\tLAST SEEN\tREVOKED")
				for _, t := range toks {
					last := "-"
					if t.LastSeenAt != nil {
						last = t.LastSeenAt.Local().Format("2006-01-02 15:04")
					}
					revoked := ""
					if t.Revoked {
						revoked = "yes"
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
						t.ID, t.Name, t.CreatedAt.Local().Format("2006-01-02 15:04"), last, revoked)
				}
				_ = w.Flush()
			})
			return nil
		},
	}
}

func newTokenRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <token-id>",
		Short: "Revoke a client token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, _, ctx, cancel, err := dialBackground(true)
			if err != nil {
				return err
			}
			defer cancel()
			defer cli.Close()
			return cli.RevokeToken(ctx, args[0])
		},
	}
}
