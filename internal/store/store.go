// Package store wraps the encrypted SQLite database holding secrets and
// client tokens. Field-level encryption goes through the vault.Manager passed
// in at construction time; the store itself owns no key material.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	_ "modernc.org/sqlite"

	"github.com/Chekunin/cloak/internal/errs"
	"github.com/Chekunin/cloak/internal/vault"
)

// AAD strings bind ciphertexts to their column.
const (
	aadSecretPayload = "cloak.secret.payload.v1"
)

// Store is the SQLite-backed secret repository.
type Store struct {
	db    *sql.DB
	vault *vault.Manager
}

// Open opens (or creates) the database at path. Idempotent: safe to call on an
// existing file; schema is migrated automatically.
func Open(path string, v *vault.Manager) (*Store, error) {
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: schema: %w", err)
	}
	return &Store{db: db, vault: v}, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// CreateSecret inserts a new secret. payload is the JSON document described
// per-type in Section 3.2 of the spec; it is encrypted with the vault DEK and
// stored as secret_blob. The caller may zero payload after this returns.
func (s *Store) CreateSecret(name string, t SecretType, description string,
	config map[string]any, endpoint EndpointConfig, payload []byte,
) (*SecretRecord, error) {
	if !t.IsKnown() {
		return nil, errs.Newf(errs.CodeInvalidRequest, "unknown secret type %q", string(t))
	}
	if strings.TrimSpace(name) == "" {
		return nil, errs.New(errs.CodeInvalidRequest, "name is required")
	}
	cfgJSON, err := json.Marshal(config)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInvalidRequest, err)
	}
	endpoint = endpoint.WithDefaults()
	epJSON, err := json.Marshal(endpoint)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, err)
	}
	blob, err := s.vault.Encrypt(payload, []byte(aadSecretPayload))
	if err != nil {
		return nil, err
	}
	id := ulid.Make().String()
	now := time.Now().UTC()
	_, err = s.db.Exec(`
		INSERT INTO secrets (id, name, type, description, config_json, secret_blob, endpoint_config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, name, string(t), description, string(cfgJSON), blob, string(epJSON), now.UnixNano(), now.UnixNano())
	if err != nil {
		if isUniqueViolation(err) {
			return nil, errs.Newf(errs.CodeNameConflict, "secret %q already exists", name)
		}
		return nil, errs.Wrap(errs.CodeInternalError, err)
	}
	rec := &SecretRecord{
		ID:             id,
		Name:           name,
		Type:           t,
		Description:    description,
		Config:         config,
		EndpointConfig: endpoint,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return rec, nil
}

// UpdateSecret applies a set of partial updates to an existing secret. nil
// values mean "leave alone".
type UpdateRequest struct {
	Description    *string
	Config         map[string]any
	EndpointConfig *EndpointConfig
	Payload        []byte // if non-nil, replace secret_blob
}

func (s *Store) UpdateSecret(idOrName string, req UpdateRequest) (*SecretRecord, error) {
	rec, err := s.lookup(idOrName)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if req.Description != nil {
		rec.Description = *req.Description
	}
	if req.Config != nil {
		rec.Config = req.Config
	}
	if req.EndpointConfig != nil {
		rec.EndpointConfig = req.EndpointConfig.WithDefaults()
	}
	cfgJSON, _ := json.Marshal(rec.Config)
	epJSON, _ := json.Marshal(rec.EndpointConfig)

	if req.Payload != nil {
		blob, err := s.vault.Encrypt(req.Payload, []byte(aadSecretPayload))
		if err != nil {
			return nil, err
		}
		_, err = s.db.Exec(`
			UPDATE secrets SET description=?, config_json=?, endpoint_config=?, secret_blob=?, updated_at=?
			WHERE id=?`,
			rec.Description, string(cfgJSON), string(epJSON), blob, now.UnixNano(), rec.ID)
		if err != nil {
			return nil, errs.Wrap(errs.CodeInternalError, err)
		}
	} else {
		_, err = s.db.Exec(`
			UPDATE secrets SET description=?, config_json=?, endpoint_config=?, updated_at=?
			WHERE id=?`,
			rec.Description, string(cfgJSON), string(epJSON), now.UnixNano(), rec.ID)
		if err != nil {
			return nil, errs.Wrap(errs.CodeInternalError, err)
		}
	}
	rec.UpdatedAt = now
	return rec, nil
}

// DeleteSecret removes a secret by id or name.
func (s *Store) DeleteSecret(idOrName string) error {
	rec, err := s.lookup(idOrName)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM secrets WHERE id=?`, rec.ID); err != nil {
		return errs.Wrap(errs.CodeInternalError, err)
	}
	return nil
}

// GetSecret returns metadata for a single secret. Does not decrypt.
func (s *Store) GetSecret(idOrName string) (*SecretRecord, error) {
	return s.lookup(idOrName)
}

// ListSecrets returns metadata for all secrets, sorted by name.
func (s *Store) ListSecrets() ([]SecretRecord, error) {
	rows, err := s.db.Query(`
		SELECT id, name, type, description, config_json, endpoint_config, created_at, updated_at, last_used_at
		FROM secrets ORDER BY name`)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, err)
	}
	defer rows.Close()
	// Non-nil empty slice so JSON encodes as `[]`, not `null`. Wire contract
	// matters: the Rust client decodes the response into `Vec<Secret>` which
	// rejects `null`.
	out := []SecretRecord{}
	for rows.Next() {
		rec, err := scanSecret(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rec)
	}
	return out, rows.Err()
}

// DecryptSecret returns the secret material for use by an adapter. The caller
// must Zero the returned SecretBytes when done.
func (s *Store) DecryptSecret(idOrName string) (*SecretMaterial, error) {
	rec, err := s.lookup(idOrName)
	if err != nil {
		return nil, err
	}
	var blob []byte
	err = s.db.QueryRow(`SELECT secret_blob FROM secrets WHERE id=?`, rec.ID).Scan(&blob)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, err)
	}
	plain, err := s.vault.Decrypt(blob, []byte(aadSecretPayload))
	if err != nil {
		return nil, err
	}
	return &SecretMaterial{Record: *rec, Payload: plain}, nil
}

// MarkUsed updates last_used_at to now.
func (s *Store) MarkUsed(id string) {
	now := time.Now().UTC().UnixNano()
	_, _ = s.db.Exec(`UPDATE secrets SET last_used_at=? WHERE id=?`, now, id)
}

// SetMeta upserts a key/value into the meta table. Used for persisting things
// like assigned persistent ports across restarts.
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// GetMeta reads a meta value. Returns "" if absent.
func (s *Store) GetMeta(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

// DB returns the underlying *sql.DB. Used by sibling files (tokens) in this package.
func (s *Store) DB() *sql.DB { return s.db }


// lookup resolves an id-or-name to a SecretRecord. Tries id first, then name,
// to give id precedence when (unusually) a name happens to match another row's id.
func (s *Store) lookup(idOrName string) (*SecretRecord, error) {
	const q = `SELECT id, name, type, description, config_json, endpoint_config,
		created_at, updated_at, last_used_at FROM secrets WHERE id=?`
	row := s.db.QueryRow(q, idOrName)
	rec, err := scanSecret(row)
	if errors.Is(err, sql.ErrNoRows) {
		const qn = `SELECT id, name, type, description, config_json, endpoint_config,
			created_at, updated_at, last_used_at FROM secrets WHERE name=?`
		row = s.db.QueryRow(qn, idOrName)
		rec, err = scanSecret(row)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.Newf(errs.CodeNotFound, "no secret %q", idOrName)
		}
	}
	if err != nil {
		return nil, err
	}
	return rec, nil
}

// rowScanner is the intersection of *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dst ...any) error
}

func scanSecret(r rowScanner) (*SecretRecord, error) {
	var (
		rec        SecretRecord
		typ        string
		descNS     sql.NullString
		cfgJSON    string
		epJSON     string
		createdAt  int64
		updatedAt  int64
		lastUsedNS sql.NullInt64
	)
	if err := r.Scan(&rec.ID, &rec.Name, &typ, &descNS, &cfgJSON, &epJSON, &createdAt, &updatedAt, &lastUsedNS); err != nil {
		return nil, err
	}
	rec.Type = SecretType(typ)
	if descNS.Valid {
		rec.Description = descNS.String
	}
	rec.CreatedAt = time.Unix(0, createdAt).UTC()
	rec.UpdatedAt = time.Unix(0, updatedAt).UTC()
	if lastUsedNS.Valid {
		t := time.Unix(0, lastUsedNS.Int64).UTC()
		rec.LastUsedAt = &t
	}
	if err := json.Unmarshal([]byte(cfgJSON), &rec.Config); err != nil {
		return nil, fmt.Errorf("store: corrupt config_json: %w", err)
	}
	if err := json.Unmarshal([]byte(epJSON), &rec.EndpointConfig); err != nil {
		return nil, fmt.Errorf("store: corrupt endpoint_config: %w", err)
	}
	rec.EndpointConfig = rec.EndpointConfig.WithDefaults()
	return &rec, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "constraint failed: UNIQUE")
}

// ensureDir creates dir 0700 if missing.
func ensureDir(dir string) error {
	return ensureDir700(dir)
}
