package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Chekunin/cloak/internal/adapters"
	"github.com/Chekunin/cloak/internal/audit"
	"github.com/Chekunin/cloak/internal/endpoints"
	"github.com/Chekunin/cloak/internal/errs"
	"github.com/Chekunin/cloak/internal/secrets"
	"github.com/Chekunin/cloak/internal/store"
	"github.com/Chekunin/cloak/internal/vault"
)

// Deps bundles the runtime objects every IPC handler needs. Pass to RegisterAll.
type Deps struct {
	Vault     *vault.Manager
	Store     *store.Store
	Endpoints *endpoints.Manager
	Audit     *audit.Logger
	Adapters  *adapters.Registry
}

// RegisterAll wires every v1 RPC method into the server.
func RegisterAll(s *Server, deps Deps) {
	// Authentication.
	s.Register("hello", helloHandler(deps), false)

	// Vault. These four are reachable on a fresh install with no tokens
	// yet, so they do not require auth. The Unix socket is already restricted
	// to mode 0600, so only the daemon owner can talk to it.
	s.Register("vault.init", vaultInitHandler(deps), false)
	s.Register("vault.unlock", vaultUnlockHandler(deps), false)
	s.Register("vault.lock", vaultLockHandler(deps), false)
	s.Register("vault.status", vaultStatusHandler(deps), false)

	// Secrets.
	s.Register("secrets.list", secretsListHandler(deps), true)
	s.Register("secrets.get", secretsGetHandler(deps), true)
	s.Register("secrets.create", secretsCreateHandler(deps), true)
	s.Register("secrets.update", secretsUpdateHandler(deps), true)
	s.Register("secrets.delete", secretsDeleteHandler(deps), true)

	// Endpoints.
	s.Register("endpoints.list", endpointsListHandler(deps), true)
	s.Register("endpoints.open", endpointsOpenHandler(deps), true)
	s.Register("endpoints.close", endpointsCloseHandler(deps), true)
	s.Register("endpoints.refresh", endpointsRefreshHandler(deps), true)

	// Tokens. tokens.create is unauthenticated only when no tokens exist yet
	// (bootstrap path). The handler itself enforces this — the socket is
	// already 0600 so only the daemon owner can reach it.
	s.Register("tokens.create", tokensCreateHandler(deps), false)
	s.Register("tokens.list", tokensListHandler(deps), true)
	s.Register("tokens.revoke", tokensRevokeHandler(deps), true)

	// Audit.
	s.Register("audit.tail", auditTailHandler(deps), true)
}

// --- types for params/results ---

type helloParams struct {
	ClientToken string `json:"client_token"`
}

type helloResult struct {
	OK    bool   `json:"ok"`
	Token string `json:"token_id"`
	Name  string `json:"token_name"`
}

type vaultInitParams struct {
	Password string `json:"password"`
}

type vaultUnlockParams struct {
	Password     string `json:"password"`
	UnlockMethod string `json:"unlock_method,omitempty"`
}

type vaultStatusResult struct {
	State          string    `json:"state"`
	IdleTimeoutSec int       `json:"idle_timeout_sec"`
	ExpiresAt      time.Time `json:"expires_at,omitempty"`
	EndpointsOpen  int       `json:"endpoints_open"`
}

type secretsCreateParams struct {
	Name           string                `json:"name"`
	Type           store.SecretType      `json:"type"`
	Description    string                `json:"description"`
	Config         map[string]any        `json:"config"`
	Secret         map[string]any        `json:"secret"`
	EndpointConfig *store.EndpointConfig `json:"endpoint_config,omitempty"`
}

type secretsUpdateParams struct {
	IDOrName       string                `json:"id_or_name"`
	Description    *string               `json:"description,omitempty"`
	Config         map[string]any        `json:"config,omitempty"`
	Secret         map[string]any        `json:"secret,omitempty"`
	EndpointConfig *store.EndpointConfig `json:"endpoint_config,omitempty"`
}

type secretsDeleteParams struct {
	IDOrName string `json:"id_or_name"`
}

type secretsGetParams struct {
	IDOrName string `json:"id_or_name"`
}

type endpointOpenParams struct {
	SecretIDOrName string `json:"secret_id"`
	TTLSeconds     int    `json:"ttl_seconds,omitempty"`
}

type endpointCloseParams struct {
	EndpointID string `json:"endpoint_id"`
}

type endpointRefreshParams struct {
	EndpointID string `json:"endpoint_id"`
	TTLSeconds int    `json:"ttl_seconds,omitempty"`
}

type tokenCreateParams struct {
	Name string `json:"name"`
}

type tokenCreateResult struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Token string `json:"token"` // shown once
}

type tokenRevokeParams struct {
	ID string `json:"id"`
}

type auditTailParams struct {
	Limit int `json:"limit"`
}

// --- handlers ---

func helloHandler(d Deps) HandlerFunc {
	return func(ctx context.Context, sess *Session, raw json.RawMessage) (any, error) {
		var p helloParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if p.ClientToken == "" {
			return nil, errs.New(errs.CodeUnauthorized, "client_token is required")
		}
		rec, err := d.Store.VerifyToken(p.ClientToken)
		if err != nil {
			_ = d.Audit.Write(audit.Entry{
				Event:  audit.EventClientAuthFailed,
				Client: &audit.Client{PID: sess.PID()},
			})
			return nil, err
		}
		sess.Authenticate(rec.ID, rec.Name)
		_ = d.Audit.Write(audit.Entry{
			Event:  audit.EventClientAuthOK,
			Client: &audit.Client{TokenID: rec.ID, Name: rec.Name, PID: sess.PID()},
		})
		return helloResult{OK: true, Token: rec.ID, Name: rec.Name}, nil
	}
}

func vaultInitHandler(d Deps) HandlerFunc {
	return func(ctx context.Context, sess *Session, raw json.RawMessage) (any, error) {
		var p vaultInitParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if p.Password == "" {
			return nil, errs.New(errs.CodeInvalidRequest, "password is required")
		}
		pw := secrets.NewFromString(p.Password)
		defer pw.Zero()
		if err := d.Vault.Init(pw); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	}
}

func vaultUnlockHandler(d Deps) HandlerFunc {
	return func(ctx context.Context, sess *Session, raw json.RawMessage) (any, error) {
		var p vaultUnlockParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if p.UnlockMethod != "" && p.UnlockMethod != "password" {
			return nil, errs.Newf(errs.CodeInvalidRequest, "unlock_method %q not supported in v1", p.UnlockMethod)
		}
		if p.Password == "" {
			return nil, errs.New(errs.CodeInvalidRequest, "password is required")
		}
		pw := secrets.NewFromString(p.Password)
		defer pw.Zero()
		if err := d.Vault.Unlock(pw); err != nil {
			return nil, err
		}
		_ = d.Audit.Write(audit.Entry{
			Event:  audit.EventVaultUnlocked,
			Client: &audit.Client{TokenID: sess.TokenID(), Name: sess.TokenName(), PID: sess.PID()},
		})
		// Auto-open persistent endpoints.
		go func() {
			if err := d.Endpoints.OpenAllPersistent(context.Background()); err != nil {
				_ = d.Audit.Write(audit.Entry{
					Event:   audit.EventEndpointClosed,
					Details: map[string]any{"error": err.Error(), "phase": "auto_open_all"},
				})
			}
		}()
		return map[string]any{"ok": true, "expires_at": d.Vault.ExpiresAt()}, nil
	}
}

func vaultLockHandler(d Deps) HandlerFunc {
	return func(ctx context.Context, sess *Session, _ json.RawMessage) (any, error) {
		d.Vault.Lock(vault.LockReasonExplicit)
		_ = d.Audit.Write(audit.Entry{
			Event:  audit.EventVaultLocked,
			Client: &audit.Client{TokenID: sess.TokenID(), Name: sess.TokenName(), PID: sess.PID()},
		})
		return map[string]any{"ok": true}, nil
	}
}

func vaultStatusHandler(d Deps) HandlerFunc {
	return func(ctx context.Context, _ *Session, _ json.RawMessage) (any, error) {
		return vaultStatusResult{
			State:          d.Vault.State().String(),
			IdleTimeoutSec: int(d.Vault.IdleTimeout() / time.Second),
			ExpiresAt:      d.Vault.ExpiresAt(),
			EndpointsOpen:  len(d.Endpoints.List()),
		}, nil
	}
}

func secretsListHandler(d Deps) HandlerFunc {
	return func(_ context.Context, _ *Session, _ json.RawMessage) (any, error) {
		recs, err := d.Store.ListSecrets()
		if err != nil {
			return nil, err
		}
		return map[string]any{"secrets": recs}, nil
	}
}

func secretsGetHandler(d Deps) HandlerFunc {
	return func(_ context.Context, _ *Session, raw json.RawMessage) (any, error) {
		var p secretsGetParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		rec, err := d.Store.GetSecret(p.IDOrName)
		if err != nil {
			return nil, err
		}
		return rec, nil
	}
}

func secretsCreateHandler(d Deps) HandlerFunc {
	return func(_ context.Context, sess *Session, raw json.RawMessage) (any, error) {
		var p secretsCreateParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if !p.Type.IsKnown() {
			return nil, errs.Newf(errs.CodeInvalidRequest, "unknown secret type %q", p.Type)
		}
		adapter, err := d.Adapters.Get(p.Type)
		if err != nil {
			return nil, errs.Wrap(errs.CodeAdapterError, err)
		}
		if err := adapter.ValidateConfig(p.Config, p.Secret); err != nil {
			return nil, errs.Wrap(errs.CodeInvalidRequest, err)
		}
		// Materialized secrets have no listener, so they cannot be persistent
		// (Section 16.2.4).
		if adapter.Kind() == adapters.KindMaterialized {
			mode := store.ModeSession
			if p.EndpointConfig != nil && p.EndpointConfig.Mode != "" {
				mode = p.EndpointConfig.Mode
			}
			if mode == store.ModePersistent {
				return nil, errs.Newf(errs.CodeInvalidRequest, "%s secrets cannot use persistent endpoint mode", p.Type)
			}
		}
		payload, err := json.Marshal(p.Secret)
		if err != nil {
			return nil, errs.Wrap(errs.CodeInvalidRequest, err)
		}
		ep := store.EndpointConfig{}
		if p.EndpointConfig != nil {
			ep = *p.EndpointConfig
		}
		rec, err := d.Store.CreateSecret(p.Name, p.Type, p.Description, p.Config, ep, payload)
		if err != nil {
			return nil, err
		}
		_ = d.Audit.Write(audit.Entry{
			Event:      audit.EventSecretCreated,
			Client:     &audit.Client{TokenID: sess.TokenID(), Name: sess.TokenName(), PID: sess.PID()},
			SecretID:   rec.ID,
			SecretName: rec.Name,
			Details:    map[string]any{"type": string(rec.Type), "mode": string(rec.EndpointConfig.Mode)},
		})
		// Auto-open if persistent and vault is unlocked.
		if rec.EndpointConfig.Mode == store.ModePersistent && d.Vault.State() == vault.StateUnlocked {
			go func() {
				_, _ = d.Endpoints.Open(context.Background(), rec.ID, 0)
			}()
		}
		return rec, nil
	}
}

func secretsUpdateHandler(d Deps) HandlerFunc {
	return func(_ context.Context, sess *Session, raw json.RawMessage) (any, error) {
		var p secretsUpdateParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		req := store.UpdateRequest{
			Description:    p.Description,
			Config:         p.Config,
			EndpointConfig: p.EndpointConfig,
		}
		if p.Secret != nil {
			payload, err := json.Marshal(p.Secret)
			if err != nil {
				return nil, errs.Wrap(errs.CodeInvalidRequest, err)
			}
			req.Payload = payload
		}
		rec, err := d.Store.UpdateSecret(p.IDOrName, req)
		if err != nil {
			return nil, err
		}
		_ = d.Audit.Write(audit.Entry{
			Event:      audit.EventSecretUpdated,
			Client:     &audit.Client{TokenID: sess.TokenID(), Name: sess.TokenName(), PID: sess.PID()},
			SecretID:   rec.ID,
			SecretName: rec.Name,
		})
		return rec, nil
	}
}

func secretsDeleteHandler(d Deps) HandlerFunc {
	return func(_ context.Context, sess *Session, raw json.RawMessage) (any, error) {
		var p secretsDeleteParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		rec, err := d.Store.GetSecret(p.IDOrName)
		if err != nil {
			return nil, err
		}
		// Close any open endpoint first.
		_ = d.Endpoints.Close(rec.ID)
		if err := d.Store.DeleteSecret(p.IDOrName); err != nil {
			return nil, err
		}
		_ = d.Audit.Write(audit.Entry{
			Event:      audit.EventSecretDeleted,
			Client:     &audit.Client{TokenID: sess.TokenID(), Name: sess.TokenName(), PID: sess.PID()},
			SecretID:   rec.ID,
			SecretName: rec.Name,
		})
		return map[string]any{"ok": true}, nil
	}
}

func endpointsListHandler(d Deps) HandlerFunc {
	return func(_ context.Context, _ *Session, _ json.RawMessage) (any, error) {
		return map[string]any{"endpoints": d.Endpoints.List()}, nil
	}
}

func endpointsOpenHandler(d Deps) HandlerFunc {
	return func(ctx context.Context, _ *Session, raw json.RawMessage) (any, error) {
		var p endpointOpenParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if d.Vault.State() != vault.StateUnlocked {
			return nil, errs.New(errs.CodeVaultLocked, "vault is locked").WithHint("Run `cloak unlock` first.")
		}
		snap, err := d.Endpoints.Open(ctx, p.SecretIDOrName, p.TTLSeconds)
		if err != nil {
			return nil, err
		}
		return snap, nil
	}
}

func endpointsCloseHandler(d Deps) HandlerFunc {
	return func(_ context.Context, _ *Session, raw json.RawMessage) (any, error) {
		var p endpointCloseParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if err := d.Endpoints.Close(p.EndpointID); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	}
}

func endpointsRefreshHandler(d Deps) HandlerFunc {
	return func(_ context.Context, _ *Session, raw json.RawMessage) (any, error) {
		var p endpointRefreshParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		snap, err := d.Endpoints.Refresh(p.EndpointID, p.TTLSeconds)
		if err != nil {
			return nil, err
		}
		return snap, nil
	}
}

func tokensCreateHandler(d Deps) HandlerFunc {
	return func(_ context.Context, sess *Session, raw json.RawMessage) (any, error) {
		var p tokenCreateParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if p.Name == "" {
			return nil, errs.New(errs.CodeInvalidRequest, "name is required")
		}
		// Bootstrap rule: only the first token can be created without auth.
		if !sess.IsAuthenticated() {
			toks, err := d.Store.ListTokens()
			if err != nil {
				return nil, errs.Wrap(errs.CodeInternalError, err)
			}
			activeExists := false
			for _, t := range toks {
				if !t.Revoked {
					activeExists = true
					break
				}
			}
			if activeExists {
				return nil, errs.New(errs.CodeUnauthorized, "authenticate first (send `hello` with a client token)")
			}
		}
		id, tok, err := d.Store.CreateToken(p.Name)
		if err != nil {
			return nil, err
		}
		_ = d.Audit.Write(audit.Entry{
			Event:  audit.EventTokenCreated,
			Client: &audit.Client{TokenID: sess.TokenID(), Name: sess.TokenName(), PID: sess.PID()},
			Details: map[string]any{
				"new_token_id":   id,
				"new_token_name": p.Name,
			},
		})
		return tokenCreateResult{ID: id, Name: p.Name, Token: tok}, nil
	}
}

func tokensListHandler(d Deps) HandlerFunc {
	return func(_ context.Context, _ *Session, _ json.RawMessage) (any, error) {
		toks, err := d.Store.ListTokens()
		if err != nil {
			return nil, err
		}
		return map[string]any{"tokens": toks}, nil
	}
}

func tokensRevokeHandler(d Deps) HandlerFunc {
	return func(_ context.Context, sess *Session, raw json.RawMessage) (any, error) {
		var p tokenRevokeParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if err := d.Store.RevokeToken(p.ID); err != nil {
			return nil, err
		}
		_ = d.Audit.Write(audit.Entry{
			Event:   audit.EventTokenRevoked,
			Client:  &audit.Client{TokenID: sess.TokenID(), Name: sess.TokenName(), PID: sess.PID()},
			Details: map[string]any{"revoked_token_id": p.ID},
		})
		return map[string]any{"ok": true}, nil
	}
}

func auditTailHandler(d Deps) HandlerFunc {
	return func(_ context.Context, _ *Session, raw json.RawMessage) (any, error) {
		var p auditTailParams
		if len(raw) > 0 {
			if err := decodeParams(raw, &p); err != nil {
				return nil, err
			}
		}
		if p.Limit <= 0 {
			p.Limit = 100
		}
		entries, err := d.Audit.Tail(p.Limit)
		if err != nil {
			return nil, errs.Wrap(errs.CodeInternalError, err)
		}
		return map[string]any{"entries": entries}, nil
	}
}

// decodeParams unmarshals params into out. Empty params is treated as {}.
func decodeParams(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		var unmarshalErr *json.UnmarshalTypeError
		if errors.As(err, &unmarshalErr) {
			return errs.Newf(errs.CodeInvalidRequest, "invalid params: %s", err.Error())
		}
		return errs.Wrap(errs.CodeInvalidRequest, fmt.Errorf("invalid params: %w", err))
	}
	return nil
}
