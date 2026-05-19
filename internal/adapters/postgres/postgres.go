// Package postgres implements the Postgres wire-protocol adapter (Section 3.4.2).
//
// Strategy (per the spec's "cleaner alternative"):
//  1. Read the client's StartupMessage so we know the database name to advertise.
//  2. Connect upstream with pgconn — this handles TLS, SCRAM, MD5, cleartext
//     auth, and leaves us at ReadyForQuery.
//  3. Tell the client AuthenticationCleartextPassword and verify their reply
//     against the ephemeral local password in constant time.
//  4. Send the upstream's ParameterStatus/BackendKeyData messages we captured
//     followed by ReadyForQuery, then transparently proxy bytes.
package postgres

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"

	"github.com/Chekunin/cloak/internal/adapters"
	"github.com/Chekunin/cloak/internal/store"
)

// Adapter is the Postgres adapter.
type Adapter struct{}

// New returns a ready-to-register adapter.
func New() *Adapter { return &Adapter{} }

// Type returns store.TypePostgres.
func (a *Adapter) Type() store.SecretType { return store.TypePostgres }

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
		c.Port = 5432
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
		return fmt.Errorf("postgres: config: %w", err)
	}
	if c.Host == "" || c.User == "" || c.Database == "" {
		return errors.New("postgres: host, user, and database are required")
	}
	switch c.TLSMode {
	case "disable", "prefer", "require", "verify-ca", "verify-full":
	default:
		return fmt.Errorf("postgres: invalid tls_mode %q", c.TLSMode)
	}
	pw, _ := secret["password"].(string)
	if pw == "" {
		return errors.New("postgres: password is required")
	}
	return nil
}

// ServeConnection serves a single Postgres client.
func (a *Adapter) ServeConnection(ctx context.Context, client net.Conn, dec adapters.DecryptedSecret, localCreds adapters.LocalCredentials) error {
	cfg, err := decodeConfig(dec.Config)
	if err != nil {
		return err
	}
	var payload Payload
	if err := json.Unmarshal(dec.Payload.Bytes(), &payload); err != nil {
		return fmt.Errorf("postgres: payload: %w", err)
	}

	// Handle SSLRequest / GSSENCRequest by replying 'N' (no, plain TCP),
	// then build the Backend on a reader that includes any bytes we
	// peeked past those negotiations.
	reader, err := refuseSSLAndGSS(client)
	if err != nil {
		return fmt.Errorf("postgres: pre-startup: %w", err)
	}
	backend := pgproto3.NewBackend(reader, client)
	msg, err := backend.ReceiveStartupMessage()
	if err != nil {
		return fmt.Errorf("postgres: read startup: %w", err)
	}
	if _, ok := msg.(*pgproto3.StartupMessage); !ok {
		return fmt.Errorf("postgres: unexpected startup message %T", msg)
	}

	// Dial upstream and authenticate using stored credentials.
	upstreamConn, paramStatus, backendKey, txStatus, err := dialUpstream(ctx, cfg, payload)
	if err != nil {
		sendError(backend, "08006", "upstream connection failed")
		return fmt.Errorf("postgres: upstream: %w", err)
	}

	// Authenticate the client.
	if localCreds.Password != nil && localCreds.Password.Len() > 0 {
		if err := authenticateClient(backend, localCreds.Password.Bytes()); err != nil {
			_ = upstreamConn.Close()
			sendError(backend, "28P01", "invalid password")
			return adapters.ErrLocalAuth
		}
	} else {
		backend.Send(&pgproto3.AuthenticationOk{})
		if err := backend.Flush(); err != nil {
			_ = upstreamConn.Close()
			return err
		}
	}

	// Mirror the upstream's parameters / backend key, then signal ReadyForQuery.
	for k, v := range paramStatus {
		backend.Send(&pgproto3.ParameterStatus{Name: k, Value: v})
	}
	if backendKey != nil {
		backend.Send(backendKey)
	}
	if txStatus == 0 {
		txStatus = 'I'
	}
	backend.Send(&pgproto3.ReadyForQuery{TxStatus: txStatus})
	if err := backend.Flush(); err != nil {
		_ = upstreamConn.Close()
		return err
	}

	_, err = adapters.Proxy(ctx, client, upstreamConn)
	return err
}

// ConnectionString returns a postgresql:// URL clients can use directly.
func (a *Adapter) ConnectionString(localAddr string, dec adapters.DecryptedSecret, creds adapters.LocalCredentials) string {
	cfg, _ := decodeConfig(dec.Config)
	user, pass := credPair(creds)
	host, port := splitHostPort(localAddr)
	return fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, cfg.Database)
}

// EnvVars returns DATABASE_URL plus PG* helpers.
func (a *Adapter) EnvVars(localAddr string, dec adapters.DecryptedSecret, creds adapters.LocalCredentials, prefix string) map[string]string {
	cfg, _ := decodeConfig(dec.Config)
	user, pass := credPair(creds)
	host, port := splitHostPort(localAddr)
	url := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, cfg.Database)
	out := map[string]string{
		"DATABASE_URL":  url,
		prefix + "_URL": url,
		"PGHOST":        host,
		"PGPORT":        port,
		"PGUSER":        user,
		"PGPASSWORD":    pass,
		"PGDATABASE":    cfg.Database,
	}
	return out
}

// --- helpers ---

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

// refuseSSLAndGSS reads the first 8 bytes; if they describe an SSLRequest or
// GSSENCRequest (both are length=8 fixed-size messages), reply 'N' and keep
// looping. The first non-negotiation header is prefix-rewound into the
// returned io.Reader so the caller can feed it to pgproto3.Backend.
func refuseSSLAndGSS(conn net.Conn) (io.Reader, error) {
	for {
		var hdr [8]byte
		if _, err := io.ReadFull(conn, hdr[:]); err != nil {
			return nil, err
		}
		length := binary.BigEndian.Uint32(hdr[0:4])
		code := binary.BigEndian.Uint32(hdr[4:8])
		if length == 8 && (code == sslRequestCode || code == gssEncRequestCode) {
			if _, err := conn.Write([]byte{'N'}); err != nil {
				return nil, err
			}
			continue
		}
		// Real startup message — rewind these 8 bytes ahead of the conn.
		buf := make([]byte, 8)
		copy(buf, hdr[:])
		return io.MultiReader(bytes.NewReader(buf), conn), nil
	}
}

const (
	sslRequestCode    = 80877103
	gssEncRequestCode = 80877104
)

func authenticateClient(b *pgproto3.Backend, expected []byte) error {
	b.Send(&pgproto3.AuthenticationCleartextPassword{})
	if err := b.Flush(); err != nil {
		return err
	}
	if err := b.SetAuthType(pgproto3.AuthTypeCleartextPassword); err != nil {
		return err
	}
	msg, err := b.Receive()
	if err != nil {
		return err
	}
	pm, ok := msg.(*pgproto3.PasswordMessage)
	if !ok {
		return errors.New("postgres: expected PasswordMessage")
	}
	if subtle.ConstantTimeCompare([]byte(pm.Password), expected) != 1 {
		return adapters.ErrLocalAuth
	}
	b.Send(&pgproto3.AuthenticationOk{})
	return b.Flush()
}

func sendError(b *pgproto3.Backend, code, message string) {
	b.Send(&pgproto3.ErrorResponse{Severity: "FATAL", Code: code, Message: message})
	_ = b.Flush()
}

// dialUpstream connects to the upstream server, completes authentication, and
// captures the ParameterStatus / BackendKeyData / TxStatus values so we can
// mirror them to the client. Returns the underlying net.Conn (hijacked from
// pgconn).
func dialUpstream(ctx context.Context, cfg Config, payload Payload) (net.Conn, map[string]string, *pgproto3.BackendKeyData, byte, error) {
	connString := fmt.Sprintf("postgres://%s@%s:%d/%s?sslmode=%s",
		cfg.User, cfg.Host, cfg.Port, cfg.Database, cfg.TLSMode)
	pgCfg, err := pgconn.ParseConfig(connString)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	pgCfg.Password = payload.Password

	conn, err := pgconn.ConnectConfig(ctx, pgCfg)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	hijacked, err := conn.Hijack()
	if err != nil {
		_ = conn.Close(ctx)
		return nil, nil, nil, 0, err
	}
	bkd := &pgproto3.BackendKeyData{
		ProcessID: hijacked.PID,
		SecretKey: hijacked.SecretKey,
	}
	return hijacked.Conn, hijacked.ParameterStatuses, bkd, hijacked.TxStatus, nil
}
