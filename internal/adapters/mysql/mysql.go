// Package mysql implements the MySQL wire-protocol adapter (Section 3.4.3).
//
// Like the Postgres adapter, the handshake is driven by the spec's "connect
// upstream first, then drive client handshake" pattern. After both handshakes
// complete, the adapter byte-copies traffic in both directions. MySQL packet
// sequence numbers reset per command, so byte-level proxying is correct for
// the steady state.
package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	gomysql "github.com/go-mysql-org/go-mysql/client"
	"github.com/go-mysql-org/go-mysql/server"

	"github.com/Chekunin/cloak/internal/adapters"
	"github.com/Chekunin/cloak/internal/store"
)

// Adapter is the MySQL adapter.
type Adapter struct{}

// New returns a ready-to-register adapter.
func New() *Adapter { return &Adapter{} }

// Type returns store.TypeMySQL.
func (a *Adapter) Type() store.SecretType { return store.TypeMySQL }

// Config is the non-secret portion.
type Config struct {
	Host              string `json:"host"`
	Port              int    `json:"port"`
	User              string `json:"user"`
	Database          string `json:"database"`
	TLSMode           string `json:"tls_mode"`
	SSHTunnelSecretID string `json:"ssh_tunnel_secret_id,omitempty"`
}

// Payload is the secret portion.
type Payload struct {
	Password string `json:"password"`
}

func decodeConfig(m map[string]any) (Config, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, err
	}
	if c.Port == 0 {
		c.Port = 3306
	}
	if c.TLSMode == "" {
		c.TLSMode = "prefer"
	}
	return c, nil
}

// ValidateConfig implements adapters.Adapter.
func (a *Adapter) ValidateConfig(config map[string]any, secret map[string]any) error {
	c, err := decodeConfig(config)
	if err != nil {
		return fmt.Errorf("mysql: config: %w", err)
	}
	if c.Host == "" || c.User == "" {
		return errors.New("mysql: host and user are required")
	}
	pw, _ := secret["password"].(string)
	if pw == "" {
		return errors.New("mysql: password is required")
	}
	return nil
}

// ServeConnection serves one accepted MySQL client connection.
func (a *Adapter) ServeConnection(ctx context.Context, client net.Conn, dec adapters.DecryptedSecret, localCreds adapters.LocalCredentials) error {
	cfg, err := decodeConfig(dec.Config)
	if err != nil {
		return err
	}
	var payload Payload
	if err := json.Unmarshal(dec.Payload.Bytes(), &payload); err != nil {
		return fmt.Errorf("mysql: payload: %w", err)
	}

	// Open upstream first.
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	upConn, err := gomysql.ConnectWithContext(ctx, addr, cfg.User, payload.Password, cfg.Database, 30*time.Second)
	if err != nil {
		return fmt.Errorf("mysql: upstream: %w", err)
	}

	// Drive the client handshake with local creds.
	localPass := ""
	if localCreds.Password != nil {
		localPass = localCreds.Password.String()
	}
	_, err = server.NewConn(client, localCreds.Username, localPass, server.EmptyHandler{})
	if err != nil {
		_ = upConn.Close()
		return fmt.Errorf("mysql: client handshake: %w", err)
	}

	// Extract the raw net.Conn from the upstream wrapper and proxy.
	upstreamRaw := upConn.Conn.Conn
	_, err = adapters.Proxy(ctx, client, upstreamRaw)
	return err
}

// ConnectionString returns a mysql:// URL.
func (a *Adapter) ConnectionString(localAddr string, dec adapters.DecryptedSecret, creds adapters.LocalCredentials) string {
	cfg, _ := decodeConfig(dec.Config)
	host, port := splitHostPort(localAddr)
	user, pass := credPair(creds)
	return fmt.Sprintf("mysql://%s:%s@%s:%s/%s", user, pass, host, port, cfg.Database)
}

// EnvVars returns MYSQL_URL and MYSQL_* helpers.
func (a *Adapter) EnvVars(localAddr string, dec adapters.DecryptedSecret, creds adapters.LocalCredentials, prefix string) map[string]string {
	cfg, _ := decodeConfig(dec.Config)
	host, port := splitHostPort(localAddr)
	user, pass := credPair(creds)
	url := fmt.Sprintf("mysql://%s:%s@%s:%s/%s", user, pass, host, port, cfg.Database)
	out := map[string]string{
		"MYSQL_URL":      url,
		prefix + "_URL":  url,
		"MYSQL_HOST":     host,
		"MYSQL_PORT":     port,
		"MYSQL_USER":     user,
		"MYSQL_PASSWORD": pass,
		"MYSQL_PWD":      pass,
		"MYSQL_DATABASE": cfg.Database,
	}
	return out
}

func credPair(creds adapters.LocalCredentials) (user, pass string) {
	user = creds.Username
	if creds.Password != nil {
		pass = creds.Password.String()
	}
	return
}

func splitHostPort(addr string) (string, string) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, ""
	}
	return host, port
}
