package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/Chekunin/cloak/pkg/client"
)

func newSecretCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "secret", Short: "Manage stored secrets"}
	cmd.AddCommand(newSecretListCmd())
	cmd.AddCommand(newSecretShowCmd())
	cmd.AddCommand(newSecretAddCmd())
	cmd.AddCommand(newSecretRotateCmd())
	cmd.AddCommand(newSecretDeleteCmd())
	return cmd
}

func newSecretListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List stored secrets",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, _, ctx, cancel, err := dialBackground(true)
			if err != nil {
				return err
			}
			defer cancel()
			defer cli.Close()
			secs, err := cli.ListSecrets(ctx)
			if err != nil {
				return err
			}
			emit(secs, func() {
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "NAME\tTYPE\tMODE\tPORT\tCREATED")
				for _, s := range secs {
					port := ""
					if s.EndpointConfig.PersistentPort != 0 {
						port = strconv.Itoa(s.EndpointConfig.PersistentPort)
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
						s.Name, s.Type, s.EndpointConfig.Mode, port, s.CreatedAt.Local().Format("2006-01-02"))
				}
				_ = w.Flush()
			})
			return nil
		},
	}
}

func newSecretShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show secret metadata (never decrypts secret material)",
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
			emit(rec, func() {
				fmt.Printf("ID:               %s\n", rec.ID)
				fmt.Printf("Name:             %s\n", rec.Name)
				fmt.Printf("Type:             %s\n", rec.Type)
				if rec.Description != "" {
					fmt.Printf("Description:      %s\n", rec.Description)
				}
				fmt.Printf("Endpoint mode:    %s\n", rec.EndpointConfig.Mode)
				if rec.EndpointConfig.PersistentPort != 0 {
					fmt.Printf("Persistent port:  %d\n", rec.EndpointConfig.PersistentPort)
				}
				fmt.Printf("Require auth:     %t\n", rec.EndpointConfig.RequireLocalAuth)
				fmt.Println("Config:")
				for k, v := range rec.Config {
					fmt.Printf("  %-22s %v\n", k+":", v)
				}
			})
			return nil
		},
	}
}

func newSecretAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <type> <name>",
		Short: "Add a new secret. <type> is ssh|postgres|mysql|http.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			typ := client.SecretType(args[0])
			name := args[1]
			cli, _, ctx, cancel, err := dialBackground(true)
			if err != nil {
				return err
			}
			defer cancel()
			defer cli.Close()
			req, err := promptForSecret(typ, name)
			if err != nil {
				return err
			}
			rec, err := cli.CreateSecret(ctx, *req)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Secret %q created (id %s).\n", rec.Name, rec.ID)
			return nil
		},
	}
	return cmd
}

func newSecretRotateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rotate <name>",
		Short: "Rotate (replace) the secret material for an existing secret",
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
			secret, err := promptForSecretPayload(rec.Type)
			if err != nil {
				return err
			}
			_, err = cli.UpdateSecret(ctx, client.UpdateSecretRequest{IDOrName: args[0], Secret: secret})
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "Rotated.")
			return nil
		},
	}
}

func newSecretDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, _, ctx, cancel, err := dialBackground(true)
			if err != nil {
				return err
			}
			defer cancel()
			defer cli.Close()
			return cli.DeleteSecret(ctx, args[0])
		},
	}
}

// promptForSecret runs the interactive form for a new secret of the given type.
func promptForSecret(typ client.SecretType, name string) (*client.CreateSecretRequest, error) {
	description, _ := readLine("Description (optional): ")
	mode, err := readLine("Endpoint mode [persistent/session, default persistent]: ")
	if err != nil {
		return nil, err
	}
	if mode == "" {
		mode = "persistent"
	}
	requireAuthStr, _ := readLine("Require local authentication? [Y/n]: ")
	requireAuth := !strings.EqualFold(strings.TrimSpace(requireAuthStr), "n")

	cfg, err := promptForConfig(typ)
	if err != nil {
		return nil, err
	}
	payload, err := promptForSecretPayload(typ)
	if err != nil {
		return nil, err
	}

	endpoint := &client.EndpointConfig{
		Mode:             client.EndpointMode(mode),
		RequireLocalAuth: requireAuth,
	}
	if endpoint.Mode == client.ModePersistent {
		portStr, _ := readLine("Persistent port (leave blank for auto): ")
		if portStr != "" {
			port, err := strconv.Atoi(portStr)
			if err != nil {
				return nil, err
			}
			endpoint.PersistentPort = port
		}
	} else {
		ttlStr, _ := readLine("Session TTL seconds [3600]: ")
		if ttlStr != "" {
			ttl, err := strconv.Atoi(ttlStr)
			if err != nil {
				return nil, err
			}
			endpoint.SessionTTLSeconds = ttl
		}
	}

	return &client.CreateSecretRequest{
		Name:           name,
		Type:           typ,
		Description:    description,
		Config:         cfg,
		Secret:         payload,
		EndpointConfig: endpoint,
	}, nil
}

func promptForConfig(typ client.SecretType) (map[string]any, error) {
	switch typ {
	case client.TypePostgres, client.TypeMySQL:
		host, _ := readLine("Host: ")
		portStr, _ := readLine("Port: ")
		port, _ := strconv.Atoi(portStr)
		user, _ := readLine("User: ")
		db, _ := readLine("Database: ")
		tls, _ := readLine("TLS mode [disable/prefer/require, default prefer]: ")
		if tls == "" {
			tls = "prefer"
		}
		return map[string]any{
			"host": host, "port": port, "user": user, "database": db, "tls_mode": tls,
		}, nil
	case client.TypeSSH:
		host, _ := readLine("Host: ")
		portStr, _ := readLine("Port [22]: ")
		port, _ := strconv.Atoi(portStr)
		if port == 0 {
			port = 22
		}
		user, _ := readLine("User: ")
		fp, _ := readLine("Upstream host key fingerprint (SHA256:...): ")
		return map[string]any{
			"host": host, "port": port, "user": user, "host_key_fingerprint": fp,
		}, nil
	case client.TypeHTTP:
		upstream, _ := readLine("Upstream URL: ")
		return map[string]any{"upstream": upstream, "follow_redirects": true}, nil
	default:
		return nil, fmt.Errorf("unknown secret type %q", typ)
	}
}

func promptForSecretPayload(typ client.SecretType) (map[string]any, error) {
	switch typ {
	case client.TypePostgres, client.TypeMySQL:
		pw, err := readPassword("Database password: ", false)
		if err != nil {
			return nil, err
		}
		return map[string]any{"password": pw}, nil
	case client.TypeSSH:
		method, _ := readLine("Auth method [password/private_key]: ")
		switch method {
		case "password":
			pw, err := readPassword("SSH password: ", false)
			if err != nil {
				return nil, err
			}
			return map[string]any{"auth_method": "password", "password": pw}, nil
		case "private_key":
			path, _ := readLine("Path to private key PEM: ")
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			passphrase, _ := readPassword("Key passphrase (empty if none): ", false)
			out := map[string]any{
				"auth_method":     "private_key",
				"private_key_pem": string(data),
			}
			if passphrase != "" {
				out["passphrase"] = passphrase
			}
			return out, nil
		default:
			return nil, fmt.Errorf("invalid auth method %q", method)
		}
	case client.TypeHTTP:
		fmt.Fprintln(os.Stderr, "Enter HTTP injection rules. Header injection format: 'name=template'. Blank line to finish.")
		headers := map[string]string{}
		for {
			line, _ := readLine("Header: ")
			if line == "" {
				break
			}
			i := strings.IndexByte(line, '=')
			if i <= 0 {
				continue
			}
			headers[line[:i]] = line[i+1:]
		}
		values := map[string]string{}
		fmt.Fprintln(os.Stderr, "Enter values referenced by templates ({{ .key }}). Blank line to finish.")
		for {
			key, _ := readLine("Value key: ")
			if key == "" {
				break
			}
			val, err := readPassword(fmt.Sprintf("Value for %s: ", key), false)
			if err != nil {
				return nil, err
			}
			values[key] = val
		}
		return map[string]any{
			"inject": map[string]any{"headers": headers},
			"values": values,
		}, nil
	default:
		return nil, fmt.Errorf("unknown secret type %q", typ)
	}
}
