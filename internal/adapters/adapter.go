// Package adapters defines the Adapter interface (Section 3.4) and the
// shared types used by per-protocol adapter packages.
//
// An Adapter knows how to:
//   - validate a secret's config + payload before persisting,
//   - serve an accepted client connection on a 127.0.0.1 listener by
//     authenticating the client, connecting upstream, and proxying bytes,
//   - render a ready-to-use connection string and env vars.
package adapters

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/Chekunin/cloak/internal/secrets"
	"github.com/Chekunin/cloak/internal/store"
)

// Adapter is the per-protocol contract.
type Adapter interface {
	Type() store.SecretType

	// ValidateConfig is called before persisting a new secret. It must return
	// nil on success. config is the public, non-secret portion; secret is the
	// secret payload (the JSON document defined in Section 3.2).
	ValidateConfig(config map[string]any, secret map[string]any) error

	// ServeConnection handles a single accepted client connection. Returns
	// when the connection closes (either side) or ctx is cancelled.
	ServeConnection(ctx context.Context, client net.Conn, decoded DecryptedSecret, localCreds LocalCredentials) error

	// ConnectionString returns a ready-to-use URL (psql, mysql, ssh, http) for
	// the local endpoint.
	ConnectionString(localAddr string, decoded DecryptedSecret, localCreds LocalCredentials) string

	// EnvVars returns env-var name/value pairs to inject for `cloak exec`.
	// envPrefix is "<NAME>" (the secret name normalized to upper case) or "".
	EnvVars(localAddr string, decoded DecryptedSecret, localCreds LocalCredentials, envPrefix string) map[string]string
}

// DecryptedSecret is passed to Adapter.ServeConnection. The Payload field
// contains the JSON-encoded secret material — adapters should immediately
// json.Unmarshal it into their type-specific struct and zero the SecretBytes
// in a defer.
type DecryptedSecret struct {
	ID     string
	Name   string
	Type   store.SecretType
	Config map[string]any
	// Payload is the decrypted JSON document. Callers (the endpoint manager)
	// own its lifetime and Zero it when they are done.
	Payload *secrets.SecretBytes
}

// LocalCredentials are the ephemeral username/password the client uses to
// authenticate against the local endpoint listener.
type LocalCredentials struct {
	Username string
	Password *secrets.SecretBytes
}

// Registry maps SecretType → Adapter implementation. Concrete adapters
// register themselves via Register.
type Registry struct {
	byType map[store.SecretType]Adapter
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{byType: map[store.SecretType]Adapter{}} }

// Register inserts a in the registry. Subsequent calls for the same type overwrite.
func (r *Registry) Register(a Adapter) { r.byType[a.Type()] = a }

// Get returns the adapter for t or an error if not registered.
func (r *Registry) Get(t store.SecretType) (Adapter, error) {
	a, ok := r.byType[t]
	if !ok {
		return nil, fmt.Errorf("adapters: no adapter for type %q", t)
	}
	return a, nil
}

// Types returns the registered types in arbitrary order.
func (r *Registry) Types() []store.SecretType {
	out := make([]store.SecretType, 0, len(r.byType))
	for t := range r.byType {
		out = append(out, t)
	}
	return out
}

// ErrLocalAuth is returned by adapters when client→endpoint authentication fails.
var ErrLocalAuth = errors.New("local authentication failed")

// ErrUpstream is returned (typically wrapped) when the upstream connection fails.
var ErrUpstream = errors.New("upstream error")
