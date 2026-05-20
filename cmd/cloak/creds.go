package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/Chekunin/cloak/pkg/client"
)

// newCredsCmd implements `cloak creds <name>` — a pull-based way to consume a
// materialized (env) secret. Its primary use is as an AWS/gcloud
// `credential_process` helper (Section 16.6).
func newCredsCmd() *cobra.Command {
	var format string
	c := &cobra.Command{
		Use:   "creds <name>",
		Short: "Print a materialized (env) secret's values (e.g. as a credential_process helper)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, _, ctx, cancel, err := dialBackground(true)
			if err != nil {
				return err
			}
			defer cancel()
			defer cli.Close()

			rec, err := cli.GetSecret(ctx, args[0])
			if err != nil {
				return err
			}
			if rec.Type != client.TypeEnv {
				return fmt.Errorf("`cloak creds` supports only env secrets; %q is type %s", rec.Name, rec.Type)
			}
			ep, err := cli.OpenEndpoint(ctx, args[0], 0)
			if err != nil {
				return err
			}
			// Pull-only: the values are already in the response; don't leave a
			// materialization handle (and its files) lingering.
			_ = cli.CloseEndpoint(ctx, ep.ID)

			return printCreds(ep.EnvVars, format)
		},
	}
	c.Flags().StringVar(&format, "format", "env", "output format: env|json|aws")
	return c
}

func printCreds(values map[string]string, format string) error {
	switch format {
	case "env":
		keys := make([]string, 0, len(values))
		for k := range values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("%s=%s\n", k, values[k])
		}
		return nil
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(values)
	case "aws":
		// AWS credential_process expects this exact JSON shape.
		id, idOK := values["AWS_ACCESS_KEY_ID"]
		secret, secretOK := values["AWS_SECRET_ACCESS_KEY"]
		if !idOK || !secretOK {
			return fmt.Errorf("--format aws requires AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY in the secret")
		}
		out := struct {
			Version         int    `json:"Version"`
			AccessKeyID     string `json:"AccessKeyId"`
			SecretAccessKey string `json:"SecretAccessKey"`
			SessionToken    string `json:"SessionToken,omitempty"`
		}{
			Version:         1,
			AccessKeyID:     id,
			SecretAccessKey: secret,
			SessionToken:    values["AWS_SESSION_TOKEN"],
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	default:
		return fmt.Errorf("unknown format %q (want env|json|aws)", format)
	}
}
