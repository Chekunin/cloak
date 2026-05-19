package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/Chekunin/cloak/pkg/client"
)

func newConnectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "connect <name>",
		Short: "Open a session endpoint and launch the appropriate native client",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, _, ctx, cancel, err := dialBackground(true)
			if err != nil {
				return err
			}
			defer cancel()

			rec, err := cli.GetSecret(ctx, args[0])
			if err != nil {
				_ = cli.Close()
				return err
			}
			ep, err := cli.OpenEndpoint(ctx, args[0], 0)
			if err != nil {
				_ = cli.Close()
				return err
			}
			// We deliberately keep the daemon-side endpoint open after the
			// CLI exits — closing requires the daemon, and a session endpoint
			// expires on its own TTL anyway. Document trade-off in connect.
			_ = cli.Close()

			switch rec.Type {
			case client.TypePostgres:
				return runChild(ep.EnvVars,
					"psql",
					"-h", "127.0.0.1",
					"-p", portFromAddr(ep.LocalAddr),
					"-U", envOrEmpty(ep.EnvVars, "PGUSER"),
					"-d", envOrEmpty(ep.EnvVars, "PGDATABASE"),
				)
			case client.TypeMySQL:
				return runChild(ep.EnvVars,
					"mysql",
					"-h", "127.0.0.1",
					"-P", portFromAddr(ep.LocalAddr),
					"-u", envOrEmpty(ep.EnvVars, "MYSQL_USER"),
					"-D", envOrEmpty(ep.EnvVars, "MYSQL_DATABASE"),
				)
			case client.TypeSSH:
				fmt.Fprintln(os.Stderr, "SSH endpoint open. Connect with:")
				fmt.Fprintf(os.Stderr, "  ssh -p %s %s@127.0.0.1\n", portFromAddr(ep.LocalAddr), envOrEmpty(ep.EnvVars, ""))
				fmt.Fprintf(os.Stderr, "Local password: %s\n", ep.EnvVars[envFirstKeyEndingWith(ep.EnvVars, "_SSH_PASSWORD")])
				return nil
			case client.TypeHTTP:
				fmt.Fprintf(os.Stderr, "URL: %s\n", ep.ConnectionString)
				if token, ok := ep.EnvVars[envFirstKeyEndingWith(ep.EnvVars, "_TOKEN")]; ok {
					fmt.Fprintf(os.Stderr, "Bearer token: %s\n", token)
				}
				return nil
			default:
				return fmt.Errorf("connect not supported for type %q", rec.Type)
			}
		},
	}
}

// runChild executes name+args with extra env vars merged on top of os.Environ.
func runChild(extraEnv map[string]string, name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	env := os.Environ()
	for k, v := range extraEnv {
		env = append(env, k+"="+v)
	}
	c.Env = env
	return c.Run()
}

func portFromAddr(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[i+1:]
		}
	}
	return addr
}

func envOrEmpty(m map[string]string, key string) string {
	if key == "" {
		return ""
	}
	return m[key]
}

func envFirstKeyEndingWith(m map[string]string, suffix string) string {
	for k := range m {
		if len(k) >= len(suffix) && k[len(k)-len(suffix):] == suffix {
			return k
		}
	}
	return ""
}
