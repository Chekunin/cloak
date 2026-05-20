// cloak is the CLI client. It is stateless; the daemon (cloakd) holds the
// vault. All meaningful work happens via JSON-RPC over a Unix domain socket.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	jsonOutput bool
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "cloak:", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "cloak",
		Short:         "Local secret broker",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON")

	root.AddCommand(newInitCmd())
	root.AddCommand(newDaemonCmd())
	root.AddCommand(newUnlockCmd())
	root.AddCommand(newLockCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newSecretCmd())
	root.AddCommand(newEndpointCmd())
	root.AddCommand(newConnectCmd())
	root.AddCommand(newExecCmd())
	root.AddCommand(newCredsCmd())
	root.AddCommand(newTokenCmd())
	root.AddCommand(newLogCmd())
	return root
}
