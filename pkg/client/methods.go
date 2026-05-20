package client

import (
	"context"
	"time"
)

// Authenticate sends `hello` with clientToken. Subsequent calls inherit the
// authenticated state of the underlying connection.
func (c *Client) Authenticate(ctx context.Context, clientToken string) error {
	return c.Call(ctx, "hello", map[string]any{"client_token": clientToken}, nil)
}

// VaultInit creates a new vault with the given password. Idempotent only when
// the vault is already in the Uninitialized state.
func (c *Client) VaultInit(ctx context.Context, password string) error {
	return c.Call(ctx, "vault.init", map[string]any{"password": password}, nil)
}

// VaultUnlock unlocks the vault. unlockMethod is "password" in v1.
func (c *Client) VaultUnlock(ctx context.Context, password string) error {
	return c.Call(ctx, "vault.unlock", map[string]any{"password": password, "unlock_method": "password"}, nil)
}

// VaultLock locks the vault, closing all endpoints.
func (c *Client) VaultLock(ctx context.Context) error {
	return c.Call(ctx, "vault.lock", nil, nil)
}

// VaultStatus is the response shape of vault.status.
type VaultStatus struct {
	State          string    `json:"state"`
	IdleTimeoutSec int       `json:"idle_timeout_sec"`
	ExpiresAt      time.Time `json:"expires_at"`
	EndpointsOpen  int       `json:"endpoints_open"`
}

// VaultStatus returns the current state of the vault.
func (c *Client) VaultStatus(ctx context.Context) (*VaultStatus, error) {
	var s VaultStatus
	if err := c.Call(ctx, "vault.status", nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// CreateSecretRequest mirrors the daemon's secrets.create params.
type CreateSecretRequest struct {
	Name           string          `json:"name"`
	Type           SecretType      `json:"type"`
	Description    string          `json:"description,omitempty"`
	Config         map[string]any  `json:"config"`
	Secret         map[string]any  `json:"secret"`
	EndpointConfig *EndpointConfig `json:"endpoint_config,omitempty"`
}

// UpdateSecretRequest mirrors the daemon's secrets.update params.
type UpdateSecretRequest struct {
	IDOrName       string          `json:"id_or_name"`
	Description    *string         `json:"description,omitempty"`
	Config         map[string]any  `json:"config,omitempty"`
	Secret         map[string]any  `json:"secret,omitempty"`
	EndpointConfig *EndpointConfig `json:"endpoint_config,omitempty"`
}

// ListSecrets returns all stored secrets (metadata only).
func (c *Client) ListSecrets(ctx context.Context) ([]Secret, error) {
	var resp struct {
		Secrets []Secret `json:"secrets"`
	}
	if err := c.Call(ctx, "secrets.list", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Secrets, nil
}

// GetSecret returns one secret by id or name.
func (c *Client) GetSecret(ctx context.Context, idOrName string) (*Secret, error) {
	var rec Secret
	if err := c.Call(ctx, "secrets.get", map[string]any{"id_or_name": idOrName}, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// RevealedSecret is the response shape of secrets.reveal: the full picture of
// one secret. Config holds the non-secret connection metadata (host, port,
// user, ...); Secret holds the decrypted material (passwords, keys) — handle
// the latter with the same care as the vault itself.
type RevealedSecret struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Type   SecretType     `json:"type"`
	Config map[string]any `json:"config"`
	Secret map[string]any `json:"secret"`
}

// RevealSecret decrypts and returns the secret material for one secret. It
// requires the vault master password as a re-authentication gate; an
// authenticated client token alone is deliberately not sufficient.
func (c *Client) RevealSecret(ctx context.Context, idOrName, masterPassword string) (*RevealedSecret, error) {
	var r RevealedSecret
	if err := c.Call(ctx, "secrets.reveal",
		map[string]any{"id_or_name": idOrName, "password": masterPassword}, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// CreateSecret creates a new secret.
func (c *Client) CreateSecret(ctx context.Context, req CreateSecretRequest) (*Secret, error) {
	var rec Secret
	if err := c.Call(ctx, "secrets.create", req, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// UpdateSecret updates an existing secret.
func (c *Client) UpdateSecret(ctx context.Context, req UpdateSecretRequest) (*Secret, error) {
	var rec Secret
	if err := c.Call(ctx, "secrets.update", req, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// DeleteSecret removes a secret.
func (c *Client) DeleteSecret(ctx context.Context, idOrName string) error {
	return c.Call(ctx, "secrets.delete", map[string]any{"id_or_name": idOrName}, nil)
}

// ListEndpoints returns currently-open local endpoints.
func (c *Client) ListEndpoints(ctx context.Context) ([]Endpoint, error) {
	var resp struct {
		Endpoints []Endpoint `json:"endpoints"`
	}
	if err := c.Call(ctx, "endpoints.list", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Endpoints, nil
}

// OpenEndpoint opens a session endpoint for the named secret.
func (c *Client) OpenEndpoint(ctx context.Context, secretIDOrName string, ttlSeconds int) (*Endpoint, error) {
	var snap Endpoint
	if err := c.Call(ctx, "endpoints.open",
		map[string]any{"secret_id": secretIDOrName, "ttl_seconds": ttlSeconds}, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

// CloseEndpoint closes an open endpoint.
func (c *Client) CloseEndpoint(ctx context.Context, endpointID string) error {
	return c.Call(ctx, "endpoints.close", map[string]any{"endpoint_id": endpointID}, nil)
}

// RefreshEndpoint extends the TTL of a session endpoint.
func (c *Client) RefreshEndpoint(ctx context.Context, endpointID string, ttlSeconds int) (*Endpoint, error) {
	var snap Endpoint
	if err := c.Call(ctx, "endpoints.refresh",
		map[string]any{"endpoint_id": endpointID, "ttl_seconds": ttlSeconds}, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

// TokenInfo is returned by CreateToken (Token holds the *plaintext* token
// string, which the daemon shows exactly once).
type TokenInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Token string `json:"token"`
}

// CreateToken issues a new client token. The plaintext token is returned once.
func (c *Client) CreateToken(ctx context.Context, name string) (*TokenInfo, error) {
	var t TokenInfo
	if err := c.Call(ctx, "tokens.create", map[string]any{"name": name}, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// ListTokens returns all tokens (metadata only).
func (c *Client) ListTokens(ctx context.Context) ([]Token, error) {
	var resp struct {
		Tokens []Token `json:"tokens"`
	}
	if err := c.Call(ctx, "tokens.list", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Tokens, nil
}

// RevokeToken revokes a token by ID.
func (c *Client) RevokeToken(ctx context.Context, id string) error {
	return c.Call(ctx, "tokens.revoke", map[string]any{"id": id}, nil)
}

// AuditEntry is an opaque map representation of an audit.Entry.
type AuditEntry map[string]any

// AuditTail returns the latest audit entries (limit defaults to 100).
func (c *Client) AuditTail(ctx context.Context, limit int) ([]AuditEntry, error) {
	var resp struct {
		Entries []AuditEntry `json:"entries"`
	}
	if err := c.Call(ctx, "audit.tail", map[string]any{"limit": limit}, &resp); err != nil {
		return nil, err
	}
	return resp.Entries, nil
}
