// Package config loads ~/.cloak/config.toml into a strongly typed Config.
//
// Unspecified values are filled with conservative defaults. The idle timeout
// is clamped to a minimum of 5 minutes per the v1 security contract (it can
// never be disabled).
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/Chekunin/cloak/internal/paths"
)

// MinIdleTimeout is the smallest acceptable idle auto-lock interval.
const MinIdleTimeout = 5 * time.Minute

// Config mirrors the on-disk TOML schema (Section 10 of the spec).
type Config struct {
	Daemon    DaemonConfig    `toml:"daemon"`
	Vault     VaultConfig     `toml:"vault"`
	Endpoints EndpointsConfig `toml:"endpoints"`
	SSH       SSHConfig       `toml:"ssh"`
	Audit     AuditConfig     `toml:"audit"`
}

type DaemonConfig struct {
	SocketPath string `toml:"socket_path"`
	LogLevel   string `toml:"log_level"`
}

type VaultConfig struct {
	IdleTimeout duration `toml:"idle_timeout"`
}

type EndpointsConfig struct {
	DefaultPersistentPortStart int `toml:"default_persistent_port_start"`
}

type SSHConfig struct {
	HostKeyDir string `toml:"host_key_dir"`
}

type AuditConfig struct {
	LogPath string `toml:"log_path"`
}

// duration is a TOML-friendly time.Duration that accepts strings like "1h".
type duration struct{ time.Duration }

func (d *duration) UnmarshalText(text []byte) error {
	v, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

// Default returns a Config populated with v1 defaults, resolved against p.
func Default(p paths.Paths) Config {
	return Config{
		Daemon: DaemonConfig{
			SocketPath: p.SocketPath(),
			LogLevel:   "info",
		},
		Vault: VaultConfig{IdleTimeout: duration{Duration: time.Hour}},
		Endpoints: EndpointsConfig{
			DefaultPersistentPortStart: 54200,
		},
		SSH:   SSHConfig{HostKeyDir: p.HostKeysDir()},
		Audit: AuditConfig{LogPath: p.AuditLog()},
	}
}

// Load reads p.ConfigFile() and overlays it on the defaults. Missing file is
// not an error — defaults apply.
func Load(p paths.Paths) (Config, error) {
	cfg := Default(p)
	data, err := os.ReadFile(p.ConfigFile())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg.normalize(), nil
		}
		return Config{}, fmt.Errorf("config: read %s: %w", p.ConfigFile(), err)
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("config: parse %s: %w", p.ConfigFile(), err)
	}
	return cfg.normalize(), nil
}

func (c Config) normalize() Config {
	if c.Vault.IdleTimeout.Duration < MinIdleTimeout {
		c.Vault.IdleTimeout.Duration = MinIdleTimeout
	}
	c.Daemon.SocketPath = expandOrPanic(c.Daemon.SocketPath)
	c.SSH.HostKeyDir = expandOrPanic(c.SSH.HostKeyDir)
	c.Audit.LogPath = expandOrPanic(c.Audit.LogPath)
	if c.Endpoints.DefaultPersistentPortStart == 0 {
		c.Endpoints.DefaultPersistentPortStart = 54200
	}
	if c.Daemon.LogLevel == "" {
		c.Daemon.LogLevel = "info"
	}
	return c
}

// IdleTimeout returns the resolved (clamped) idle auto-lock interval.
func (c Config) IdleTimeout() time.Duration { return c.Vault.IdleTimeout.Duration }

func expandOrPanic(p string) string {
	out, err := paths.Expand(p)
	if err != nil {
		return p
	}
	return out
}
