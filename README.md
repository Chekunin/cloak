# Cloak

**Use your credentials without ever exposing them.**

Cloak is a local secret broker. It keeps your real credentials in an encrypted
vault and hands them to your apps, scripts, and AI agents through local
endpoints — so the secret itself never lands in a `.env` file, your shell
history, or an AI agent's context window.

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](./LICENSE)
![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)
![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-lightgrey)
![Status](https://img.shields.io/badge/status-v1-success)

---

## Why Cloak exists

Secrets leak through the cracks of everyday development:

- **`.env` files** get committed to Git, copied between projects, and pasted into chat.
- **AI coding agents** that can read your filesystem can read every credential in it.
- **Rotating a credential** means hunting it down across every `.env`, CI config, and teammate's laptop.

The root problem is that the secret is *sitting in plaintext* wherever it's
used. Cloak removes it from those places. Your `.env` holds a reference to a
local endpoint on `127.0.0.1`; the real credential stays encrypted in the
vault. Your tools connect exactly as before — they just never see the secret.

```dotenv
# Before — the real password is right there, in plaintext
DATABASE_URL=postgres://admin:S3cr3t-Pa55w0rd@db.prod.example.com:5432/app

# After — a local endpoint with a throwaway password; the real one stays in the vault
DATABASE_URL=postgres://cloak:ephemeral-local-pw@127.0.0.1:54200/app
```

---

## How it works

Cloak runs a small daemon. For each stored credential it opens a local
listener that speaks that credential's **native protocol**. Your client
connects to `127.0.0.1` with a throwaway local password; Cloak decrypts the
real credential, authenticates upstream with it, and proxies the traffic.

```
  client                       Cloak daemon                  upstream service
  ──────                       ────────────                  ────────────────

  psql · curl · ssh            encrypted vault on disk        your real database,
  your app · an AI agent       master password never          API, or server
                               written anywhere

      │  1. connect to 127.0.0.1   │                              │
      ├───────────────────────────▶│  2. decrypt the credential   │
      │     throwaway password     │     into mlock'd memory      │
      │                            ├─────────────────────────────▶│
      │                            │  3. authenticate upstream    │
      │◀───────────────────────────┼──────────────────────────────┤
      │     proxied traffic — the real secret never comes back     │
```

Nothing about your tooling changes — `psql`, `curl`, `ssh`, DBeaver, JDBC,
your app's database driver all connect the way they always have. Every
connection is recorded in an append-only audit log.

---

## Features

**Five secret types**

| Type | What it does |
|---|---|
| `postgres` · `mysql` | Wire-protocol proxies — works with `psql`, `pgx`, JDBC, ORMs, GUI clients. |
| `ssh` | Password or private-key auth, SFTP, `ssh -L` forwarding, pinned upstream host keys. |
| `http` | Reverse proxy that injects headers / query params — keep an API key out of your code. |
| `env` | Injects credentials into CLI tools that can't be proxied — AWS CLI, `gcloud`, `kubectl`, `terraform`. |

**Security by design**

- Credentials are encrypted at rest with **XChaCha20-Poly1305**; the key is derived from your master password with **Argon2id**.
- The master password is **never persisted** — not to disk, not anywhere.
- Decrypted secrets live in **`mlock`-pinned memory** only for the lifetime of a connection, then they're zeroed.
- An **append-only, hash-chained audit log** records every secret access — tampering is detectable.
- The vault **auto-locks** after a period of inactivity, tearing down every endpoint.

**Built for real workflows**

- **Persistent endpoints** for `.env`-style use, **session endpoints** (TTL-bounded) for scripts and agent sessions.
- `cloak exec` runs any command with credentials wired into its environment; `cloak connect` opens the right native client for you.
- A **desktop app** (macOS) with a one-click "Run" screen and a built-in updater — no terminal required.
- Pure Go, **no CGo** — single static binaries that cross-compile cleanly.

---

## Install

### CLI + daemon (from source)

Requires **Go 1.25+**.

```bash
git clone https://github.com/Chekunin/cloak
cd cloak
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ./bin/cloakd ./cmd/cloakd
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ./bin/cloak  ./cmd/cloak
export PATH="$PWD/bin:$PATH"
```

### Desktop app (macOS)

Download the latest `.dmg` from the
[Releases page](https://github.com/Chekunin/cloak/releases), drag **Cloak**
into Applications, and open it. The app bundles and starts the daemon for you.

---

## Quick start

```bash
cloak daemon start                   # start the background daemon
cloak init                           # create the vault, set a master password
cloak unlock                         # unlock it
cloak token create --name shell --save   # issue this shell a client token
cloak secret add postgres prod-db    # interactive prompt for connection details
cloak connect prod-db                # opens psql against the local endpoint
```

When you're done:

```bash
cloak lock                           # zero the key, close every endpoint
```

That's the whole loop. The [**user manual**](./MANUAL.md) is the full reference.

---

## Usage by example

**Run an app with a database URL wired in — no secret in the environment file:**

```bash
cloak exec --with prod-db -- ./my-app
# my-app sees DATABASE_URL=postgresql://...@127.0.0.1:54200/app
```

**Call a paid API without the key ever touching your code or your shell:**

```bash
cloak secret add http stripe-api          # store the key + an injection rule
cloak exec --with stripe-api -- \
  curl -H "Authorization: Bearer $STRIPE_API_TOKEN" "$STRIPE_API_URL/v1/customers"
# curl sends a local token; Cloak swaps in the real Stripe key upstream
```

**Use the AWS CLI with managed credentials:**

```bash
cloak secret add env aws-prod              # store AWS_ACCESS_KEY_ID, etc.
cloak exec --with aws-prod -- aws s3 ls
```

See [`MANUAL.md`](./MANUAL.md) for per-type recipes (Postgres, MySQL, SSH, HTTP, Env).

---

## Security model

Cloak is precise about what it does and does not protect — read this before you rely on it.

**What Cloak protects**

1. Real credentials never reach the client — your app, your shell, your AI agent see only `127.0.0.1` and a throwaway password.
2. No plaintext credentials on disk outside the AEAD-encrypted vault.
3. Decrypted material is minimised in memory and zeroed after use.
4. Every connection and secret operation is auditable.
5. Locking the vault closes every endpoint immediately.

**What it does not protect against (by design, in v1)**

- A **compromised daemon process or user account** — anything that can read the daemon's memory defeats at-rest protection.
- **Per-operation policy** — v1 access is binary: an endpoint is open or it isn't. Per-statement / per-request policy is on the roadmap.
- The **`env` secret type is a deliberately weaker tier** — its values *are* injected into the child process, so the secret reaches that process. Use it only for tools that can't be proxied; prefer a proxied type when one exists.

The cryptography uses standard, well-reviewed primitives (XChaCha20-Poly1305,
Argon2id, `crypto/rand`). The codebase has **not** had an external security
audit — treat it accordingly.

---

## Documentation

| Document | Contents |
|---|---|
| [`MANUAL.md`](./MANUAL.md) | Day-to-day user manual — every command, every secret type, configuration, troubleshooting. |
| [`cloak-architecture.md`](./cloak-architecture.md) | Full v1 design specification and the post-v1 roadmap. |
| [`apps/cloak-gui/README.md`](./apps/cloak-gui/README.md) | Building, signing, and distributing the desktop app. |

---

## Repository layout

```
cmd/cloakd     the daemon — holds the vault, runs the endpoint listeners
cmd/cloak      the CLI client — stateless, talks to cloakd over a Unix socket
internal/      vault, secret store, adapters, endpoint manager, IPC, audit log
pkg/client     Go client library for the daemon's JSON-RPC API
apps/cloak-gui the desktop app (Tauri 2 + Svelte 5)
```

---

## Project status

Cloak is at **v1**: functional and usable, and under active development. The
design is documented in full in [`cloak-architecture.md`](./cloak-architecture.md),
which also lays out what comes next — per-operation policy, an MCP server for
AI agents, and team secret sharing.

---

## Contributing

Issues and pull requests are welcome. Before a non-trivial change, skim
[`cloak-architecture.md`](./cloak-architecture.md) so the design intent is
clear. Run the test suite with:

```bash
go test ./...
```

---

## License

[Apache-2.0](./LICENSE). © The Cloak Authors.
