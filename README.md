<p align="center">
  <img src="./preview-banner.jpg" alt="Cloak — credentials your AI agent never sees" width="100%">
</p>

# Cloak — credentials your AI agent never sees

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](./LICENSE)
![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)
![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-lightgrey)
![Status](https://img.shields.io/badge/status-v1-success)

Cloak keeps your real credentials in an encrypted vault on your computer, and
lets your apps and AI coding tools *use* them without ever *seeing* them.

---

## The problem

More and more, software gets built with an AI assistant — Claude Code, Cursor,
Copilot — writing and running the code for you.

But to do anything real, that AI needs your credentials: the database
password, the API key, the cloud login. So they end up somewhere the AI can
read them — a `.env` file, a config file, or pasted straight into the chat.

The moment a secret sits there in plain text, it can leak:

- the AI reads it, and it flows into its context window and logs;
- it gets committed to GitHub by accident;
- it's left behind in a file you later share or screenshot.

A credential that something else can read is a credential that can get out.

## What Cloak does

Cloak locks your real credentials in an encrypted vault and gives everything
else a **safe stand-in**.

Your app — or your AI assistant — connects to a local address on your own
machine (`127.0.0.1`) using a throwaway password. Cloak quietly swaps in the
real credential behind the scenes and connects to the actual database, API, or
server. Everything works exactly as before.

The real secret never leaves the vault. It can't reach the AI's context, its
logs, your shell history, or a `.env` file — because it was never there.

```
Before:      DATABASE_URL = postgres://admin:S3cr3t-Pa55w0rd@db.example.com:5432/app
With Cloak:  DATABASE_URL = postgres://cloak:throwaway-password@127.0.0.1:54200/app
```

Your AI assistant gets the second line. The first one — your real password —
stays locked in the vault.

---

## How it works (the curious can read on; everyone else can skip ahead)

Cloak runs a small background program — the *daemon*. For each credential you
store, it opens a local listener that speaks that credential's **native
protocol**. A client connects to `127.0.0.1` with a throwaway password; Cloak
decrypts the real credential, authenticates upstream with it, and proxies the
traffic.

```
  client                       Cloak daemon                  upstream service
  ──────                       ────────────                  ────────────────

  psql · curl · ssh            encrypted vault on disk        your real database,
  your app · an AI agent       master password never          API, or server
                               written anywhere

      │  1. connect to 127.0.0.1   │                              │
      ├───────────────────────────▶│  2. decrypt the credential   │
      │     throwaway password     │     into protected memory    │
      │                            ├─────────────────────────────▶│
      │                            │  3. authenticate upstream    │
      │◀───────────────────────────┼──────────────────────────────┤
      │     proxied traffic — the real secret never comes back     │
```

Nothing about your tooling changes — `psql`, `curl`, `ssh`, DBeaver, JDBC,
your app's database driver all connect the way they always have. And every
connection is written to a tamper-evident audit log, so you can see exactly
what used which secret, and when.

---

## Get started

### The easy way — the desktop app (macOS)

1. Download the latest `Cloak.dmg` from the
   [**Releases page**](https://github.com/Chekunin/cloak/releases).
2. Open it and drag **Cloak** into Applications.
3. Launch it, set a master password, and add your first secret — all
   point-and-click. No terminal.

The app runs everything for you in the background.

### The command line — for developers

Build the two binaries (requires **Go 1.25+**):

```bash
git clone https://github.com/Chekunin/cloak
cd cloak
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ./bin/cloakd ./cmd/cloakd
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ./bin/cloak  ./cmd/cloak
export PATH="$PWD/bin:$PATH"
```

Then the five-minute loop:

```bash
cloak daemon start                       # start the background daemon
cloak init                               # create the vault, set a master password
cloak unlock                             # unlock it
cloak token create --name shell --save   # issue this shell a client token
cloak secret add postgres prod-db        # interactive prompt for connection details
cloak connect prod-db                    # opens psql against the local endpoint
```

The [**user manual**](./MANUAL.md) is the complete reference.

---

## What you can store

| Type | What it's for |
|---|---|
| `postgres` · `mysql` | Databases — works with `psql`, `pgx`, JDBC, ORMs, GUI clients. |
| `ssh` | Servers — password or key auth, SFTP, port forwarding, pinned host keys. |
| `http` | APIs — injects an API key into requests, so it stays out of your code. |
| `env` | CLI tools that can't be proxied — the AWS CLI, `gcloud`, `kubectl`, `terraform`. |

---

## A `.env` with no secrets in it

This is the everyday win. Your `.env` today probably looks like this:

```dotenv
DATABASE_URL=postgresql://admin:S3cr3t-Pa55w0rd@db.prod.example.com:5432/app
STRIPE_API_KEY=sk_live_51H4xQ2eZ...realkey...
```

Every value is a real, working secret — and your AI agent, your editor, your
Git history, and anyone you ever share the file with can read all of it.

With Cloak you move those values into the vault **once**, and your `.env`
points at local endpoints instead.

**1. Store the real credentials in Cloak — once.**

```bash
cloak secret add postgres prod-db      # your real database
cloak secret add http     payments-api # your real API key
```

When prompted, choose **persistent** mode, give each a fixed port (say `54200`
and `54100`), and answer **n** to "require local authentication" so the `.env`
value stays stable. Then enter the real host, password, and API key — they go
straight into the encrypted vault. (In the desktop app, you make the same
choices in the Add Secret screen.)

**2. Point your `.env` at the local endpoints.**

```dotenv
# .env — not one real secret left in here
DATABASE_URL=postgresql://cloak@127.0.0.1:54200/app?sslmode=disable
PAYMENTS_API_URL=http://127.0.0.1:54100
```

**3. That's it.** Your database driver needs no change — the same
`DATABASE_URL` variable, it just points at Cloak now. For an API, point your
HTTP client at the local URL. Cloak, running in the background with the vault
unlocked, swaps in the real host, password, and API key on every request.

Your `.env` now holds only loopback addresses. If your AI agent reads it, if
it lands in a Git commit, if you paste it into a chat — there is nothing
sensitive to leak. The real credentials never left the vault.

And you get two things for free:

- **Rotate in one place.** Password changed? Run `cloak secret rotate prod-db`
  once — every `.env` and every service pointing at that endpoint keeps
  working, with no edits and no redeploys.
- **One audit trail.** Every connection through the endpoint is logged — you
  can see what used the credential, and when.

> Declining local authentication is what keeps the `.env` value static. The
> endpoint is still reachable only from your own machine, and only while the
> vault is unlocked. If you want a per-endpoint password as well, don't
> hardcode the URL — launch your app with `cloak exec` instead (see the
> [manual](./MANUAL.md)).

---

## Examples

**Inject credentials at runtime — nothing written to a file at all:**

```bash
cloak exec --with prod-db -- ./my-app
# my-app starts with DATABASE_URL set; the endpoint closes when it exits
```

**Let an API be called without the key ever touching the code or the shell:**

```bash
cloak exec --with stripe-api -- \
  curl -H "Authorization: Bearer $STRIPE_API_TOKEN" "$STRIPE_API_URL/v1/customers"
# curl sends a local token; Cloak swaps in the real Stripe key upstream
```

**Give a tool your cloud credentials, scoped to one command:**

```bash
cloak exec --with aws-prod -- aws s3 ls
```

In each case, whatever runs after `--` — your app, a script, an AI agent's
command — does its job with credentials it never actually receives. See
[`MANUAL.md`](./MANUAL.md) for per-type recipes.

---

## Security

Cloak is precise about what it does and does not protect — worth reading
before you rely on it.

**What Cloak protects**

- Real credentials never reach the client — your app, your shell, your AI agent see only `127.0.0.1` and a throwaway password.
- Credentials are encrypted at rest with **XChaCha20-Poly1305**; the key is derived from your master password with **Argon2id**.
- The master password is **never written anywhere** — not to disk, not to a config file.
- Decrypted secrets live in **`mlock`-pinned memory** only for the lifetime of a connection, then they're zeroed.
- Every secret access is recorded in an **append-only, hash-chained audit log** — tampering is detectable.
- The vault **auto-locks** after inactivity, closing every endpoint.

**What it does not protect against (by design, in v1)**

- A **compromised computer or user account** — anything that can read the daemon's memory defeats at-rest protection.
- **Per-operation rules** — v1 access is all-or-nothing: an endpoint is open, or it isn't. Per-query / per-request policy is on the roadmap.
- The **`env` secret type is a deliberately weaker tier** — its values *are* handed to the program you run, so that program does see the secret. Use it only for tools that can't be proxied; prefer a proxied type when one exists.

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

Pure Go, no CGo — the daemon and CLI are single static binaries that
cross-compile cleanly.

---

## Project status

Cloak is at **v1**: functional and usable, and under active development. The
design is documented in full in [`cloak-architecture.md`](./cloak-architecture.md),
which also lays out what comes next — per-operation policy, a Model Context
Protocol (MCP) server so AI agents can manage secrets directly, and team
secret sharing.

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
