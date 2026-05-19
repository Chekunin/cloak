// Package paths resolves filesystem locations for Cloak. All callers that need
// a file under the Cloak home should go through this package instead of
// rebuilding paths inline.
package paths

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	EnvHome   = "CLOAK_HOME"
	EnvConfig = "CLOAK_CONFIG"
)

// Paths resolves a Cloak home and exposes the well-known files within it.
type Paths struct {
	Home string
}

// Default returns the platform-appropriate Cloak home, honoring CLOAK_HOME if set.
func Default() (Paths, error) {
	if dir := os.Getenv(EnvHome); dir != "" {
		expanded, err := Expand(dir)
		if err != nil {
			return Paths{}, err
		}
		return Paths{Home: expanded}, nil
	}
	switch runtime.GOOS {
	case "windows":
		appdata := os.Getenv("APPDATA")
		if appdata == "" {
			return Paths{}, errors.New("paths: %APPDATA% is not set")
		}
		return Paths{Home: filepath.Join(appdata, "Cloak")}, nil
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return Paths{}, err
		}
		return Paths{Home: filepath.Join(home, ".cloak")}, nil
	}
}

// Expand resolves leading ~ and ~/ to the user's home directory.
func Expand(path string) (string, error) {
	if path == "" {
		return path, nil
	}
	if path == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

// EnsureHome creates the Cloak home directory (mode 0700) if it does not exist
// and returns it.
func (p Paths) EnsureHome() error {
	return os.MkdirAll(p.Home, 0o700)
}

// VaultDB is the path to the encrypted SQLite database.
func (p Paths) VaultDB() string { return filepath.Join(p.Home, "vault.db") }

// VaultMeta is the path to the (unencrypted) vault metadata file holding
// the KDF salt, wrapped DEK, format version, etc.
func (p Paths) VaultMeta() string { return filepath.Join(p.Home, "vault.meta.json") }

// ConfigFile is the path to config.toml. CLOAK_CONFIG overrides this directly.
func (p Paths) ConfigFile() string {
	if c := os.Getenv(EnvConfig); c != "" {
		expanded, err := Expand(c)
		if err == nil {
			return expanded
		}
		return c
	}
	return filepath.Join(p.Home, "config.toml")
}

// AuditLog is the JSONL audit log path.
func (p Paths) AuditLog() string { return filepath.Join(p.Home, "audit.log") }

// HostKeysDir is the directory containing SSH host keys for the SSH adapter.
func (p Paths) HostKeysDir() string { return filepath.Join(p.Home, "host_keys") }

// SocketPath returns the Unix domain socket the daemon listens on. Windows
// callers should treat this as a named-pipe identifier.
func (p Paths) SocketPath() string {
	if runtime.GOOS == "windows" {
		return `\\.\pipe\cloakd`
	}
	return filepath.Join(p.Home, "cloakd.sock")
}

// PIDFile is the path used to record the daemon PID for `cloak daemon status`.
func (p Paths) PIDFile() string { return filepath.Join(p.Home, "cloakd.pid") }

// DaemonLog is the path of the operational (zerolog) log file.
func (p Paths) DaemonLog() string { return filepath.Join(p.Home, "cloakd.log") }
