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

// AdapterKind tells the endpoint manager which lifecycle a secret type
// follows. See Section 16.1.
type AdapterKind int

const (
	// KindProxy adapters open a 127.0.0.1 listener and proxy a protocol. The
	// real secret never reaches the client.
	KindProxy AdapterKind = iota
	// KindMaterialized adapters have no listener; they decrypt the stored
	// values and inject them into a child process. The real secret does reach
	// the client. This is the weaker tier.
	KindMaterialized
)

// Adapter is the common contract every secret type implements. Concrete
// adapters additionally implement either ProxyAdapter or MaterializingAdapter
// depending on Kind().
type Adapter interface {
	Type() store.SecretType

	// Kind reports whether this adapter is proxied or materialized.
	Kind() AdapterKind

	// ValidateConfig is called before persisting a new secret. It must return
	// nil on success. config is the public, non-secret portion; secret is the
	// secret payload (the JSON document defined in Section 3.2 / 16.2).
	ValidateConfig(config map[string]any, secret map[string]any) error
}

// ProxyAdapter is the per-protocol contract for KindProxy adapters
// (postgres, mysql, ssh, http).
type ProxyAdapter interface {
	Adapter

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

// MaterializingAdapter is the contract for KindMaterialized adapters (env).
type MaterializingAdapter interface {
	Adapter

	// Materialize decrypts the secret into the values to deliver to a child
	// process. It does not touch the filesystem — the endpoint manager owns
	// file writing and cleanup (Section 16.4.4).
	Materialize(decoded DecryptedSecret) (Materialization, error)
}

// Materialization is what a MaterializingAdapter produces from a decrypted
// secret.
type Materialization struct {
	// Env is the set of environment variables to inject (real secret values).
	// May be empty when the secret only renders files.
	Env map[string]string
	// Files are file bodies the manager must write before returning.
	Files []RenderedFile
}

// RenderedFile is one file the manager writes to the per-materialization run
// directory.
type RenderedFile struct {
	// Basename is the file name within the run directory.
	Basename string
	// PathEnv, if non-empty, is an env var set to the written file's path.
	PathEnv string
	// Content is the file body. The manager zeroes it after writing.
	Content *secrets.SecretBytes
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
