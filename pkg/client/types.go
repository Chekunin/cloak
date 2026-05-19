package client

import "time"

// SecretType matches the internal type. Defined here so external consumers do
// not depend on internal/store. Mirror in JSON shape exactly.
type SecretType string

const (
	TypeSSH      SecretType = "ssh"
	TypePostgres SecretType = "postgres"
	TypeMySQL    SecretType = "mysql"
	TypeHTTP     SecretType = "http"
)

// EndpointMode mirrors store.EndpointMode.
type EndpointMode string

const (
	ModePersistent EndpointMode = "persistent"
	ModeSession    EndpointMode = "session"
)

// EndpointConfig mirrors store.EndpointConfig.
type EndpointConfig struct {
	Mode                     EndpointMode `json:"mode"`
	PersistentPort           int          `json:"persistent_port,omitempty"`
	SessionTTLSeconds        int          `json:"session_ttl_seconds,omitempty"`
	RequireLocalAuth         bool         `json:"require_local_auth"`
	MaxConcurrentConnections int          `json:"max_concurrent_connections,omitempty"`
}

// Secret mirrors store.SecretRecord.
type Secret struct {
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

// Endpoint mirrors endpoints.EndpointSnapshot.
type Endpoint struct {
	ID               string            `json:"id"`
	SecretID         string            `json:"secret_id"`
	SecretName       string            `json:"secret_name"`
	Type             SecretType        `json:"type"`
	Mode             EndpointMode      `json:"mode"`
	LocalAddr        string            `json:"local_addr"`
	ConnectionString string            `json:"connection_string"`
	EnvVars          map[string]string `json:"env_vars,omitempty"`
	OpenedAt         time.Time         `json:"opened_at"`
	ExpiresAt        time.Time         `json:"expires_at,omitempty"`
	Stats            EndpointStats     `json:"stats"`
}

// EndpointStats mirrors endpoints.StatsSnapshot.
type EndpointStats struct {
	BytesIn          int64     `json:"bytes_in"`
	BytesOut         int64     `json:"bytes_out"`
	ConnectionsOpen  int64     `json:"connections_open"`
	ConnectionsTotal int64     `json:"connections_total"`
	LastActivity     time.Time `json:"last_activity,omitempty"`
}

// Token mirrors store.TokenRecord.
type Token struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	Revoked    bool       `json:"revoked"`
}
