package store

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
	"golang.org/x/crypto/argon2"

	"github.com/Chekunin/cloak/internal/errs"
)

// Client token Argon2id parameters. Lighter than the master-password KDF
// because tokens are random 32-byte secrets to begin with; the hash is purely
// defense-in-depth against database theft.
const (
	tokenKDFTime      uint32 = 1
	tokenKDFMemoryKiB uint32 = 32 * 1024
	tokenKDFThreads   uint8  = 2
	tokenKDFKeyLen    uint32 = 32
	tokenSaltLen      int    = 16
	tokenSecretLen    int    = 32
)

// CreateToken issues a new client token. The plaintext token is returned
// **once**; the database stores only its Argon2id hash.
func (s *Store) CreateToken(name string) (id, plaintext string, err error) {
	raw := make([]byte, tokenSecretLen)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	salt := make([]byte, tokenSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", "", err
	}
	hash := argon2.IDKey(raw, salt, tokenKDFTime, tokenKDFMemoryKiB, tokenKDFThreads, tokenKDFKeyLen)
	tokID := ulid.Make().String()
	now := time.Now().UTC().UnixNano()
	_, err = s.db.Exec(`
		INSERT INTO client_tokens (id, name, token_hash, token_salt, created_at)
		VALUES (?, ?, ?, ?, ?)`, tokID, name, hash, salt, now)
	if err != nil {
		return "", "", errs.Wrap(errs.CodeInternalError, err)
	}
	plain := tokID + "." + base64.RawURLEncoding.EncodeToString(raw)
	return tokID, plain, nil
}

// VerifyToken parses a token string of the form "<id>.<base64-secret>",
// recomputes the Argon2id hash with the stored salt, and constant-time
// compares. Returns the matching TokenRecord on success.
func (s *Store) VerifyToken(token string) (*TokenRecord, error) {
	id, secret, ok := splitToken(token)
	if !ok {
		return nil, errs.New(errs.CodeUnauthorized, "malformed token")
	}
	rec, hash, salt, err := s.fetchTokenForVerify(id)
	if err != nil {
		return nil, err
	}
	if rec.Revoked {
		return nil, errs.New(errs.CodeUnauthorized, "token has been revoked")
	}
	candidate := argon2.IDKey(secret, salt, tokenKDFTime, tokenKDFMemoryKiB, tokenKDFThreads, tokenKDFKeyLen)
	if subtle.ConstantTimeCompare(candidate, hash) != 1 {
		return nil, errs.New(errs.CodeUnauthorized, "invalid token")
	}
	now := time.Now().UTC().UnixNano()
	_, _ = s.db.Exec(`UPDATE client_tokens SET last_seen_at=? WHERE id=?`, now, rec.ID)
	t := time.Unix(0, now).UTC()
	rec.LastSeenAt = &t
	return rec, nil
}

// ListTokens returns metadata for all tokens, sorted by created_at ascending.
func (s *Store) ListTokens() ([]TokenRecord, error) {
	rows, err := s.db.Query(`
		SELECT id, name, created_at, last_seen_at, revoked
		FROM client_tokens ORDER BY created_at`)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternalError, err)
	}
	defer rows.Close()
	var out []TokenRecord
	for rows.Next() {
		var (
			rec        TokenRecord
			createdAt  int64
			lastSeenNS sql.NullInt64
			revoked    int
		)
		if err := rows.Scan(&rec.ID, &rec.Name, &createdAt, &lastSeenNS, &revoked); err != nil {
			return nil, err
		}
		rec.CreatedAt = time.Unix(0, createdAt).UTC()
		if lastSeenNS.Valid {
			t := time.Unix(0, lastSeenNS.Int64).UTC()
			rec.LastSeenAt = &t
		}
		rec.Revoked = revoked != 0
		out = append(out, rec)
	}
	return out, rows.Err()
}

// RevokeToken marks a token as revoked. Idempotent.
func (s *Store) RevokeToken(id string) error {
	res, err := s.db.Exec(`UPDATE client_tokens SET revoked=1 WHERE id=?`, id)
	if err != nil {
		return errs.Wrap(errs.CodeInternalError, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errs.Newf(errs.CodeNotFound, "no token %q", id)
	}
	return nil
}

func (s *Store) fetchTokenForVerify(id string) (*TokenRecord, []byte, []byte, error) {
	var (
		rec        TokenRecord
		hash, salt []byte
		createdAt  int64
		lastSeenNS sql.NullInt64
		revoked    int
	)
	err := s.db.QueryRow(`
		SELECT id, name, token_hash, token_salt, created_at, last_seen_at, revoked
		FROM client_tokens WHERE id=?`, id).
		Scan(&rec.ID, &rec.Name, &hash, &salt, &createdAt, &lastSeenNS, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, errs.New(errs.CodeUnauthorized, "unknown token")
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("store: fetch token: %w", err)
	}
	rec.CreatedAt = time.Unix(0, createdAt).UTC()
	if lastSeenNS.Valid {
		t := time.Unix(0, lastSeenNS.Int64).UTC()
		rec.LastSeenAt = &t
	}
	rec.Revoked = revoked != 0
	return &rec, hash, salt, nil
}

func splitToken(token string) (id string, secret []byte, ok bool) {
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			raw, err := base64.RawURLEncoding.DecodeString(token[i+1:])
			if err != nil {
				return "", nil, false
			}
			return token[:i], raw, true
		}
	}
	return "", nil, false
}
