package store

import (
	"encoding/json"
	"time"

	"github.com/Chekunin/cloak/internal/secrets"
)

// SecretType is one of the v1 adapter types.
type SecretType string

const (
	TypeSSH      SecretType = "ssh"
	TypePostgres SecretType = "postgres"
	TypeMySQL    SecretType = "mysql"
	TypeHTTP     SecretType = "http"
	// TypeEnv is a materialized secret: it has no network listener. Its stored
	// key/value bag is injected into a child process as environment variables
	// and/or rendered files (Section 16).
	TypeEnv SecretType = "env"
)

// IsKnown reports whether t is a v1-recognized adapter type.
func (t SecretType) IsKnown() bool {
	switch t {
	case TypeSSH, TypePostgres, TypeMySQL, TypeHTTP, TypeEnv:
		return true
	}
	return false
}

// EndpointMode enumerates the two listener strategies.
type EndpointMode string

const (
	ModePersistent EndpointMode = "persistent"
	ModeSession    EndpointMode = "session"
)

// EndpointConfig is the per-secret listening configuration. Mirrors the JSON
// shape described in Section 3.2 of the spec.
type EndpointConfig struct {
	Mode                     EndpointMode `json:"mode"`
	PersistentPort           int          `json:"persistent_port,omitempty"`
	SessionTTLSeconds        int          `json:"session_ttl_seconds,omitempty"`
	RequireLocalAuth         bool         `json:"require_local_auth"`
	MaxConcurrentConnections int          `json:"max_concurrent_connections,omitempty"`
}

// Defaults applies v1 defaults to zero fields in-place and returns the value.
func (c EndpointConfig) WithDefaults() EndpointConfig {
	if c.Mode == "" {
		c.Mode = ModeSession
	}
	if c.SessionTTLSeconds == 0 && c.Mode == ModeSession {
		c.SessionTTLSeconds = 3600
	}
	if c.MaxConcurrentConnections == 0 {
		c.MaxConcurrentConnections = 16
	}
	return c
}

// SecretRecord is the metadata representation of a stored secret. The secret
// material itself is held in the encrypted SecretBlob field; non-secret config
// is in ConfigJSON.
type SecretRecord struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Type           SecretType     `json:"type"`
	Description    string         `json:"description,omitempty"`
	Config         map[string]any `json:"config"`
	EndpointConfig EndpointConfig `json:"endpoint_config"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	LastUsedAt     *time.Time     `json:"last_used_at,omitempty"`
}

// SecretMaterial pairs a SecretRecord with a decrypted payload (the JSON shape
// described per type in Section 3.2). The caller must Zero the payload when
// finished.
type SecretMaterial struct {
	Record  SecretRecord
	Payload *secrets.SecretBytes
}

// ParsePayload decodes the JSON payload into out. The caller owns the
// underlying SecretBytes; this method does not zero it.
func (m SecretMaterial) ParsePayload(out any) error {
	return json.Unmarshal(m.Payload.Bytes(), out)
}

// TokenRecord describes a client token (metadata only; the hash is internal).
type TokenRecord struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	Revoked    bool       `json:"revoked"`
}
