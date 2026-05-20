package main

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newEndpointCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "endpoint", Short: "Manage active endpoints"}
	cmd.AddCommand(newEndpointListCmd())
	cmd.AddCommand(newEndpointOpenCmd())
	cmd.AddCommand(newEndpointCloseCmd())
	return cmd
}

func newEndpointListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List currently open endpoints",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, _, ctx, cancel, err := dialBackground(true)
			if err != nil {
				return err
			}
			defer cancel()
			defer cli.Close()
			eps, err := cli.ListEndpoints(ctx)
			if err != nil {
				return err
			}
			emit(eps, func() {
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "NAME\tTYPE\tKIND\tMODE\tADDRESS\tCONNS")
				for _, e := range eps {
					addr := e.LocalAddr
					if addr == "" {
						addr = "-"
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\n",
						e.SecretName, e.Type, e.Kind, e.Mode, addr, e.Stats.ConnectionsTotal)
				}
				_ = w.Flush()
			})
			return nil
		},
	}
}

func newEndpointOpenCmd() *cobra.Command {
	var ttl int
	c := &cobra.Command{
		Use:   "open <name>",
		Short: "Open a session endpoint for the named secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, _, ctx, cancel, err := dialBackground(true)
			if err != nil {
				return err
			}
			defer cancel()
			defer cli.Close()
			ep, err := cli.OpenEndpoint(ctx, args[0], ttl)
			if err != nil {
				return err
			}
			emit(ep, func() {
				if ep.LocalAddr != "" {
					fmt.Printf("Endpoint:        %s\n", ep.LocalAddr)
				}
				if ep.ConnectionString != "" {
					fmt.Printf("Connection URL:  %s\n", ep.ConnectionString)
				}
				if !ep.ExpiresAt.IsZero() {
					fmt.Printf("Expires at:      %s\n", ep.ExpiresAt.Local().Format("2006-01-02 15:04:05"))
				}
				if ep.Kind == "materialized" {
					// Materialized secret values are real credentials; print
					// only the variable names, never the values.
					names := make([]string, 0, len(ep.EnvVars))
					for k := range ep.EnvVars {
						names = append(names, k)
					}
					sort.Strings(names)
					fmt.Println("Injected variables (values hidden):")
					for _, k := range names {
						fmt.Printf("  %s\n", k)
					}
					fmt.Fprintln(os.Stderr, "Consume with `cloak exec --with <name> -- ...` or `cloak creds <name>`.")
				} else if len(ep.EnvVars) > 0 {
					fmt.Println("Environment:")
					for k, v := range ep.EnvVars {
						fmt.Printf("  %s=%s\n", k, v)
					}
				}
			})
			return nil
		},
	}
	c.Flags().IntVar(&ttl, "ttl", 0, "session TTL in seconds (overrides secret default)")
	return c
}

func newEndpointCloseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "close <endpoint-id-or-secret-name>",
		Short: "Close an active endpoint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, _, ctx, cancel, err := dialBackground(true)
			if err != nil {
				return err
			}
			defer cancel()
			defer cli.Close()
			return cli.CloseEndpoint(ctx, args[0])
		},
	}
}
