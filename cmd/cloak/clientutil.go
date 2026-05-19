package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Chekunin/cloak/internal/paths"
	"github.com/Chekunin/cloak/pkg/client"
)

// EnvToken overrides the on-disk token. Useful for scripts.
const EnvToken = "CLOAK_TOKEN"

// dial connects to the daemon. authenticate=true also runs `hello` using
// the locally-stored CLI token (or CLOAK_TOKEN env var).
func dial(ctx context.Context, authenticate bool) (*client.Client, paths.Paths, error) {
	p, err := paths.Default()
	if err != nil {
		return nil, p, err
	}
	c, err := client.Dial(ctx, p.SocketPath())
	if err != nil {
		return nil, p, fmt.Errorf("daemon unreachable at %s — is `cloak daemon start` running? (%w)", p.SocketPath(), err)
	}
	if !authenticate {
		return c, p, nil
	}
	tok, err := loadToken(p)
	if err != nil {
		_ = c.Close()
		return nil, p, err
	}
	if err := c.Authenticate(ctx, tok); err != nil {
		_ = c.Close()
		return nil, p, fmt.Errorf("authentication failed: %w", err)
	}
	return c, p, nil
}

// dialBackground returns a Client and a context for follow-up RPCs.
//
// The dial itself (network connect + optional `hello`) uses a short 30s
// deadline so a hung daemon doesn't block the CLI forever. The returned
// context has *no* deadline — interactive commands (init, unlock, secret add,
// secret rotate) may sit on the connection for minutes while the user types,
// and we don't want subsequent RPCs to fail with i/o timeout once the dial
// deadline has elapsed.
func dialBackground(authenticate bool) (*client.Client, paths.Paths, context.Context, context.CancelFunc, error) {
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer dialCancel()
	c, p, err := dial(dialCtx, authenticate)
	if err != nil {
		return nil, paths.Paths{}, nil, nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return c, p, ctx, cancel, nil
}

// loadToken returns the CLI client token. Resolution order:
//  1. $CLOAK_TOKEN
//  2. ~/.cloak/cli_token
func loadToken(p paths.Paths) (string, error) {
	if env := os.Getenv(EnvToken); env != "" {
		return env, nil
	}
	data, err := os.ReadFile(filepath.Join(p.Home, "cli_token"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("no CLI token configured — run `cloak token create --name <name>` and save the printed token to %s or export %s", filepath.Join(p.Home, "cli_token"), EnvToken)
		}
		return "", err
	}
	return string(trimNewline(data)), nil
}

func saveToken(p paths.Paths, token string) error {
	return os.WriteFile(filepath.Join(p.Home, "cli_token"), []byte(token+"\n"), 0o600)
}

func trimNewline(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

// emit prints either the JSON form of v or the textFunc rendering, based on
// the --json flag.
func emit(v any, textFunc func()) {
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(v)
		return
	}
	textFunc()
}
