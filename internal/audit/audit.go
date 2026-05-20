// Package audit implements the append-only, hash-chained JSONL audit log
// described in Section 3.6 of the specification. The log captures *metadata*
// about every meaningful event in the daemon (vault state transitions, secret
// CRUD, endpoint lifecycle, connection events). It never contains payload
// data such as SQL queries or HTTP bodies.
package audit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event types. Adding to this list is fine; renaming is a breaking change for
// downstream log readers.
const (
	EventVaultUnlocked   = "vault.unlocked"
	EventVaultLocked     = "vault.locked"
	EventVaultAutoLocked = "vault.auto_locked"
	EventSecretCreated   = "secret.created"
	EventSecretUpdated   = "secret.updated"
	EventSecretDeleted   = "secret.deleted"
	// Reveal of decrypted secret material to a human client, gated behind a
	// master-password re-check. RevealDenied is a failed gate attempt. Logged
	// with the secret name only — never the material itself.
	EventSecretRevealed     = "secret.revealed"
	EventSecretRevealDenied = "secret.reveal_denied"
	EventEndpointOpened  = "endpoint.opened"
	EventEndpointClosed  = "endpoint.closed"
	EventEndpointExpired = "endpoint.expired"
	// Materialized secrets (Section 16.7). Logged with variable names only —
	// never values.
	EventSecretMaterialized   = "secret.materialized"
	EventSecretUnmaterialized = "secret.unmaterialized"
	EventConnOpened           = "endpoint.connection.opened"
	EventConnClosed           = "endpoint.connection.closed"
	EventConnUpstreamFail     = "endpoint.connection.upstream_failed"
	EventTokenCreated         = "token.created"
	EventTokenRevoked         = "token.revoked"
	EventClientAuthOK         = "client.authenticated"
	EventClientAuthFailed     = "client.auth_failed"
)

// Client identifies the caller responsible for the event (when applicable).
type Client struct {
	TokenID string `json:"token_id,omitempty"`
	Name    string `json:"name,omitempty"`
	PID     int    `json:"pid,omitempty"`
}

// Entry is a single audit-log line.
type Entry struct {
	Timestamp  time.Time      `json:"ts"`
	Seq        uint64         `json:"seq"`
	PrevHash   string         `json:"prev_hash"`
	Event      string         `json:"event"`
	Client     *Client        `json:"client,omitempty"`
	SecretID   string         `json:"secret_id,omitempty"`
	SecretName string         `json:"secret_name,omitempty"`
	EndpointID string         `json:"endpoint_id,omitempty"`
	RemoteAddr string         `json:"remote_addr,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
}

// Logger appends Entry records to a JSONL file with a SHA-256 hash chain
// connecting each entry to its predecessor. Safe for concurrent use.
type Logger struct {
	mu       sync.Mutex
	path     string
	file     *os.File
	seq      uint64
	prevHash string
}

const zeroHash = "sha256:" + "0000000000000000000000000000000000000000000000000000000000000000"

// Open prepares a Logger at path, creating the file if needed and replaying it
// to recover the chain tip (seq + prev_hash). Directory must already exist.
func Open(path string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("audit: mkdir: %w", err)
	}
	seq, prev, err := scanTail(path)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("audit: open: %w", err)
	}
	return &Logger{path: path, file: f, seq: seq, prevHash: prev}, nil
}

// Close flushes and releases the file handle.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

// Write appends an Entry, filling in Timestamp/Seq/PrevHash automatically.
// The caller may pre-populate Details and the identification fields.
func (l *Logger) Write(e Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return errors.New("audit: logger is closed")
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	l.seq++
	e.Seq = l.seq
	e.PrevHash = l.prevHash
	if e.PrevHash == "" {
		e.PrevHash = zeroHash
	}
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("audit: marshal: %w", err)
	}
	if _, err := l.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("audit: write: %w", err)
	}
	if err := l.file.Sync(); err != nil {
		return fmt.Errorf("audit: sync: %w", err)
	}
	l.prevHash = hashLine(data)
	return nil
}

// Tail returns up to n most-recent entries (oldest first). Filters are applied
// in the caller; this method just reads.
func (l *Logger) Tail(n int) ([]Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return readAll(l.path, n)
}

// Path returns the on-disk path of the log file.
func (l *Logger) Path() string { return l.path }

func hashLine(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func scanTail(path string) (seq uint64, prev string, err error) {
	prev = zeroHash
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, zeroHash, nil
		}
		return 0, "", fmt.Errorf("audit: open for replay: %w", err)
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, 1<<16)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := line
			if trimmed[len(trimmed)-1] == '\n' {
				trimmed = trimmed[:len(trimmed)-1]
			}
			if len(trimmed) == 0 {
				if err == io.EOF {
					break
				}
				continue
			}
			var e Entry
			if jerr := json.Unmarshal(trimmed, &e); jerr != nil {
				return 0, "", fmt.Errorf("audit: corrupt entry near seq %d: %w", seq, jerr)
			}
			seq = e.Seq
			prev = hashLine(trimmed)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return 0, "", fmt.Errorf("audit: replay: %w", err)
		}
	}
	return seq, prev, nil
}

func readAll(path string, limit int) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Entry{}, nil
		}
		return nil, err
	}
	defer f.Close()
	// Non-nil empty slice so JSON encodes as `[]`, not `null`.
	out := []Entry{}
	br := bufio.NewReaderSize(f, 1<<16)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := line
			if trimmed[len(trimmed)-1] == '\n' {
				trimmed = trimmed[:len(trimmed)-1]
			}
			if len(trimmed) > 0 {
				var e Entry
				if jerr := json.Unmarshal(trimmed, &e); jerr == nil {
					out = append(out, e)
				}
			}
		}
		if err != nil {
			break
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}
