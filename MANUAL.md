# Cloak — User Manual

Cloak is a local secret broker. It keeps real credentials inside an encrypted
local vault and exposes each one as a **local network endpoint** that speaks the
credential's native protocol (Postgres, MySQL, SSH, HTTP). Your application — or
your AI agent, or `psql`, or `curl` — connects to `127.0.0.1` and never sees the
real secret.

This document is the day-to-day guide. For the full design specification, see
[`cloak-architecture.md`](./cloak-architecture.md).

---

## Table of contents

1. [Installation](#1-installation)
2. [Quick start](#2-quick-start)
3. [Mental model](#3-mental-model)
4. [Command reference](#4-command-reference)
   - [`cloak daemon`](#cloak-daemon)
   - [`cloak init` / `unlock` / `lock` / `status`](#cloak-init--unlock--lock--status)
   - [`cloak secret`](#cloak-secret)
   - [`cloak endpoint`](#cloak-endpoint)
   - [`cloak connect`](#cloak-connect)
   - [`cloak exec`](#cloak-exec)
   - [`cloak creds`](#cloak-creds)
   - [`cloak token`](#cloak-token)
   - [`cloak log`](#cloak-log)
5. [Recipes by secret type](#5-recipes-by-secret-type)
   - [Postgres](#postgres)
   - [MySQL](#mysql)
   - [SSH](#ssh)
   - [HTTP](#http)
   - [Env](#env)
6. [Configuration](#6-configuration)
7. [Environment variables](#7-environment-variables)
8. [Files on disk](#8-files-on-disk)
9. [Security model](#9-security-model)
10. [Troubleshooting](#10-troubleshooting)
11. [FAQ](#11-faq)

---

## 1. Installation

Cloak is two binaries: **`cloakd`** (the daemon) and **`cloak`** (the CLI).
Both cross-compile cleanly because the project depends on no CGo packages.

### Build from source

```bash
git clone https://github.com/Chekunin/cloak
cd cloak
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ./bin/cloakd ./cmd/cloakd
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ./bin/cloak  ./cmd/cloak
export PATH="$PWD/bin:$PATH"
```

The `cloak` CLI locates `cloakd` in this order:

1. `$CLOAKD_BIN` if set,
2. `cloakd` (or `cloakd.exe` on Windows) next to the running `cloak`,
3. `cloakd` on `$PATH`.

Placing both binaries in the same directory and putting it on `$PATH` covers
every case.

### Verify

```bash
cloak --help
cloakd -help
```

---

## 2. Quick start

The five-minute path from nothing to a working Postgres proxy.

```bash
cloak daemon start                   # 1. start the daemon
cloak init                           # 2. set a master password
cloak unlock                         # 3. unlock the vault
cloak token create --name shell --save   # 4. bootstrap a CLI token
cloak secret add postgres prod-db    # 5. interactive prompt for connection details
cloak connect prod-db                # 6. opens psql against the local endpoint
```

When you're done:

```bash
cloak lock                           # zero the DEK, close all endpoints
cloak daemon stop                    # stop the daemon process (optional)
```

That's the whole loop. The rest of this manual is the reference.

---

## 3. Mental model

Four concepts.

### Vault

The encrypted store on disk (`~/.cloak/vault.db`) plus the master-key state in
the daemon's memory. It has three states:

- **`uninitialized`** — `cloak init` hasn't run yet.
- **`locked`** — the database exists on disk but the daemon doesn't hold the
  decryption key. No secrets can be read.
- **`unlocked`** — the daemon holds the data-encryption key in protected
  memory and can decrypt individual secrets on demand.

The vault auto-locks after one hour of inactivity (configurable, but never less
than 5 minutes). You can also lock it explicitly with `cloak lock`.

### Secret

One stored credential plus its endpoint configuration. Has:

- A **name** you chose (e.g. `prod-db`) and a stable **id** (a ULID).
- A **type** — one of `postgres`, `mysql`, `ssh`, `http`, `env`.
- **Non-secret config** (host, port, user, database, host-key fingerprint…) —
  stored plaintext.
- **Secret material** (password, private-key PEM, header injection values) —
  AEAD-encrypted under the vault DEK.
- An **endpoint config** (persistent or session, optional fixed port, TTL,
  whether local auth is required).

Secrets come in two tiers. **Proxied** secrets (`postgres`, `mysql`, `ssh`,
`http`) are reached through a local endpoint that speaks their protocol — the
real credential never reaches the client. **Materialized** secrets (`env`) have
no endpoint: Cloak decrypts their stored values and injects them into a child
process as environment variables and/or a rendered file. That covers any CLI
tool — the AWS CLI, `gcloud`, `kubectl`, and so on — but the real secret *does*
reach that process. See [Security model](#9-security-model).

### Endpoint

A live `127.0.0.1:<port>` TCP listener that proxies for a secret. Two modes:

| Mode | Port | Lifetime | Opened by |
|---|---|---|---|
| **persistent** | stable (configured or auto-assigned) | until vault lock | auto on vault unlock |
| **session** | random ephemeral | TTL-bounded (default 1h) | explicit `endpoint open` or `cloak exec` |

Persistent endpoints are made for `.env`-style use: your app reads
`DATABASE_URL=postgresql://...@localhost:54200/db` and connects there. Session
endpoints are made for one-shot scripts, agent sessions, and `cloak exec`.

When you connect to an endpoint, the daemon decrypts the real secret for that
single connection, hands it to the protocol adapter, and the adapter proxies
traffic between you and the upstream service. The decrypted credential lives in
process memory only for the lifetime of the connection.

### Client token

The bearer token your CLI (or future MCP server, or GUI) sends to the daemon
to authenticate. Each token is a random 32-byte secret printed **once** on
creation; the daemon stores only its Argon2id hash.

The CLI reads its token from `~/.cloak/cli_token` (or `$CLOAK_TOKEN`). The very
first token can be created without authenticating (bootstrap path); after that,
`tokens.create` requires a valid existing token.

---

## 4. Command reference

Every command supports `--json` for machine-readable output. The CLI exits
non-zero on any error.

### `cloak daemon`

| Command | Purpose |
|---|---|
| `cloak daemon start` | Start the daemon in the background. |
| `cloak daemon start --foreground` | Run in the foreground (useful for development / `journalctl`-style logging). |
| `cloak daemon stop` | Send `SIGTERM` to the running daemon. Triggers a clean shutdown: locks the vault, closes endpoints, removes the socket. |
| `cloak daemon status` | Print pid + socket path if running, else "not running". |

The daemon writes its pid to `~/.cloak/cloakd.pid` so `status` and `stop` can
find it. It listens on a Unix socket at `~/.cloak/cloakd.sock`, mode `0600` —
only the daemon's owning user can connect.

### `cloak init` / `unlock` / `lock` / `status`

| Command | Purpose |
|---|---|
| `cloak init` | One-time vault setup. Prompts for a master password twice. |
| `cloak unlock` | Prompts for the master password, unlocks the vault, auto-opens all persistent endpoints. |
| `cloak lock` | Closes every endpoint and zeros the DEK. Equivalent to the auto-lock timer firing. |
| `cloak status` | Show the vault state, idle timeout, and number of open endpoints. |

Both `init` and `unlock` accept `--from-stdin` to read the password from the
first line of stdin instead of a TTY prompt — useful for scripted bootstrapping:

```bash
echo "$MY_VAULT_PASSWORD" | cloak unlock --from-stdin
```

**The master password is never persisted.** If you forget it, the vault cannot
be recovered. Cloak does not implement password reset.

### `cloak secret`

| Command | Purpose |
|---|---|
| `cloak secret list` | List stored secrets (metadata only — no credentials). |
| `cloak secret show <name>` | Show full metadata + non-secret config for one secret. |
| `cloak secret reveal <name>` | Decrypt and print the secret material. Prompts for the master password. |
| `cloak secret add <type> <name>` | Interactive prompt to add a new secret. `<type>` is `postgres`, `mysql`, `ssh`, `http`, or `env`. |
| `cloak secret rotate <name>` | Replace just the secret material (password, key) — config is left alone. |
| `cloak secret delete <name>` | Remove the secret. Any open endpoint for it is closed first. |

Secret values are **never** accepted as command-line flags. They're prompted
on a TTY with echo disabled, or read from stdin via `--from-stdin`.

The vault must be unlocked for any `secret` subcommand. `secret list` and
`secret show` never decrypt the payload — they only return metadata.

`secret reveal` is the one command that does decrypt and print the stored
credential — so Cloak can double as an everyday password manager. It is
deliberately gated: the daemon re-checks the **master password** before
decrypting (a client token alone — which an AI agent also holds — is not
enough), and every reveal is recorded in the audit log. Pass `--from-stdin`
to supply the master password non-interactively. The same gate backs the
**Reveal** button in the desktop app.

### `cloak endpoint`

| Command | Purpose |
|---|---|
| `cloak endpoint list` | Show currently-open endpoints (name, type, mode, address, connection count). |
| `cloak endpoint open <name> [--ttl <seconds>]` | Open a session endpoint for the named secret. Prints the connection URL and env vars. |
| `cloak endpoint close <endpoint-id-or-secret-name>` | Close a specific endpoint. |

For persistent-mode secrets, an endpoint is auto-opened on vault unlock; you
typically don't call `endpoint open` for them.

For session-mode secrets, `endpoint open` is how you instantiate one. The
endpoint expires after `--ttl` seconds (or the secret's configured TTL,
default 3600).

### `cloak connect`

```
cloak connect <name>
```

Convenience: opens (or reuses) an endpoint and immediately launches the
appropriate native client with the ephemeral local credentials wired in.

- `postgres` → `exec psql -h 127.0.0.1 -p <port> -U <local_user> -d <db>` with
  `PGPASSWORD` set.
- `mysql` → similar with `MYSQL_PWD`.
- `ssh` → prints connection info; SSH-from-scratch automation is on the v1.x
  list.
- `http` → prints the URL and bearer token.
- `env` → not applicable (no endpoint to launch a client against). `connect`
  prints a pointer to `cloak exec` / `cloak creds` instead.

The native client (`psql`, `mysql`, …) must be installed on your `$PATH`.

### `cloak exec`

```
cloak exec --with <name>[,<name>...] -- <command> [args...]
```

Run a child command with endpoint environment variables injected for one or
more secrets. The endpoints are opened on entry and closed on exit. Typical use:

```bash
cloak exec --with prod-db -- ./my-app
cloak exec --with prod-db,stripe-api -- bash -c 'curl -H "Authorization: Bearer $STRIPE_API_TOKEN" $STRIPE_API_URL/v1/customers'
cloak exec --with prod-db -- goose postgres "$DATABASE_URL" up
```

Each adapter contributes its own env-var set:

| Type | Variables injected |
|---|---|
| postgres | `DATABASE_URL`, `<NAME>_URL`, `PGHOST`, `PGPORT`, `PGUSER`, `PGPASSWORD`, `PGDATABASE` |
| mysql | `MYSQL_URL`, `<NAME>_URL`, `MYSQL_HOST`, `MYSQL_PORT`, `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_PWD`, `MYSQL_DATABASE` |
| ssh | `<NAME>_SSH_HOST`, `<NAME>_SSH_PORT`, `<NAME>_SSH_USER`, `<NAME>_SSH_PASSWORD` |
| http | `<NAME>_URL`, `<NAME>_TOKEN` (only if local auth is enabled) |
| env | the secret's stored key/value pairs verbatim, plus any rendered-file path variable |

For an `env` secret the injected variables are the **real** stored values, not
ephemeral ones — see the [Env recipe](#env). If two secrets named in one
`--with` set the same variable, `cloak exec` reports the collision and exits.

`<NAME>` is the secret name uppercased, with non-alphanumeric characters
replaced by `_`. So a secret called `stripe-api` produces
`STRIPE_API_URL` and `STRIPE_API_TOKEN`.

**Note on long-lived children**: if your command forks a daemon-style process
and the parent exits, Cloak still closes the endpoints. The orphan child can no
longer reach the upstream. For long-running services, use `cloak endpoint open`
plus `nohup` instead.

### `cloak creds`

```
cloak creds <name> [--format env|json|aws]
```

Print a materialized (`env`) secret's values to stdout. Unlike `cloak exec`,
this runs no child process — it is meant to be wired into a tool's own
credential-helper hook.

The classic use is the AWS CLI's `credential_process`. In `~/.aws/config`:

```ini
[profile prod]
credential_process = cloak creds aws-prod --format aws
```

Now any `aws …` invocation for that profile transparently fetches the
credentials from Cloak — no `cloak exec` wrapper needed.

Formats:

- `env` (default) — `KEY=VALUE` lines.
- `json` — a `{"KEY": "VALUE"}` object.
- `aws` — the AWS `credential_process` JSON shape; requires `AWS_ACCESS_KEY_ID`
  and `AWS_SECRET_ACCESS_KEY` in the secret (and includes `AWS_SESSION_TOKEN`
  if present).

`cloak creds` prints real secret values to stdout — that is its purpose. The
vault must be unlocked.

### `cloak token`

| Command | Purpose |
|---|---|
| `cloak token create --name <name> [--save]` | Issue a new client token. The plaintext is printed exactly once. `--save` writes it to `~/.cloak/cli_token`. |
| `cloak token list` | Show all tokens (metadata only — id, name, created/last-seen, revoked flag). |
| `cloak token revoke <id>` | Revoke a token. Subsequent `hello`s with it are rejected. |

The first `tokens.create` on a fresh daemon can be made without authentication
— this is the bootstrap path. Once any token exists, subsequent token creation
requires a valid existing token via `hello`.

### `cloak log`

```
cloak log [--follow] [--since <duration>] [--secret <name>] [--type <prefix>] [--limit <n>]
```

Show entries from the hash-chained audit log (`~/.cloak/audit.log`).

Examples:

```bash
cloak log --since 1h                           # last hour
cloak log --secret prod-db                     # one secret
cloak log --type endpoint.connection           # only connection-level events
cloak log --follow                             # tail (polls every second)
cloak log --json                               # raw JSONL for piping
```

Event types you'll see:

- `vault.unlocked`, `vault.locked`, `vault.auto_locked`
- `secret.created`, `secret.updated`, `secret.deleted`
- `secret.revealed`, `secret.reveal_denied` (decrypt-and-show; denied = wrong master password)
- `secret.materialized`, `secret.unmaterialized` (for `env` secrets)
- `endpoint.opened`, `endpoint.closed`, `endpoint.expired`
- `endpoint.connection.opened`, `endpoint.connection.closed`,
  `endpoint.connection.upstream_failed`
- `token.created`, `token.revoked`
- `client.authenticated`, `client.auth_failed`

The audit log is append-only with a SHA-256 hash chain (`prev_hash`) connecting
entries. Tampering is detectable on replay; cryptographic signing is on the
v2 roadmap.

---

## 5. Recipes by secret type

### Postgres

```
$ cloak secret add postgres prod-db
Description (optional): production read-replica
Endpoint mode [persistent/session, default persistent]: persistent
Require local authentication? [Y/n]: Y
Host: db.example.com
Port: 5432
User: app_user
Database: app_db
TLS mode [disable/prefer/require, default prefer]: require
Persistent port (leave blank for auto): 54200
Database password: ********
```

Then any of:

```bash
cloak connect prod-db                                       # psql shell
cloak endpoint open prod-db                                 # print URL
# .env:
DATABASE_URL=postgresql://cloak_<id>:<random>@localhost:54200/app_db?sslmode=disable
```

Use any Postgres client: `psql`, `pgcli`, JDBC, `pg` (Node), `psycopg`,
`pgx`, GUIs like Postico/DBeaver. The client's connection always talks plain
TCP to `127.0.0.1`; the Cloak daemon handles TLS upstream according to the
secret's `tls_mode`.

If your tooling rejects `sslmode=disable`, set `tls_mode=require` on the
secret — that only affects the Cloak→upstream leg. The local leg is always
plain (it's loopback).

### MySQL

```
$ cloak secret add mysql prod-mysql
...
Host: mysql.example.com
Port: 3306
User: app_user
Database: app_db
TLS mode [disable/prefer/require, default prefer]: prefer
Persistent port (leave blank for auto): 54300
Database password: ********
```

Then:

```bash
cloak connect prod-mysql
cloak exec --with prod-mysql -- mysql -e "SHOW TABLES"
```

### SSH

```
$ cloak secret add ssh prod-server
...
Host: prod-server.internal
Port [22]: 22
User: deploy
Upstream host key fingerprint (SHA256:...): SHA256:abc123...
Endpoint mode [persistent/session, default persistent]: session
Auth method [password/private_key]: private_key
Path to private key PEM: /Users/me/.ssh/id_ed25519
Key passphrase (empty if none): ********
```

**The host-key fingerprint is required.** Cloak refuses to connect upstream
without a pinned fingerprint — there is no TOFU in v1. Get it once with:

```bash
ssh-keyscan -t ed25519 prod-server.internal | ssh-keygen -lf -
```

Then use the printed `SHA256:...` (or `sha256:...` — both forms accepted).

Once added:

```bash
cloak endpoint open prod-server
# prints, e.g.:
#   Endpoint:        127.0.0.1:54201
#   Connection URL:  ssh://cloak_01abc...@127.0.0.1:54201
# Then in another terminal:
ssh -p 54201 -o StrictHostKeyChecking=no cloak_01abc...@127.0.0.1
# password prompt: paste the value of <NAME>_SSH_PASSWORD shown by `cloak endpoint open`
```

`StrictHostKeyChecking=no` because each daemon start generates fresh local
host keys for the SSH-server-side of the endpoint. (The *upstream* host key
is still pinned by your stored fingerprint.)

SFTP works through the same endpoint:

```bash
sftp -P 54201 cloak_01abc...@127.0.0.1
```

`ssh -L` (local port forwarding) is supported. `ssh -R` (reverse forwarding),
X11 forwarding, and agent forwarding are not — the daemon rejects those
channel types.

### HTTP

The HTTP adapter is the most flexible — it can inject request headers and
query parameters from stored values. Typical use is "I have a Stripe key
but I don't want my code or my agent to see it."

```
$ cloak secret add http stripe-api
Description: Stripe live API
Endpoint mode [persistent/session, default persistent]: persistent
Require local authentication? [Y/n]: Y
Upstream URL: https://api.stripe.com
Enter HTTP injection rules. Header injection format: 'name=template'. Blank line to finish.
Header: Authorization=Bearer {{ .api_key }}
Header: <blank to finish>
Enter values referenced by templates ({{ .key }}). Blank line to finish.
Value key: api_key
Value for api_key: sk_live_...
Value key: <blank to finish>
Persistent port: 54100
```

Now:

```bash
$ cloak endpoint open stripe-api
Endpoint:        127.0.0.1:54100
Connection URL:  http://127.0.0.1:54100
Environment:
  STRIPE_API_URL=http://127.0.0.1:54100
  STRIPE_API_TOKEN=somerandombytes...

# Use it:
$ curl -H "Authorization: Bearer $STRIPE_API_TOKEN" \
       http://127.0.0.1:54100/v1/customers
```

What just happened: your `curl` sent the local token; Cloak verified it,
stripped the Authorization header, injected `Authorization: Bearer sk_live_...`
from the stored values, and forwarded the request to `https://api.stripe.com`.
Your `curl` (and your shell history, and your AI agent) never touched
`sk_live_...`.

The injection template syntax is Go's `text/template`. Multiple values per
header work too:

```
Header: X-Custom={{ .prefix }}-{{ .id }}
```

### Env

The `env` type is the catch-all for tools Cloak cannot proxy — the AWS CLI,
`gcloud`, `kubectl`, `terraform`, `docker login`, `gh`, or any program that
reads a credential from its environment or a config file.

An `env` secret is a bag of key/value pairs. It is always session-mode and has
no endpoint. `cloak exec` injects the pairs as environment variables for one
child process; `cloak creds` prints them for a credential helper.

```
$ cloak secret add env aws-prod
Description (optional): AWS production account
Enter key/value pairs (the key is the environment variable name).
Blank key to finish.
Key: AWS_ACCESS_KEY_ID
Value for AWS_ACCESS_KEY_ID: ********
Key: AWS_SECRET_ACCESS_KEY
Value for AWS_SECRET_ACCESS_KEY: ********
Key: AWS_DEFAULT_REGION
Value for AWS_DEFAULT_REGION: ********
Key:
Inject as environment variables? [Y/n]: Y
Render a credentials file? [y/N]: N
Session TTL seconds [3600]:
```

Then:

```bash
cloak exec --with aws-prod -- aws s3 ls
cloak exec --with aws-prod -- terraform apply
```

**Rendered files.** Some tools read only a file, not the environment. An `env`
secret can render one: answer `y` to "Render a credentials file?" and supply a
basename, the env var that should hold the file's path, and a Go
`text/template` body referencing the value keys as `{{ .KEY }}`:

```
Render a credentials file? [y/N]: y
  File basename (e.g. credentials): credentials
  Env var to receive the file path (e.g. AWS_SHARED_CREDENTIALS_FILE): AWS_SHARED_CREDENTIALS_FILE
  File template — reference values as {{ .KEY }}; end with a line containing only '.':
[default]
aws_access_key_id={{ .AWS_ACCESS_KEY_ID }}
aws_secret_access_key={{ .AWS_SECRET_ACCESS_KEY }}
.
```

At materialization the file is written under `~/.cloak/run/` with mode `0600`,
its path is injected as the named variable, and it is shredded when the
endpoint closes, its TTL expires, or the vault locks.

**Credential helper.** For the AWS CLI you can skip `cloak exec` entirely — see
[`cloak creds`](#cloak-creds).

**Security note.** Unlike the proxied types, an `env` secret's values reach the
process you run. This is the weaker of Cloak's two tiers — see
[Security model](#9-security-model).

---

## 6. Configuration

`~/.cloak/config.toml` (or wherever `$CLOAK_CONFIG` points). All fields are
optional; defaults are shown.

```toml
[daemon]
socket_path = "~/.cloak/cloakd.sock"
log_level   = "info"                       # trace|debug|info|warn|error

[vault]
idle_timeout = "1h"                        # auto-lock after this idle period;
                                           # clamped to >= 5m

[endpoints]
default_persistent_port_start = 54200      # persistent secrets with no
                                           # configured port get assigned
                                           # from this range upward

[ssh]
host_key_dir = "~/.cloak/host_keys"        # where Cloak stores the host keys
                                           # it serves to SSH clients

[audit]
log_path = "~/.cloak/audit.log"
```

`~` and `~/` are expanded against the user's home directory.

To change configuration: edit the file, then restart the daemon. There's no
live reload in v1.

---

## 7. Environment variables

| Variable | Effect |
|---|---|
| `CLOAK_HOME` | Override the default `~/.cloak` location. Daemon and CLI both honour it. Useful for testing, multi-vault setups, and CI. |
| `CLOAK_CONFIG` | Override the config file path directly (otherwise it's `$CLOAK_HOME/config.toml`). |
| `CLOAK_TOKEN` | CLI bearer token. Overrides the on-disk `~/.cloak/cli_token`. |
| `CLOAKD_BIN` | CLI uses this to locate the `cloakd` binary instead of searching `$PATH`. |

---

## 8. Files on disk

Everything lives under `~/.cloak/` (or `$CLOAK_HOME`).

| File | Purpose | Permissions | Contents |
|---|---|---|---|
| `vault.db` | SQLite database. | 0600 | Secrets table with **only `secret_blob` encrypted**. Names, types, hosts, ports are plaintext. Tokens stored as Argon2id hashes. |
| `vault.meta.json` | Vault metadata. | 0600 | KDF parameters, salt, wrapped DEK. **No secret material.** |
| `config.toml` | User config. | 0600 (recommended) | Daemon settings. |
| `audit.log` | Audit log. | 0600 | Hash-chained JSONL. No payload content. |
| `host_keys/` | SSH adapter host keys. | 0700 dir, 0600 keys | Generated on first SSH adapter use. |
| `run/` | Rendered files for materialized (`env`) secrets. | 0700 dir, 0600 files | One subdirectory per open materialization. Shredded on close; the whole directory is emptied on daemon start. |
| `cloakd.sock` | Daemon's Unix socket. | 0600 | Removed on clean shutdown. |
| `cloakd.pid` | PID file. | 0600 | For `cloak daemon stop` / `status`. |
| `cli_token` | CLI's bearer token (if you used `--save`). | 0600 | Plaintext token. Treat like an SSH key. |

The whole directory is created with mode 0700 the first time `cloakd` runs.

---

## 9. Security model

What Cloak protects:

1. **Real credentials never reach the client.** Your app, your agent, your
   shell sees `localhost:54200` and a random ephemeral password — never the
   real DB password.
2. **No plaintext credentials on disk.** Secret material is AEAD-encrypted
   (XChaCha20-Poly1305) under a DEK that is itself wrapped with a KEK derived
   from your master password via Argon2id.
3. **In-memory credentials are minimised.** Decrypted credentials live in
   `mlock`-pinned memory only for the duration of one connection, then
   they're overwritten and unlinked.
4. **All access is auditable.** Every connection, every secret operation,
   every token use writes an entry to a hash-chained append-only log.
5. **Endpoints close on lock.** Locking the vault tears down every listener
   and cancels every in-flight connection.

**A weaker tier — materialized (`env`) secrets.** Properties 1 and 3 above hold
only for *proxied* secrets (`postgres`, `mysql`, `ssh`, `http`). An `env` secret
is *injected*, not proxied: its values reach the child process's environment,
and any rendered file lands in `~/.cloak/run/` on disk (mode `0600`, shredded on
close, expiry, or lock). For `env` secrets Cloak still gives you encryption at
rest, one-place rotation, auto-lock, and an audit trail of every
materialization — but the credential itself is visible to the process you ran,
to anything that can read its environment, and to crash dumps of it. Use `env`
secrets when a tool cannot be proxied; prefer a proxied type whenever one
exists. The audit log marks the difference (`secret.materialized` events, and
the `kind` field on endpoints).

**A deliberate exception — `secret reveal`.** Property 1 says credentials never
reach the client. The one intended exception is `cloak secret reveal` (and the
desktop app's **Reveal** button), which decrypts and shows the stored credential
so Cloak can serve as a password manager. It is gated so it cannot become an
agent-reachable hole: the daemon re-checks the **master password** before
decrypting — a client token alone is not enough, and an AI agent does not hold
the master password — and every reveal (and every failed attempt) is written to
the audit log. Reveal hands plaintext to whatever process asked for it, so use
it from a human-facing client (the CLI, the GUI), not an automated one.

What Cloak does *not* protect against (v1 limitations, by design):

- **A compromised daemon process.** If an attacker can inject code into the
  running `cloakd`, they have the DEK. The defence is OS-level: keep your
  machine patched, your other software trusted.
- **A compromised daemon-running user account.** Anything that user can do —
  including reading `~/.cloak/`, attaching a debugger, dumping memory —
  defeats the at-rest protection.
- **Per-operation policy.** v1 is binary: an endpoint is open, or it isn't.
  v2 will add per-SQL-statement, per-SSH-command, per-HTTP-request policy
  with confirmation flows.
- **Content inspection / redaction.** Cloak proxies bytes; it doesn't read
  them. SQL queries, SSH commands, and HTTP bodies pass through unmodified.
- **Network attacks against the upstream.** TLS validation is enforced for
  Postgres/MySQL (per `tls_mode`) and SSH host-key fingerprints are pinned —
  but if you misconfigure the fingerprint or trust the wrong CA, Cloak
  trusts what you told it to.

Stronger storage (full-DB encryption via SQLCipher) and biometric/Keychain
unlock are on the v1.1 roadmap. Hardware tokens are v3.x.

---

## 10. Troubleshooting

### `daemon unreachable at /Users/.../cloakd.sock — is "cloak daemon start" running?`

The daemon isn't running, or its socket is stale. Check:

```bash
cloak daemon status
ls -la ~/.cloak/
```

If the PID file claims it's running but `daemon status` says it isn't, the
process died without cleaning up. Remove the stale files:

```bash
rm ~/.cloak/cloakd.pid ~/.cloak/cloakd.sock
cloak daemon start
```

### `vault_locked: vault is locked`

Self-explanatory. The vault auto-locked from idleness, or you `cloak lock`'d
it explicitly.

```bash
cloak unlock
```

### `unauthorized: invalid token` after `cloak unlock`

Your stored CLI token has been revoked, or your `~/.cloak/cli_token` is stale.

```bash
cloak token list                          # see which tokens exist
cloak token create --name shell --save    # issue a new one
```

### `vault_not_initialized: vault is not initialized; run \`cloak init\``

There's no vault yet. Initialise it.

### `name_conflict: secret "prod-db" already exists`

You already have a secret with that name. Either pick a different name or
delete the existing one (`cloak secret delete prod-db`).

### `adapter_error: ssh: host key mismatch: got SHA256:..., expected SHA256:...`

The upstream SSH server's host key doesn't match the fingerprint you stored.
This is either a real attack or — much more likely — the server's keys
genuinely changed. Verify out-of-band that the new key is correct, then
update the secret:

```bash
ssh-keyscan -t ed25519 prod-server.internal | ssh-keygen -lf -
# edit the secret via secrets.update over JSON-RPC, or recreate it
```

### Endpoint listed but client gets "connection refused"

This usually means the vault locked between the listing and your connection
attempt. Run `cloak status`; if locked, `cloak unlock` re-opens persistent
endpoints (with fresh ephemeral credentials — old connection strings won't
work).

### Audit log has gaps in `seq`

This shouldn't happen — sequence numbers are monotonic and assigned under a
mutex. If you see one, the audit log was probably edited by hand. The hash
chain in `prev_hash` will surface the tamper.

### `i/o timeout` on an interactive `cloak secret add` or `cloak init`

Fixed in the current build. If you're on an older binary, rebuild from
`main`.

### Need more detail

Set `daemon.log_level = "debug"` in `config.toml` and restart the daemon.
Operational logs go to stderr (or `~/.cloak/cloakd.log` if redirected).

---

## 11. FAQ

**Can I use Cloak with non-Cloak-aware tooling?**
Yes — that's the whole point. Anything that takes a `DATABASE_URL` or an SSH
host works, because the local endpoint speaks the real protocol.

**Can two machines share a vault?**
Not in v1. The vault is local. Team profile distribution is v3.0.

**Can I use Cloak as a plain password manager?**
Yes. Store any credential as a secret, and read it back when you need it with
`cloak secret reveal <name>` or the desktop app's **Reveal** button. Reveal
re-checks your master password and is audit-logged, so the encrypted-at-rest,
one-place-rotation, and auto-lock guarantees apply to passwords you just want
to *keep* — not only ones Cloak proxies.

**Can I script `cloak init` / `unlock` non-interactively?**
Yes, with `--from-stdin`. Use this carefully — anything that puts a
password on stdin has to keep it out of shell history and process listings.

**Does Cloak see my SQL queries / SSH commands / HTTP bodies?**
No, not in v1. It proxies bytes after authentication. Per-operation
inspection and policy is v2.

**What if `cloakd` crashes?**
On restart, the vault returns to `locked`. The PID file is left behind but
`cloak daemon stop` / `status` detect that and treat it as not-running. On
the next `cloak daemon start` the stale socket is replaced.

**Is the daemon multi-tenant?**
No. One daemon per OS user. The Unix socket's 0600 mode is the boundary.

**Can I rotate the master password?**
Not via the CLI in v1. The vault schema supports it (re-wrap the DEK under a
new KEK), but the RPC isn't exposed yet.

**Does Cloak support a hardware token (YubiKey, etc.)?**
Not in v1. The vault meta has an `unlock_methods` field reserved for this;
v3.x will add it.

**Can I export / back up the vault?**
Yes — copy `~/.cloak/vault.db` and `~/.cloak/vault.meta.json`. Without the
master password, the copy is useless.

**Where can I file bugs?**
Open an issue at https://github.com/Chekunin/cloak.
