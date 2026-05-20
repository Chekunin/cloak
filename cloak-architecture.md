# Cloak v1 — Architecture Specification

## Context for the implementing LLM

You are implementing **Cloak**, a local secret broker. Cloak solves two related problems for a single audience (developers, including those using AI agents):

1. **Secrets in `.env` files leak.** Developers commit them to Git, paste them into chat, give file-system access to LLM agents that read them. Cloak replaces raw secrets in `.env` (and similar config locations) with references to **local network endpoints** that proxy to real services while keeping the actual credentials inside an encrypted vault.

2. **LLM agents need to use credentials without seeing them.** When Claude Code, Cursor, or a similar agent connects to a database, SSH server, or HTTP API, it should be able to use native tooling (`psql`, `ssh`, `curl`) against a local endpoint rather than ever receiving the real password.

The same mechanism — local protocol-aware proxies that authenticate upstream with stored credentials — serves both use cases.

**Naming conventions.** The product is **Cloak**. The CLI binary is `cloak`. The daemon binary is `cloakd` (standard Unix daemon-naming convention). User-facing paths use `~/.cloak/`. Environment variables use the `CLOAK_` prefix. In error messages and documentation, use the verb metaphor naturally: "cloak your secrets", "cloaked endpoint", "vault unlocked, endpoints cloaked". Do not over-use the metaphor; technical writing stays direct.

This document specifies the v1 implementation. Implement in **Go (1.22+)**. Prefer the standard library and a small, well-known dependency set. Keep the design minimal but make extensibility points explicit, because v2 will add inspection, policy enforcement, and an MCP server on top of the v1 foundation without rewriting the core.

---

## 1. Product Overview

Cloak is a long-running local daemon plus a CLI client. The daemon:

1. Stores credentials in a locally-encrypted database.
2. For each stored credential, can open a **local endpoint**: a `127.0.0.1:port` listener that speaks the credential's native protocol (Postgres wire, MySQL wire, SSH, HTTP).
3. Authenticates against the upstream service with the real credential. Proxies traffic transparently between the local client and the upstream.
4. Issues **ephemeral local credentials** for clients to authenticate against the endpoint itself, so the endpoint isn't open to any process on the machine without explicit grant.
5. Logs every connection event to an append-only audit log.
6. Auto-locks after idle timeout, closing all endpoints and zeroing all in-memory secret material.

### Two operating modes for endpoints

- **Persistent endpoints** open automatically on vault unlock, listen on a stable named port, and live until vault lock. Designed for `.env`-style use: an app reads `DATABASE_URL=postgresql://...@localhost:54200/...` and just works.
- **Session endpoints** are opened by an explicit CLI command or RPC call, listen on an ephemeral random port, and live for a bounded TTL. Designed for one-shot scripts, LLM agent sessions, and `cloak exec` wrappers.

### v1 scope: transparent proxying, no protocol inspection

v1 proxies byte-for-byte after authentication. It does **not** parse SQL queries, SSH commands, or HTTP request bodies. Access control in v1 is binary: an endpoint is open for the unlocked vault, or it isn't. This is a deliberate trade-off for v1 — protocol-level inspection and per-operation policy enforcement are scheduled for v2.

The defensive value v1 provides without inspection:

- Real secrets never reach the client (LLM or app).
- Real secrets never live on disk outside the encrypted vault.
- All connections go through Cloak and appear in the audit log.
- Credentials rotate in one place, not in every `.env`.
- Endpoints close when the vault locks.

### Non-goals for v1

- No GUI.
- No MCP server (deferred to v1.2; the IPC layer is designed to host one).
- No team sharing, cloud sync, or remote profile distribution.
- No biometric integration (deferred to v1.1).
- No sandboxing of clients connecting to endpoints.
- No protocol-level inspection or per-command policy enforcement.
- No request caching/mocking (deferred to v2).

---

## 2. High-Level Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│                          cloakd (daemon)                              │
│                                                                       │
│  ┌──────────────┐    ┌──────────────────┐    ┌───────────────────┐  │
│  │ IPC Server   │───▶│ Endpoint Manager │───▶│ Adapters          │  │
│  │ (unix sock,  │    │  - lifecycle     │    │ ┌──────┐ ┌──────┐│  │
│  │  JSON-RPC)   │    │  - listeners     │    │ │ ssh  │ │ pg   ││  │
│  └──────────────┘    │  - upstream pool │    │ ├──────┤ ├──────┤│  │
│         ▲             └──────────────────┘    │ │mysql │ │ http ││  │
│         │                      │              │ └──────┘ └──────┘│  │
│         │                      ▼              └───────────────────┘  │
│         │             ┌──────────────────┐             │             │
│         │             │  Secret Store    │◀────────────┘             │
│         │             │  (encrypted      │   (decrypt secret         │
│         │             │   SQLite)        │    on connect)            │
│         │             └──────────────────┘                            │
│         │                      ▲                                      │
│         │                      │                                      │
│         │             ┌──────────────────┐                            │
│         │             │  Vault Manager   │                            │
│         │             │  (KDF, DEK,      │                            │
│         │             │   auto-lock)     │                            │
│         │             └──────────────────┘                            │
│         │                                                             │
│         │             ┌──────────────────┐                            │
│         └────────────▶│  Audit Logger    │                            │
│                       │  (hash-chained   │                            │
│                       │   JSONL)         │                            │
│                       └──────────────────┘                            │
└──────────────────────────────────────────────────────────────────────┘
        ▲                                       │
        │ IPC                                   │ Endpoints listen on
        │                                       │ 127.0.0.1:<port>
   ┌────┴────────┐                              │
   │   cloak     │                              ▼
   │   (CLI)     │                  ┌───────────────────────┐
   └─────────────┘                  │ Local clients         │
                                    │  - psql / mysql       │
                                    │  - ssh / scp / sftp   │
                                    │  - curl / app code    │
                                    │  - LLM agents         │
                                    └───────┬───────────────┘
                                            │ proxies to
                                            ▼
                                    ┌───────────────────────┐
                                    │ Upstream services     │
                                    │ (real DBs, servers,   │
                                    │  APIs)                │
                                    └───────────────────────┘
```

Two binaries, sharing a Go module:

- `cloakd` — the daemon. Holds the unlocked vault, runs all endpoint listeners, owns the audit log.
- `cloak` — the CLI client. Stateless. Talks to `cloakd` over a Unix domain socket.

---

## 3. Component Specifications

### 3.1 Vault Manager

Responsible for the master key lifecycle and the lock/unlock state machine.

**Key hierarchy:**

- **Master password** — user-supplied, never persisted.
- **KEK (Key Encryption Key)** — derived from the master password via Argon2id (`time=3, memory=64 MiB, threads=4, keyLen=32`). The salt is stored in vault metadata.
- **DEK (Data Encryption Key)** — random 256-bit key generated on first vault creation. Stored on disk wrapped by the KEK using XChaCha20-Poly1305. Decrypted into memory on unlock.

Field-level encryption uses the DEK directly (AEAD with random nonce per field, nonce prepended to ciphertext).

**Memory protection:**

- A `SecretBytes` type wraps `[]byte`. Constructor allocates via `mlock`'d memory where the platform supports it. `Zero()` overwrites the buffer. All decrypted secret material in the codebase must use this type.
- The DEK lives in `SecretBytes` for the entire unlocked lifetime of the vault.
- Decrypted per-field secrets exist only inside an `Adapter.Connect()` call's stack and are zeroed in a `defer`.

**Lock state machine:**

- `Uninitialized` → after `cloak init`, transitions to `Locked`.
- `Locked` → after successful `unlock`, transitions to `Unlocked`.
- `Unlocked` → after `lock`, idle timeout expiry, or daemon shutdown, transitions to `Locked`. All open endpoints close, all upstream connections close, all `SecretBytes` zero out.

Idle timeout: default 1 hour, configurable, minimum 5 minutes (cannot be disabled in v1).

**Files (default paths):**

- `~/.cloak/vault.db` — encrypted SQLite (per-field encryption; full-DB encryption deferred to v1.1 with SQLCipher migration).
- `~/.cloak/vault.meta.json` — KDF salt, KDF parameters, wrapped DEK, vault format version, creation timestamp. Not encrypted (contains no secret material).
- `~/.cloak/config.toml` — daemon configuration: socket path, audit log path, idle timeout, default ports for persistent endpoints.
- `~/.cloak/audit.log` — append-only JSONL audit log.
- `~/.cloak/host_keys/` — directory of SSH host keys generated for Cloak's own SSH listener (one key per algorithm: ed25519, rsa).

Linux/macOS use `~/.cloak/`. Windows uses `%APPDATA%\Cloak\`. Abstract path resolution in the `paths` package.

### 3.2 Secret Store

Encrypted persistent storage for credentials.

**Implementation: plain SQLite (`modernc.org/sqlite`, pure-Go, no CGo) with per-field encryption.** Secret-bearing columns store AEAD-encrypted blobs. Non-sensitive columns (names, types, hosts, ports) are plaintext for query convenience.

This is weaker than full-database encryption (an attacker with disk access learns secret names, types, and hosts), but it eliminates CGo build complexity, supports trivial cross-compilation, and is acceptable for v1. SQLCipher migration is a v1.1 task and the schema is designed to support it.

**Schema:**

```sql
CREATE TABLE secrets (
    id                TEXT PRIMARY KEY,           -- ULID
    name              TEXT NOT NULL UNIQUE,       -- user-facing identifier
    type              TEXT NOT NULL,              -- "ssh" | "postgres" | "mysql" | "http"
    description       TEXT,
    config_json       TEXT NOT NULL,              -- non-secret config (host, port, ...)
    secret_blob       BLOB NOT NULL,              -- AEAD-encrypted JSON payload
    endpoint_config   TEXT NOT NULL,              -- listening mode, port, etc.
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    last_used_at      INTEGER
);

CREATE INDEX idx_secrets_name ON secrets(name);
CREATE INDEX idx_secrets_type ON secrets(type);

CREATE TABLE client_tokens (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,                   -- e.g. "claude-cli", "my-laptop-cursor"
    token_hash   BLOB NOT NULL,                   -- Argon2id hash
    created_at   INTEGER NOT NULL,
    last_seen_at INTEGER,
    revoked      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
```

**Encrypted payload shape per type** (the structure inside `secret_blob` after decryption):

- `ssh`:
  ```json
  {
    "auth_method": "private_key" | "password",
    "private_key_pem": "...",       // when private_key
    "passphrase": "...",            // optional, when private_key has one
    "password": "..."               // when password
  }
  ```
- `postgres`, `mysql`:
  ```json
  {
    "password": "..."
  }
  ```
- `http`:
  ```json
  {
    "inject": {
      "headers": {"Authorization": "Bearer {{ .api_key }}"},
      "query": {"key": "{{ .secret_key }}"}
    },
    "values": {
      "api_key": "sk_live_...",
      "secret_key": "..."
    }
  }
  ```

**Non-secret config_json shape per type:**

- `ssh`:
  ```json
  {
    "host": "prod-server.internal",
    "port": 22,
    "user": "deploy",
    "host_key_fingerprint": "SHA256:abc...",   // for upstream host verification
    "jump_host_secret_id": null                  // optional, for ProxyJump-style chaining (v1.x)
  }
  ```
- `postgres`, `mysql`:
  ```json
  {
    "host": "db.example.com",
    "port": 5432,
    "user": "app_user",
    "database": "app_db",
    "tls_mode": "require",                       // disable | prefer | require
    "ssh_tunnel_secret_id": null                 // optional
  }
  ```
- `http`:
  ```json
  {
    "upstream": "https://api.stripe.com",
    "follow_redirects": true,
    "strip_request_headers": ["X-Original-Host"]
  }
  ```

**Endpoint config per secret:**

```json
{
  "mode": "persistent" | "session",
  "persistent_port": 54200,                       // only when mode=persistent
  "session_ttl_seconds": 3600,                    // only when mode=session
  "require_local_auth": true,                     // ephemeral creds for client→endpoint auth
  "max_concurrent_connections": 16
}
```

### 3.3 Endpoint Manager

Central component owning the lifecycle of all local listeners.

**Responsibilities:**

- On vault unlock: read all secrets with `mode=persistent`, open their listeners.
- On vault lock: close every active listener, close every upstream connection, zero all transient secret material.
- Handle `endpoint.open` / `endpoint.close` / `endpoint.list` IPC requests.
- For each listener, track stats: bytes in/out, connections opened/closed, last activity, current concurrent connections.
- Enforce per-endpoint `max_concurrent_connections`.

**Internal data structures:**

```go
type EndpointManager struct {
    mu        sync.RWMutex
    active    map[string]*ActiveEndpoint  // keyed by secret ID
    listeners map[string]net.Listener      // keyed by secret ID
    vault     *VaultManager
    store     *SecretStore
    audit     *AuditLogger
}

type ActiveEndpoint struct {
    SecretID         string
    SecretName       string
    Type             string
    Mode             string                 // "persistent" | "session"
    Listener         net.Listener
    LocalAddr        string
    ConnectionString string                 // ready-to-use, e.g. "postgresql://kb_session:abc@127.0.0.1:54200/db"
    LocalCreds       LocalCredentials       // ephemeral auth for client→endpoint
    OpenedAt         time.Time
    ExpiresAt        time.Time              // for session mode
    Stats            EndpointStats
    cancel           context.CancelFunc
}

type LocalCredentials struct {
    Username string                          // e.g. "kb_session_01HX..."
    Password SecretBytes                     // ephemeral, random 24 bytes base64
}
```

**Listener lifecycle:**

1. On open, allocate port (stable for persistent, random for session), `net.Listen("tcp", "127.0.0.1:<port>")`.
2. Generate ephemeral local credentials if `require_local_auth=true`.
3. Spawn a goroutine that `Accept()`s in a loop, dispatching each connection to the adapter.
4. On `Close()`, stop accepting, close in-flight connections, wait for goroutines, close listener.

**Concurrency model:**

- One goroutine per listener accept loop.
- One goroutine per accepted client connection (plus an inverse goroutine for upstream-to-client direction in transparent proxies).
- All goroutines respect a context derived from the endpoint's context; cancelling the endpoint context cleanly tears everything down.

### 3.4 Adapters

Adapters know how to:

1. Open a listener on a TCP port speaking a specific protocol.
2. Authenticate incoming client connections (using ephemeral local credentials).
3. Open an upstream connection to the real service using the decrypted real credentials.
4. Proxy traffic between client and upstream.

**Adapter interface:**

```go
type Adapter interface {
    Type() string

    // Validates that the given configs are well-formed before saving the secret.
    // Returns nil on success or an error describing the problem.
    ValidateConfig(config map[string]any, secret map[string]any) error

    // ServeConnection handles a single accepted client connection.
    // It receives the client conn, the decrypted secret, the local credentials
    // that the client must authenticate with, and a context tied to the endpoint lifetime.
    // Returns when the connection closes (either side) or context is cancelled.
    ServeConnection(ctx context.Context, client net.Conn, secret DecryptedSecret, localCreds LocalCredentials) error

    // ConnectionString returns a ready-to-use URL for the endpoint
    // given the listening address and local credentials.
    // Examples:
    //   postgres: "postgresql://user:pass@127.0.0.1:54200/dbname"
    //   ssh:      "ssh://user@127.0.0.1:54201" (informational; users typically run ssh CLI)
    //   http:     "http://127.0.0.1:54100"
    ConnectionString(localAddr string, secret DecryptedSecret, localCreds LocalCredentials) string

    // EnvVars returns environment variable name → value mappings to inject
    // when `cloak exec` is used with this secret.
    // Examples:
    //   postgres → {"DATABASE_URL": "postgresql://..."}
    //   http     → {"<NAME>_URL": "http://127.0.0.1:54100"}
    EnvVars(localAddr string, secret DecryptedSecret, localCreds LocalCredentials, envPrefix string) map[string]string
}

type DecryptedSecret struct {
    ID     string
    Name   string
    Type   string
    Config map[string]any                // non-secret
    Secret SecretBytes                   // decrypted JSON payload; zeroed by caller after use
}
```

The four v1 adapters:

#### 3.4.1 HTTP adapter

Simplest of the four. Uses `net/http` + `httputil.ReverseProxy`.

**Listening side:** plain HTTP on the listener address (no TLS for v1; v1.x adds optional local TLS for clients that refuse plaintext upstream).

**Client authentication:** if `require_local_auth=true`, expect a header `Authorization: Bearer <local_password>` (or accept `X-Cloak-Token`). Reject with 401 otherwise.

**Request handling:**

1. Strip the local-auth header.
2. Apply `strip_request_headers` from config.
3. Resolve injection templates against `values` in the decrypted secret. Add headers, query params.
4. Rewrite target URL: `upstream` + original request path + (merged) query string.
5. Forward request via `httputil.ReverseProxy`.
6. Stream response back to the client.

**Connection string:** `http://127.0.0.1:<port>` (clients add their own paths). When `require_local_auth=true`, Cloak inserts the token via the URL fragment for documentation purposes and via env var `<NAME>_TOKEN` for `cloak exec`.

**Env vars for `cloak exec`:**
- `<NAME>_URL=http://127.0.0.1:<port>`
- `<NAME>_TOKEN=<local_password>` (if local auth enabled)

**Implementation notes:**

- Templating uses `text/template` with `{{ .field_name }}` syntax against the `values` map.
- Body is streamed; do not buffer entire bodies.
- Set a configurable max body size for safety (default 10 MiB; configurable later).
- `Host` header rewriting follows `ReverseProxy` defaults.

#### 3.4.2 Postgres adapter

Acts as a Postgres wire protocol server toward the client and a Postgres client toward the upstream. Uses `github.com/jackc/pgx/v5/pgconn` for the upstream connection and `github.com/jackc/pgx/v5/pgproto3` for parsing the client-facing handshake.

**Listening side handshake:**

1. Read client's `StartupMessage`. Ignore most fields, but respect `user` and `database` for the response.
2. Respond with `AuthenticationCleartextPassword` (simplest; require local TLS to avoid sending the local password over plain TCP — but since this is `127.0.0.1`, plain is acceptable in v1).
3. Read client's `PasswordMessage`. Compare to `localCreds.Password` in constant time. On mismatch, send `ErrorResponse` and close.
4. Send `AuthenticationOk`, `ParameterStatus` messages (mirrored from the upstream after we connect, or sensible defaults), `BackendKeyData`, `ReadyForQuery`.

**Upstream connection:**

1. Build a `pgconn.Config` from `config_json` (host, port, user, database, tls_mode) and the decrypted password.
2. If `ssh_tunnel_secret_id` is set, first establish an SSH tunnel via that secret and dial the upstream through the tunnel.
3. Call `pgconn.ConnectConfig(ctx, config)`. This handles TLS, SCRAM/MD5/cleartext upstream auth, all of it.
4. Hijack the underlying `net.Conn` from the `pgconn.PgConn` (via `pgconn.Hijack()`) for raw proxying.

**Proxying:**

After both handshakes complete, run two goroutines: `io.Copy(client, upstream)` and `io.Copy(upstream, client)`. Close both when either direction finishes. No protocol awareness past the handshake.

**Caveat: the `ReadyForQuery` we sent to the client may not match the upstream's session state.** A clean way to handle this: after authenticating the client, before doing `io.Copy`, do one round trip where we issue a no-op query upstream (`SELECT 1`) to drain any pending state and then send a fresh `ReadyForQuery` to the client. In practice, since we connect upstream lazily right after the client handshake, the upstream is already at `ReadyForQuery` and we can just relay its messages from this point.

A slightly cleaner alternative: do the upstream connection **first**, then drive the client handshake, sending the upstream's actual `ParameterStatus` and `BackendKeyData` to the client. This makes the proxied connection truly indistinguishable from a direct one. I recommend this order.

**Connection string:** `postgresql://<localCreds.Username>:<localCreds.Password>@127.0.0.1:<port>/<config.database>?sslmode=disable`.

**Env vars for `cloak exec`:**
- `DATABASE_URL=postgresql://...` (if secret name matches a standard pattern; otherwise `<NAME>_URL`).
- Optionally: `PGHOST`, `PGPORT`, `PGUSER`, `PGPASSWORD`, `PGDATABASE` for tools that read them.

#### 3.4.3 MySQL adapter

Structurally identical to Postgres, with MySQL wire protocol. Use `github.com/go-mysql-org/go-mysql/server` for the listening side and `github.com/go-sql-driver/mysql` (via raw `net.Conn` for proxying) for the upstream.

Same approach: complete upstream connection first, then drive the client handshake using credentials we control, then `io.Copy` both directions.

**Env vars:** `MYSQL_URL` or `<NAME>_URL`, plus `MYSQL_HOST`, `MYSQL_PORT`, `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_DATABASE`.

#### 3.4.4 SSH adapter

The most complex of the four. Cloak runs an SSH server on the listening port using `golang.org/x/crypto/ssh`, accepts authenticated client connections, and proxies channels to an upstream SSH connection.

**Server-side configuration:**

- Host keys loaded from `~/.cloak/host_keys/` (generated on first daemon start if missing: one ed25519 key, one RSA 3072-bit key).
- Authentication: `PasswordCallback` validating against `localCreds.Password` in constant time. Public-key auth is **not supported in v1** to keep the model simple.
- Allowed authentications advertised to client: `password` only.

**On accepted SSH connection from client:**

1. Open upstream SSH connection using `ssh.Dial("tcp", host:port, &ssh.ClientConfig{...})`:
   - `User`: from `config.user`.
   - `Auth`: from decrypted secret (`PublicKeys` from PEM, or `Password`).
   - `HostKeyCallback`: validate against `config.host_key_fingerprint`. Reject on mismatch.
2. Once upstream is connected, accept channels on the client side and proxy them.

**Channel proxying:**

For each new client-side channel request:

- **`session` channels** (interactive shell or remote command execution): open a corresponding `session` channel upstream, then proxy stdin/stdout/stderr in both directions, plus relay `pty-req`, `shell`, `exec`, `subsystem`, `window-change`, `signal`, `exit-status`, `exit-signal` requests. `subsystem` covers SFTP.
- **`direct-tcpip` channels** (port forwarding from client through SSH): open the corresponding upstream channel, proxy bytes. Optional in v1 — if not implemented, reject these requests with a clear error.
- **`forwarded-tcpip` channels** (reverse forwarding): not supported in v1. Reject `tcpip-forward` global requests.

**Rejected channel types in v1:**

- `x11`: rejected. X11 forwarding is rarely needed and complex to proxy safely.
- `auth-agent@openssh.com`: rejected. Agent forwarding is a security concern and adds substantial complexity.

`direct-tcpip` is borderline; I recommend supporting it for v1 because it's how `ssh -L` and ProxyJump work, and developers expect those. Implement it but log clearly when it's used.

**Connection string:** `ssh://<localCreds.Username>@127.0.0.1:<port>` (informational). The CLI command `cloak connect <secret>` shells out to `ssh -p <port> <user>@127.0.0.1` with the right options to pass the local password (via `sshpass`-style mechanism or, better, via `SSH_ASKPASS` set to a tiny helper binary that echoes the password — Cloak ships this helper).

**Env vars for `cloak exec`:**
- `<NAME>_SSH_HOST=127.0.0.1`
- `<NAME>_SSH_PORT=<port>`
- `<NAME>_SSH_USER=<localCreds.Username>`
- `<NAME>_SSH_PASSWORD=<localCreds.Password>`

For most workflows, users invoke `cloak connect` rather than `cloak exec` with SSH.

### 3.5 IPC Server

Local-only communication between daemon and CLI.

**Transport:** Unix domain socket at `~/.cloak/cloakd.sock` with mode `0600`. On Windows, a named pipe at `\\.\pipe\cloakd` with appropriate DACL restricting access to the current user.

**Protocol:** newline-delimited JSON-RPC 2.0. Simple, debuggable, and trivially adaptable to MCP later (MCP tools map directly to RPC methods).

**Authentication:**

- On connect, the client sends `hello` with `{client_token: "..."}`.
- The daemon validates the token by computing its Argon2id hash and comparing to `client_tokens.token_hash` in constant time.
- Additionally, on Linux/macOS, fetch the peer's PID and (on Linux) the executable path via `SO_PEERCRED` / `getsockopt(LOCAL_PEERPID)`. Log in audit but do not enforce in v1.
- Sessions are stateful per-connection: once authenticated, subsequent RPCs on the same socket are authorized as the same client.

**RPC methods (v1):**

| Method | Description |
|---|---|
| `hello` | Authenticate the connection. Body: `{client_token}`. |
| `vault.init` | First-time vault setup. Body: `{password}`. Generates DEK, writes meta. |
| `vault.unlock` | Body: `{password}`. Decrypts DEK, transitions to Unlocked. Auto-opens persistent endpoints. |
| `vault.lock` | Closes all endpoints, zeros DEK. |
| `vault.status` | Returns `{state, idle_timeout, expires_at, endpoints_open}`. |
| `secrets.list` | Returns secrets with metadata only (id, name, type, description, config (non-secret), endpoint config, timestamps). Never returns secret material. |
| `secrets.get` | Same as list but for one. |
| `secrets.reveal` | Decrypt and return one secret's material. Body: `{id_or_name, password}`. The `password` is the vault master password — re-checked before decrypting, since a client token alone (held also by AI agents) must not unlock plaintext. Returns `{id, name, type, config, secret}`. Audit-logged (`secret.revealed`, or `secret.reveal_denied` on a wrong password). |
| `secrets.create` | Create a new secret. Body: `{name, type, config, secret, endpoint_config}`. Secret material is encrypted before persisting. |
| `secrets.update` | Update fields. |
| `secrets.delete` | Delete a secret. |
| `endpoints.open` | Open a session endpoint. Body: `{secret_id, ttl_seconds?}`. Returns `{endpoint_id, local_addr, connection_string, env_vars, expires_at}`. |
| `endpoints.close` | Close an active endpoint. Body: `{endpoint_id}`. |
| `endpoints.list` | List active endpoints. |
| `endpoints.refresh` | Extend TTL of a session endpoint. |
| `tokens.create` | Issue a new client token. Body: `{name}`. Returns plaintext token **once**; daemon stores only the hash. |
| `tokens.list` | List tokens (metadata only). |
| `tokens.revoke` | Revoke by ID. |
| `audit.tail` | Stream recent audit log entries with optional filters. |

**Error format:**

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32000,
    "message": "vault_locked",
    "data": {"hint": "Run `cloak unlock` first."}
  }
}
```

Stable string error codes used by the CLI to render helpful messages:

- `vault_locked`, `vault_not_initialized`, `vault_already_initialized`
- `unauthorized`, `forbidden`
- `not_found`, `invalid_request`, `name_conflict`
- `adapter_error` (with sub-code), `endpoint_error`
- `internal_error`

### 3.6 Audit Logger

Append-only JSONL log at `~/.cloak/audit.log`. The log is **not encrypted** in v1 (it contains no secret material — only metadata about events).

**Each line:**

```json
{
  "ts": "2026-05-14T10:23:45.123Z",
  "seq": 4521,
  "prev_hash": "sha256:abc...",
  "event": "endpoint.connection.opened",
  "client": {"token_id": "01HX...", "name": "claude-cli", "pid": 12345},
  "secret_id": "01HX...",
  "secret_name": "prod-db",
  "endpoint_id": "01HX...",
  "remote_addr": "192.0.2.5:5432",
  "details": {...}
}
```

`prev_hash` is the SHA-256 of the previous line's full JSON (without the trailing newline). The first line uses a zero hash. This detects truncation and in-place edits. Cryptographic signing is deferred to v2.

**Event types logged in v1:**

- `vault.unlocked`, `vault.locked`, `vault.auto_locked`
- `secret.created`, `secret.updated`, `secret.deleted`
- `secret.revealed`, `secret.reveal_denied` (master-password-gated decrypt-and-show)
- `endpoint.opened`, `endpoint.closed`, `endpoint.expired`
- `endpoint.connection.opened`, `endpoint.connection.closed` (with bytes in/out)
- `endpoint.connection.upstream_failed` (auth failure, network error to upstream)
- `token.created`, `token.revoked`
- `client.authenticated`, `client.auth_failed`

Do not log connection contents (no SQL, no SSH commands, no HTTP bodies). v2 adds optional content logging gated by policy.

### 3.7 CLI

Single binary `cloak` using `github.com/spf13/cobra`.

**Top-level commands:**

```
cloak init                          One-time vault setup.
cloak daemon start [--foreground]   Start the daemon.
cloak daemon stop
cloak daemon status

cloak unlock                        Prompt for password, unlock vault.
cloak lock

cloak secret list
cloak secret show <name>            Metadata + non-secret config.
cloak secret reveal <name>          Decrypt + print material. Prompts for the master password.
cloak secret add ssh <name>         Interactive prompt for fields.
cloak secret add postgres <name>
cloak secret add mysql <name>
cloak secret add http <name>
cloak secret edit <name>            Edit non-secret config in $EDITOR.
cloak secret rotate <name>          Update secret material (interactive prompt).
cloak secret delete <name>

cloak endpoint list                 Show active endpoints.
cloak endpoint open <name>          Open a session endpoint, print URL.
cloak endpoint close <id-or-name>

cloak connect <name>                Convenience: open endpoint + run native client.
                                        - postgres → psql
                                        - mysql    → mysql
                                        - ssh      → ssh
                                        - http     → print URL, optionally curl shell

cloak exec --with <name>,<name>... -- <command...>
                                        Run a command with env vars injected
                                        for each named secret. Session endpoints
                                        opened on entry, closed on exit.

cloak token create --name <name>    Issue a token; printed once.
cloak token list
cloak token revoke <id>

cloak log [--follow] [--since 1h] [--secret <name>] [--type <event>]
```

**Interactive secret input:** never accept secret material as a CLI flag (no `--password`). Always read from a TTY with echo disabled, or from stdin via `--from-stdin` for scripted bootstrapping.

**Output format:** human-readable by default; `--json` flag on every command for scripting and for the future MCP server to invoke.

#### `cloak connect` details

For Postgres: open session endpoint, set `PGPASSWORD=<local_password>`, `exec` `psql -h 127.0.0.1 -p <port> -U <local_user> -d <database>`.

For MySQL: similar with `MYSQL_PWD`.

For SSH: open session endpoint, write the local password to a temporary `SSH_ASKPASS` helper, `exec` `ssh -p <port> -o UserKnownHostsFile=<kb_host_keys> -o StrictHostKeyChecking=accept-new <local_user>@127.0.0.1`.

For HTTP: print the URL and (if `require_local_auth`) the token, and exit. Optionally drop into a shell with `<NAME>_URL` and `<NAME>_TOKEN` set.

#### `cloak exec` details

1. Parse `--with` list. Resolve each name to a secret.
2. Open a session endpoint for each (in parallel).
3. Build the child process environment:
   - Inherit current environment.
   - For each secret, call `Adapter.EnvVars(...)` and merge.
4. `exec` the child command (replace process or fork+wait — fork+wait so we can clean up endpoints on exit).
5. On child exit (or SIGTERM/SIGINT received), close all opened endpoints.

Edge cases:
- If child fork()s into a long-lived process and the parent exits, Cloak cleans up endpoints anyway. This means the orphan can't reach the upstream anymore. Document this — for daemon-style apps, use `cloak endpoint open` + `nohup`.
- If a port is taken, fall back to a different random port (for session mode). For persistent mode, error out — the user must reconfigure.

---

## 4. Project Structure

```
cloak/
├── cmd/
│   ├── cloakd/                 # daemon binary
│   │   └── main.go
│   └── cloak/                  # CLI binary
│       └── main.go
├── internal/
│   ├── vault/                      # KEK/DEK, KDF, mlock, lock state
│   ├── store/                      # SQLite access, field encryption
│   ├── endpoints/                  # Endpoint Manager
│   ├── adapters/
│   │   ├── adapter.go              # interface
│   │   ├── http/
│   │   ├── postgres/
│   │   ├── mysql/
│   │   └── ssh/
│   ├── ipc/                        # JSON-RPC server, method registry
│   ├── audit/                      # append-only logger with hash chain
│   ├── paths/                      # OS-specific paths
│   ├── secrets/                    # SecretBytes, zeroing helpers
│   └── config/                     # TOML config loading
├── pkg/
│   └── client/                     # Go client library for the JSON-RPC API
│                                   # used by the CLI; reusable for MCP server, GUI
├── go.mod
├── go.sum
└── README.md
```

The future MCP server lives in `cmd/cloak-mcp/` and uses `pkg/client`. The future GUI does the same. This separation is the primary extensibility point — never bypass `pkg/client` from within those future binaries.

---

## 5. Cryptographic Primitives

Use only well-vetted libraries:

- **KDF:** `golang.org/x/crypto/argon2` (Argon2id).
- **AEAD:** `golang.org/x/crypto/chacha20poly1305` (XChaCha20-Poly1305 for the larger nonce — generated fresh per field).
- **Random:** `crypto/rand` exclusively.
- **Hashing:** `crypto/sha256`.
- **SSH:** `golang.org/x/crypto/ssh`.

Nonces for AEAD: fresh per encryption, prepended to ciphertext on storage.

All cryptographic operations live in `internal/vault` and `internal/store`. No ad-hoc crypto elsewhere.

---

## 6. Dependencies (target list)

Approved for v1:

- `github.com/spf13/cobra` — CLI framework.
- `github.com/BurntSushi/toml` — config parsing.
- `modernc.org/sqlite` — pure-Go SQLite.
- `golang.org/x/crypto/...` — KDF, AEAD, SSH.
- `github.com/jackc/pgx/v5` — Postgres upstream + pgproto3 for listening side.
- `github.com/go-sql-driver/mysql` — MySQL driver (upstream connection).
- `github.com/go-mysql-org/go-mysql` — MySQL server-side handshake.
- `github.com/oklog/ulid/v2` — IDs.
- `github.com/rs/zerolog` — structured logging (operational; separate from audit log).
- `golang.org/x/sys` — mlock, peer credential syscalls.
- `golang.org/x/term` — TTY password prompts.

No web frameworks, no DI containers, no ORMs. Hand-written SQL with `database/sql`.

---

## 7. Concurrency Model

- Single daemon process, multi-goroutine.
- Vault is goroutine-safe via RWMutex; reads vastly dominate writes after unlock.
- IPC server: one goroutine per connection.
- Endpoint Manager: one goroutine per listener accept loop, plus one or two per accepted connection (adapter-dependent).
- Each connection's processing is tied to a context derived from `endpoint.ctx`. Cancelling endpoint context tears down everything beneath.
- On vault lock: cancel the root context. All listeners and connections terminate. Vault Manager waits up to a timeout (default 5s) for clean shutdown, then proceeds.

---

## 8. Error Handling

Errors crossing the IPC boundary use stable string codes (Section 3.5). Internal errors wrap with `fmt.Errorf("...: %w", err)`. The IPC layer maps internal errors to safe public codes; full details go to the operational log (`zerolog`), not to the client.

Adapter errors include a sub-code (`adapter_error.upstream_auth_failed`, `adapter_error.upstream_unreachable`, `adapter_error.local_auth_failed`, etc.) for the CLI to render meaningful messages.

---

## 9. Security Properties to Maintain

The contract v1 must uphold. Future versions must not regress:

1. **Master password is never persisted.** Only its Argon2id derivation reaches storage.
2. **DEK on disk is always wrapped by the KEK.** No code path writes an unwrapped DEK.
3. **Decrypted secret material exists only inside an adapter's `ServeConnection` invocation,** and is zeroed in a `defer`. Same applies to local credentials when an endpoint closes.
4. **The IPC server never returns raw secret material to clients.** Code review checklist: any handler reading `secret_blob` must not include its plaintext in any response.
5. **Client tokens are stored as Argon2id hashes only.** Plaintext shown once at creation.
6. **Audit log is append-only and hash-chained.** No non-append open of the file.
7. **Idle auto-lock cannot be disabled.** Minimum 5 minutes.
8. **Endpoint listeners bind only to `127.0.0.1`** (never `0.0.0.0`). No exception.
9. **Upstream host keys (SSH) and TLS certificates (Postgres/MySQL) are validated** against the secret's configured fingerprints / CA. Reject on mismatch; do not prompt the client.
10. **Local endpoint client authentication uses constant-time comparison.** Avoid timing leaks on ephemeral credentials.

---

## 10. Configuration

`~/.cloak/config.toml`:

```toml
[daemon]
socket_path = "~/.cloak/cloakd.sock"
log_level = "info"

[vault]
idle_timeout = "1h"
# Minimum enforced 5m; auto-clamped if lower.

[endpoints]
default_persistent_port_start = 54200
# Persistent endpoints with no explicit port get assigned from this range upward.

[ssh]
host_key_dir = "~/.cloak/host_keys"

[audit]
log_path = "~/.cloak/audit.log"
```

All paths support `~` expansion. Environment variables `CLOAK_HOME`, `CLOAK_CONFIG` override defaults.

---

## 11. Build, Test, Distribution

**Build:**

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o cloakd ./cmd/cloakd
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o cloak  ./cmd/cloak
```

Cross-compilation works because we depend on no CGo packages.

**Testing:**

- Unit tests in every package. Tight coverage on `vault`, `store`, `audit`.
- Adapter integration tests under build tag `//go:build integration`, using:
  - `dockertest` for Postgres and MySQL.
  - An in-process SSH server (using `golang.org/x/crypto/ssh` itself) for SSH tests.
  - `httptest.Server` for HTTP.
- One full end-to-end test that exercises: `init` → `unlock` → `secret add` → `endpoint open` → connect with real client → `lock`.

**Distribution:**

- Pre-built tarballs for `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`.
- Homebrew tap (`brew install cloak/tap/cloak`).
- Simple `install.sh` script for Linux/macOS.
- No installer for v1 on Windows beyond the zip.

---

## 12. Acceptance Criteria for v1

A user on a fresh machine must be able to:

1. Install Cloak (`brew install` or download tarball).
2. `cloak init` — set master password.
3. `cloak daemon start` — daemon running.
4. `cloak unlock` — vault unlocked.
5. `cloak secret add postgres prod-db` — interactively enter host, port, user, database, password. Configure as persistent endpoint on port 54200.
6. `cloak token create --name my-shell` — get a token, store in shell config.
7. From a separate terminal: `psql -h localhost -p 54200 -U <local_user> -d <db>` (with `PGPASSWORD` set to the local password printed by `cloak connect prod-db --show-creds`) → successful connection to the real DB.
8. Alternatively: `cloak connect prod-db` opens psql automatically with all the right options.
9. `cloak exec --with prod-db -- ./my-app` runs `my-app` with `DATABASE_URL` pointing at the local endpoint.
10. Same for `cloak secret add ssh prod-server` and `cloak connect prod-server`.
11. Same for `cloak secret add http stripe-api` with header injection rules. `curl -H "Authorization: Bearer $TOKEN" http://localhost:54100/v1/customers` works without the real Stripe key ever touching `curl`.
12. `cloak log` shows every connection event with timestamps and byte counts.
13. `cloak lock` (or 1 hour of idle) → all endpoints close, vault locked, further connections refused until re-unlock.

At no point in this flow is the user expected to handle raw credential material in shell history, environment variables of unrelated processes, or unencrypted files.

---

## 13. Roadmap — what comes after v1

This section exists so v1 leaves the right hooks in place. **Do not implement any of this in v1.** But when designing v1 components, check this list and confirm the design admits these extensions cleanly.

### v1.1: Native key storage

**Goal:** stop requiring the master password on every unlock for users who prefer biometric / hardware-backed unlock.

- macOS Keychain integration: store DEK (wrapped by a Keychain-managed key) in Keychain with Touch ID access control. `cloak unlock` prompts via the system biometric dialog.
- Windows: DPAPI or WebAuthn-via-Windows-Hello equivalent.
- Linux: Secret Service API (gnome-keyring / KWallet); biometric support varies, falls back to OS-managed passphrase entry.

Extensibility hooks needed in v1: the `vault.unlock` IPC method must accept alternative unlock proofs, not just `password`. Add an `unlock_method` field with `"password"` as the only value in v1; future methods are `"keychain"`, `"dpapi"`, `"yubikey"`. Code path branches on method, with a single Unlock interface.

### v1.2: MCP server

**Goal:** AI agents using MCP (Claude Desktop, Cursor, Cline, others) gain first-class access to Cloak without needing to shell out to the CLI.

- New binary `cmd/cloak-mcp/` implementing the Model Context Protocol over stdio.
- Tools exposed to the agent:
  - `list_secrets()` — names and types only.
  - `open_endpoint(name)` — returns connection string + ephemeral credentials.
  - `close_endpoint(id)`.
  - `list_endpoints()`.
- The MCP server uses `pkg/client` to talk to the daemon.
- Auto-configuration helpers: `cloak mcp register --client claude-desktop` writes the MCP server entry into the client's config file.

Extensibility hooks needed in v1: `pkg/client` must be a stable, documented Go API. All operations the future MCP server needs are already exposed as RPC methods in v1.

### v1.3: GUI

**Goal:** non-CLI users (designers, junior devs, ops with less terminal comfort) can use Cloak.

- Tauri-based desktop app: tray icon + secret editor + endpoint dashboard + audit log viewer.
- Uses `pkg/client` over the same Unix socket. The GUI is just another IPC client.
- Confirmation dialogs replace CLI `[y/N]` prompts (relevant once v2 confirmation flow exists).

### v2.0: Protocol-level inspection and policy

**Goal:** per-operation access control instead of binary endpoint-level access.

- Adapter interface extended with an optional `Inspect` hook called per protocol message.
- Postgres adapter: parse `Query` / `Parse` messages, extract SQL strings, apply policy (regex allow/deny, statement type restrictions).
- MySQL adapter: same for COM_QUERY.
- SSH adapter: for `exec` channels, the command is in the channel request — easy to inspect. For interactive shells, optionally enable session recording (asciinema-style) instead of inline inspection.
- HTTP adapter: per-request method/path inspection (already structured; just adds policy hooks).
- Policy DSL: declarative rules per secret. JSON for v2.0; consider a more readable format later.

Extensibility hooks needed in v1:
- The transparent `io.Copy` proxying for Postgres/MySQL must be encapsulated behind a function the adapter calls — so v2 can replace it with a message-aware loop without restructuring the adapter.
- The SSH adapter's channel proxying must already dispatch by channel type (it does, per spec); v2 adds command extraction in the `session` channel handler.

### v2.1: Confirmation flow

**Goal:** for sensitive operations, block until the user explicitly approves.

- Policy decisions can return `confirmation_required` with a structured summary.
- IPC: clients receive the confirmation request and call `endpoints.confirm` with `allow` / `deny`.
- CLI: terminal prompt with operation summary.
- GUI: modal dialog.
- v1.1 biometric backends can be wired here for biometric approval.

Extensibility hooks needed in v1: the IPC method registry must support long-running requests with intermediate messages (or the protocol must support out-of-band notifications). v1 should choose JSON-RPC with notification support, which it does.

### v2.2: Request caching / VCR mode

**Goal:** record HTTP responses on first call, replay on subsequent identical calls, for fast offline testing without burning paid API quota.

- HTTP adapter mode: `live` (default), `record`, `replay`, `record_or_replay`.
- Cache keyed by method + path + body hash + selected headers.
- Cache storage: per-secret SQLite or files on disk; not encrypted (responses generally aren't secret, but redaction rules can apply).

Extensibility hooks needed in v1: HTTP adapter's request handler should accept middleware; caching is one such middleware.

### v2.3: SQL/wire-protocol response inspection

**Goal:** detect and redact sensitive patterns (credit card numbers, JWTs, AWS keys) in responses before they reach the client.

- Per-secret regex-based redaction rules.
- Adapter inspects response bodies / row values and applies substitutions.

### v3.0: Team features

**Goal:** secret templates and policies shareable across a team, with each member holding their own values.

- **Profile format:** committable JSON describing required secrets, their types, and their policies — without secret values. `cloak profile import myapp.profile.json` walks the user through setting the missing values.
- **Cloud sync (optional):** end-to-end encrypted sync of profiles (not values) between user's devices. Open-source self-hostable sync server.
- **Audit log export** to external SIEM (CloudWatch, Loki, Datadog).

Extensibility hooks needed in v1: the secret schema should be designed so the non-secret part (`config_json` + `endpoint_config` + `policy`) can be exported cleanly without secret material. It already is.

### v3.x: Hardware tokens

**Goal:** support YubiKey / SoloKey / similar for secret operations.

- For SSH secrets: the private key lives on the YubiKey (PIV applet or SSH-FIDO2). Cloak talks to the agent or PKCS#11 to sign challenges. Real key never reaches Cloak memory.
- For DEK: optional wrapping by a YubiKey-held key in addition to (or instead of) the password-derived KEK.

Extensibility hooks needed in v1: the `auth_method` field in SSH secret payloads allows adding `"yubikey_piv"` etc. without schema migration.

### v3.x: Mobile companion app

**Goal:** approve sensitive operations from a phone, away from the work machine.

- Pairing via QR code.
- Push notifications for confirmation requests.
- Approval triggers the equivalent of `endpoints.confirm`.

---

## 14. Open Questions Left for the Implementing LLM

Where the spec is intentionally underspecified — pick the simpler option and leave a `// TODO(v1.x):` comment:

- Exact format of stored endpoint stats (rolling window vs cumulative counters).
- Whether `endpoints.refresh` is allowed only by the originating client or any authenticated client (recommendation: any authenticated client; tokens are equivalent in v1).
- Whether daemon restart preserves persistent endpoint ports across runs (recommendation: yes, persist port assignments in the SQLite `meta` table).
- Exact directory permissions checks at startup (recommendation: refuse to start if `~/.cloak/` is not `0700` or socket isn't `0600`).

---

## 15. Out-of-scope reminders (do not over-build v1)

- Do **not** implement MCP.
- Do **not** implement GUI.
- Do **not** implement biometrics or OS keystore integration.
- Do **not** implement protocol inspection, policy enforcement per operation, or confirmation flow.
- Do **not** implement request caching / VCR mode.
- Do **not** implement team / sync / sharing features.
- Do **not** implement hardware token integration.
- Do **not** implement sandboxing of clients.

If a v1 design choice would make any of the above harder, change the choice. The extensibility points called out in Section 13 are the priority.

---

## 16. Materialized secrets — the `env` adapter

> **Status:** implemented (v1.x). This section specifies a feature added after
> the original v1 spec. It is the consolidated, authoritative reference for the
> `env` type: where it conflicts with the inline struct definitions in §3.2–3.6
> and §9, this section's "Amendments" notes win, and those older sections are
> left as written for the original four proxied types.

### 16.1 Motivation and the two secret tiers

The four v1 adapters (`postgres`, `mysql`, `ssh`, `http`) all share one shape:
the client speaks a network protocol to a `127.0.0.1:port` listener, Cloak
terminates that protocol, and re-injects the real credential on the upstream
leg. The real secret never reaches the client.

Many credentials do **not** fit that shape. CLI tools like `aws`, `gcloud`,
`kubectl`, `terraform`, `docker`, and `gh` authenticate by reading credentials
from **environment variables**, a **config file**, or a `credential_process`
command — not from a socket Cloak can proxy. To cover them, Cloak needs a
second, explicitly weaker tier.

Cloak therefore recognizes two kinds of secret:

- **Proxied secrets** (`postgres`, `mysql`, `ssh`, `http`) — Cloak sits in the
  byte path. The real secret never reaches the client. This is the strong tier
  and the product's headline guarantee.
- **Materialized secrets** (`env`, introduced here) — Cloak has no protocol to
  terminate. It decrypts the stored values and **injects them** into a single
  child process (as environment variables and/or a rendered file). The real
  secret **does** reach that process. This is the weak tier: Cloak degrades
  from "proxy" to "encrypted-at-rest store + central management + lifecycle +
  audit".

The weakening is deliberate and must be **visible**: materialized secrets are a
distinct secret type, flagged distinctly in `secret list` and the audit log, so
a user always knows which of their secrets are never-exposed and which are
merely well-managed. See §16.8.

### 16.2 Data model

#### 16.2.1 New secret type

Add `env` to `store.SecretType`:

```go
const TypeEnv SecretType = "env"
```

`IsKnown()` accepts it. The `secrets.type` column comment in §3.2 becomes
`"ssh" | "postgres" | "mysql" | "http" | "env"`. No SQL schema migration is
required — `env` is just another value in the existing `type` column, and its
key/value bag reuses `secret_blob` exactly as the other types reuse it.

#### 16.2.2 Encrypted payload shape (`secret_blob`)

The decrypted payload for `env` is a flat string→string map of **all** stored
values, secret and non-secret alike (e.g. `AWS_DEFAULT_REGION` is not secret
but is stored encrypted anyway — keeping the bag homogeneous is simpler than a
split, and over-encrypting a region name is harmless):

```json
{
  "values": {
    "AWS_ACCESS_KEY_ID":     "AKIA...",
    "AWS_SECRET_ACCESS_KEY":  "...",
    "AWS_DEFAULT_REGION":     "eu-west-1"
  }
}
```

Each key in `values` is used **verbatim** as an environment variable name when
env injection is enabled (§16.2.4). This is intentional and differs from the
`<NAME>_`-prefixing the proxied adapters apply: the point of an `env` secret is
that tools find the exact variable names they already look for.

#### 16.2.3 Non-secret config shape (`config_json`)

`config_json` for `env` holds only structure — no secret values:

```json
{
  "inject_env": true,
  "files": [
    {
      "basename": "credentials",
      "path_env": "AWS_SHARED_CREDENTIALS_FILE",
      "template": "[default]\naws_access_key_id={{ .AWS_ACCESS_KEY_ID }}\naws_secret_access_key={{ .AWS_SECRET_ACCESS_KEY }}\n"
    }
  ]
}
```

- `inject_env` (default `true`) — whether the `values` map is injected as
  environment variables. Set `false` for tools that read only a file.
- `files` (optional, may be empty) — file-rendering specs. Each spec:
  - `basename` — file name created inside the per-materialization run dir
    (§16.4.4).
  - `path_env` — environment variable that receives the **absolute path** of
    the written file.
  - `template` — a `text/template` body rendered against the decrypted
    `values` map (`{{ .KEY }}`). The template is structure, not secret, so it
    lives in `config_json`; only the rendered *output* is secret.

A secret with `inject_env=false` and no `files` is rejected by validation —
it would deliver nothing.

#### 16.2.4 Endpoint config constraints

`env` secrets reuse the existing `EndpointConfig` struct with constraints:

- `mode` **must** be `session`. Persistent mode is rejected by `ValidateConfig`
  in v1.x — a persistent `env` secret has no listener and nothing to auto-open
  into; "render a file on unlock, shred on lock" is a coherent idea but is
  deferred (§16.9).
- `persistent_port` — unused; rejected if set.
- `require_local_auth` — unused (there is no listener to authenticate to);
  ignored.
- `session_ttl_seconds` — **used**. Bounds the lifetime of the materialization
  handle and its rendered files (§16.4). Default 3600.
- `max_concurrent_connections` — unused; ignored.

#### 16.2.5 Validation rules (`ValidateConfig`)

The `env` adapter's `ValidateConfig` enforces:

1. `values` is non-empty.
2. Every key in `values` matches `^[A-Za-z_][A-Za-z0-9_]*$` (valid env var
   name) — this holds even when `inject_env=false`, because file templates
   reference the keys by name.
3. `inject_env=true` **or** `len(files) > 0` (the secret must deliver
   something).
4. Each `files[].template` parses as a `text/template` and references only
   keys present in `values` (rendered with `Option("missingkey=error")`).
5. Each `files[].path_env` matches the env-var-name regex; `basename` is a
   single path element (no `/`, not `.`/`..`).
6. `endpoint_config.mode == "session"`.

### 16.3 Adapter interface generalization

The current `Adapter` interface (§3.4) is built entirely around
`ServeConnection(ctx, client net.Conn, ...)`. An `env` adapter has no
connection. Split the interface into a common base plus two role interfaces.

```go
// AdapterKind tells the endpoint manager which lifecycle a type follows.
type AdapterKind int

const (
    KindProxy       AdapterKind = iota // opens a 127.0.0.1 listener
    KindMaterialized                   // injects values into a child process
)

// Adapter is the common base every type implements.
type Adapter interface {
    Type() store.SecretType
    Kind() AdapterKind
    ValidateConfig(config map[string]any, secret map[string]any) error
}

// ProxyAdapter is implemented by postgres, mysql, ssh, http.
// (These are the methods the current Adapter interface already has.)
type ProxyAdapter interface {
    Adapter
    ServeConnection(ctx context.Context, client net.Conn, decoded DecryptedSecret, localCreds LocalCredentials) error
    ConnectionString(localAddr string, decoded DecryptedSecret, localCreds LocalCredentials) string
    EnvVars(localAddr string, decoded DecryptedSecret, localCreds LocalCredentials, envPrefix string) map[string]string
}

// MaterializingAdapter is implemented by env (and future credential-injection types).
type MaterializingAdapter interface {
    Adapter
    // Materialize decrypts the secret into the values to deliver to a child
    // process. It does not touch the filesystem — the endpoint manager owns
    // file writing and cleanup (§16.4.4).
    Materialize(decoded DecryptedSecret) (Materialization, error)
}

// Materialization is what a MaterializingAdapter produces.
type Materialization struct {
    // Env is the set of environment variables to inject (real secret values).
    // Empty when config.inject_env is false.
    Env map[string]string
    // Files are file bodies the manager must write to the run dir before
    // returning to the client.
    Files []RenderedFile
}

type RenderedFile struct {
    Basename string                // file name within the per-materialization run dir
    PathEnv  string                // env var to set to the written file's absolute path
    Content  *secrets.SecretBytes  // file body; manager zeroes it after writing
}
```

`Registry` continues to store `Adapter`. Call sites type-assert to
`ProxyAdapter` / `MaterializingAdapter` (or branch on `Kind()`). The four
existing adapters gain a one-line `Kind() AdapterKind { return KindProxy }`;
no other change to them.

**Amends §3.4.** The interface block in §3.4 is replaced by the above. The
`DecryptedSecret` struct is unchanged.

### 16.4 Endpoint Manager generalization

Today an "endpoint" is always a TCP listener, and `ActiveEndpoint` always has a
`net.Listener`. This is the central structural change: an endpoint becomes a
**materialization** that is *either* a network listener *or* an injected-values
handle.

#### 16.4.1 Generalized `ActiveEndpoint`

```go
type EndpointKind int

const (
    EndpointListener     EndpointKind = iota // proxied secret: a live listener
    EndpointMaterialized                     // materialized secret: injected values
)

type ActiveEndpoint struct {
    ID         string
    SecretID   string
    SecretName string
    Type       store.SecretType
    Kind       EndpointKind
    Mode       store.EndpointMode   // always "session" for materialized
    OpenedAt   time.Time
    ExpiresAt  time.Time            // session TTL; applies to both kinds
    Stats      EndpointStats
    cancel     context.CancelFunc

    // Set only when Kind == EndpointListener:
    Listener   net.Listener
    LocalAddr  string
    LocalCreds LocalCredentials

    // Set only when Kind == EndpointMaterialized:
    RunDir     string               // per-materialization 0700 dir, or "" if no files
    FilePaths  []string             // absolute paths written, for cleanup
}
```

Key point: a materialized endpoint holds **no decrypted secret values** after
`endpoints.open` returns. The values are computed, sent to the client in the
response, and dropped. The manager retains only `RunDir`/`FilePaths` so it can
shred the rendered files later. (The rendered files on disk are the only
retained plaintext copy, and only when the secret defines `files`.)

#### 16.4.2 Open path

`endpoints.open` branches on the adapter kind:

- **`KindProxy`** — unchanged from §3.3: allocate port, `net.Listen`, generate
  local creds, spawn the accept-loop goroutine, build `connection_string` +
  `env_vars` from the adapter.
- **`KindMaterialized`** —
  1. Decrypt the secret payload into a `DecryptedSecret`.
  2. Call `adapter.Materialize(decoded)`.
  3. If `Files` is non-empty, create `RunDir = $CLOAK_HOME/run/<endpoint-id>/`
     mode `0700`; write each `RenderedFile` mode `0600`; zero its `Content`;
     record the absolute path and set `Env[file.PathEnv] = absPath`.
  4. Register an `ActiveEndpoint{Kind: EndpointMaterialized, ...}` with
     `ExpiresAt = now + session_ttl`. **No goroutine, no listener.**
  5. Return the IPC response with `env_vars = Materialization.Env` (plus the
     injected file-path vars), `local_addr = ""`, `connection_string = ""`.

There is no accept loop and no context-bound connection tree for a materialized
endpoint; `cancel` is retained only to unify teardown bookkeeping.

#### 16.4.3 Close / expiry / lock path

For a materialized endpoint, "close" means **shred**:

1. Overwrite each file in `FilePaths` with zeros, then remove it.
2. Remove `RunDir`.
3. Deregister the `ActiveEndpoint`.

This runs on all three existing teardown triggers, unchanged:
`endpoints.close`, session-TTL expiry, and vault lock. No proxied-endpoint
teardown logic changes — the manager simply also walks materialized endpoints.

#### 16.4.4 Run directory and cleanup ownership

Rendered files live under `~/.cloak/run/<endpoint-id>/` (a new entry in §3.1's
file list), **not** `/tmp`. Rationale: it inherits the `~/.cloak/` `0700`
boundary, sits on a known filesystem, and is trivially swept on daemon start.

**The daemon writes and owns the files** (not the CLI), because the daemon owns
the lifecycle: it must delete them on TTL expiry and vault lock even if the
`cloak exec` process that opened them was killed. On daemon startup, `~/.cloak/
run/` is emptied entirely — any contents are stale from a previous run.

The `cloak exec` child reads the file via the injected `path_env` variable; it
is a descendant of `cloak exec`, runs as the same user, and the file is `0600`.

#### 16.4.5 List, persistent auto-open, env collisions

- `endpoints.list` includes materialized endpoints; the response gains a
  `kind` field (`"listener"` | `"materialized"`). For materialized rows
  `local_addr` and `connection_string` are empty.
- The unlock-time persistent auto-open loop skips `env` secrets entirely
  (`env` cannot be persistent, §16.2.4), so no change is needed beyond the
  existing `mode == persistent` filter.
- `cloak exec --with a,b` where two materialized secrets define the same env
  var name is a **conflict**: `exec` detects the duplicate key across the
  merged `env_vars` maps and exits with an error naming the collision, rather
  than silently letting one win.

**Amends §3.3.** `EndpointManager.listeners` stays as-is (keyed by secret ID,
proxied endpoints only). `active` now also holds materialized endpoints. The
`ActiveEndpoint` struct in §3.3 is replaced by §16.4.1.

### 16.5 IPC changes

Deliberately minimal — materialized endpoints reuse the existing
`endpoints.open` / `endpoints.close` / `endpoints.list` methods.

- `endpoints.open` request is unchanged (`{secret_id, ttl_seconds?}`).
- `endpoints.open` response: `env_vars` is already present; for materialized
  secrets it carries the **real values** (see §16.8). `local_addr` and
  `connection_string` are `""`. Add an informational `kind` field.
- `endpoints.list` rows gain `kind`.

`cloak creds` (§16.6, the `credential_process` pull path) also rides
`endpoints.open`: it opens with `ttl_seconds = 0`, meaning "compute and return
`env_vars`, register no handle, write no files". The daemon returns the values
and retains nothing. Output formatting is done CLI-side.

**Amends §3.5.** No new methods. `endpoints.open`/`list` response fields as
above.

### 16.6 CLI surface

- `cloak secret add env <name>` — interactive: description, then a loop of
  key/value pairs (values read with echo disabled), then `inject_env?` and an
  optional file-render spec, then TTL. Validated per §16.2.5.
- `cloak exec --with <name> -- <cmd>` — already injects `env_vars` from
  `endpoints.open`; for `env` secrets those are the real values. No new code
  beyond the collision check (§16.4.5) and tolerating an empty `local_addr`.
- `cloak endpoint open <name>` — for `env`, prints the injected variable
  **names** and any file paths (never the values).
- `cloak creds <name> [--format env|dotenv|json|aws]` — new command for the
  `credential_process` integration. Dials the daemon, opens an ephemeral
  materialization (`ttl=0`), prints the values in the chosen format, exits.
  `--format aws` emits the AWS `credential_process` JSON shape
  (`{"Version":1,"AccessKeyId":...,"SecretAccessKey":...}`), mapping the
  conventional `AWS_*` keys; this mapping is a CLI-side formatter.
- `cloak connect <name>` — for `env`, out of scope; direct the user to
  `cloak exec`.

### 16.7 Audit events

Two new event types, logged with **names only — never values**:

- `secret.materialized` — `details`: `secret_name`, `client` (token id + pid),
  injected env var **names**, written file paths, `ttl`.
- `secret.unmaterialized` — emitted on close / TTL expiry / lock, with the
  teardown reason.

`secret list` / `endpoints.list` consumers can distinguish the tier from the
`type` (`env`) and `kind` (`materialized`) fields.

**Amends §3.6.** Adds the two event types to the §3.6 list.

### 16.8 Security model — explicit amendments

Materialized secrets necessarily relax two of the §9 properties. The relaxation
is **scoped to `type=env` secrets only** and must be stated, not silent.

- **Amends §9 property 3** ("decrypted secret material exists only inside an
  adapter's `ServeConnection` invocation"). For materialized secrets there is
  no `ServeConnection`. Decrypted values exist: (a) briefly in the daemon
  during `Materialize` and IPC-response encoding, (b) on disk in `~/.cloak/run/`
  for any rendered files, for the materialization's lifetime, and (c) in the
  client process and its child for the child's lifetime. Note also that once a
  value is placed in a Go `map[string]string` / `[]string` env it is an
  immutable, GC-managed string and **cannot be reliably zeroed** — the
  `SecretBytes`/`Zero` discipline cannot extend past the point where the value
  becomes an env string. This is inherent to the tier.

- **Amends §9 property 4** ("the IPC server never returns raw secret material
  to clients"). For `type=env` secrets, `endpoints.open` **does** return raw
  secret values in `env_vars` — that is the entire purpose of the type. The
  exception is scoped: only `type=env`, only to an authenticated client. The
  code-review checklist becomes: *no handler returns `secret_blob` plaintext
  except the `env`-type path of `endpoints.open`.*

- **New property:** materialized rendered files are written `0600` inside a
  `0700` per-materialization directory under `~/.cloak/run/`, and are
  zero-overwritten then removed on close / TTL expiry / vault lock. `~/.cloak/
  run/` is emptied on daemon startup.

- **Unchanged:** all proxied types keep the full §9 guarantees. The tier split
  exists precisely so that the strong guarantee is never quietly weakened for
  the types that have it.

### 16.9 Deferred (not in this draft)

- **Persistent file rendering** — render a secret's file on vault unlock and
  shred it on lock (e.g. maintain `~/.aws/credentials` while unlocked). Coherent
  but writes a secret to a well-known path; deferred until the session-scoped
  path is proven.
- **Short-lived credential vending** — for AWS specifically, store the
  long-lived IAM key and have Cloak call `sts:GetSessionToken` / `AssumeRole`
  so the child receives only an expiring, scoped token. This belongs in a
  future `aws`-specific adapter, not the generic `env` type.
- **The `aws` SigV4 proxy adapter** — a true proxied (`KindProxy`) adapter that
  re-signs AWS API requests, keeping the strong-tier guarantee for AWS. Tracked
  separately; it builds on the §16.3 `Kind()` split but is otherwise unrelated
  to this section.

---

## End of specification

Implement v1 strictly to this specification. When ambiguities arise, prefer the **simpler and more conservative** option, and leave a `// TODO(v1.x):` comment explaining the trade-off so it can be revisited.
