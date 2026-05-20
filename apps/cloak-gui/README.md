# cloak-gui

The Cloak desktop GUI. Tauri 2 + Svelte 5 + TypeScript + Tailwind v4.

This is **Phase 0** of the GUI roadmap — a runnable shell that connects to a
local `cloakd`, displays vault status, and gives us the scaffolding to grow
into the full feature set described in the project plan.

---

## Prerequisites

- **Rust** 1.77+ (stable channel) — `rustup install stable`
- **Go** 1.22+ — to build the `cloakd` daemon the app bundles
- **Node.js** 20+ and **pnpm** 9+ — `npm i -g pnpm`
- **Tauri 2 platform tooling**:
  - macOS: Xcode Command Line Tools
  - Linux: `webkit2gtk-4.1`, `libgtk-3-dev`, `libayatana-appindicator3-dev`,
    `librsvg2-dev` (or your distro's equivalents)
  - Windows: WebView2 runtime (pre-installed on Windows 11), MSVC toolchain

You do **not** need a daemon running beforehand. The app bundles `cloakd`
and starts it automatically on launch (see [Daemon lifecycle](#daemon-lifecycle)).
The GUI resolves the socket the same way the CLI does:
`$CLOAK_HOME/cloakd.sock` or `~/.cloak/cloakd.sock`.

---

## Quick start

```bash
# from the project root
cd apps/cloak-gui
pnpm install                     # install JS deps

# Build the cloakd daemon the app bundles. Tauri's `externalBin` looks for
# it by Rust target triple; this builds the one for your machine. Re-run
# whenever the Go daemon code changes.
mkdir -p src-tauri/binaries
CGO_ENABLED=0 go build -o "src-tauri/binaries/cloakd-$(rustc -vV | sed -n 's/host: //p')" ../../cmd/cloakd

pnpm tauri dev                   # runs Vite + cargo, opens a window
```

On first launch the dev server takes ~30 s while Cargo compiles the shell.
Subsequent rebuilds are fast (Vite HMR for the frontend, incremental
`cargo` for Rust).

The window opens against a frontend served at `http://localhost:1420`. The
Svelte runtime calls `daemon_ping` and `vault_status` on a 1.5 s tick and
renders the result.

The GUI starts the bundled `cloakd` for you — the dashboard should come up
within a second or two. If you prefer to run your own daemon (e.g. one
started with `cloak daemon start`), the GUI detects it and uses that instead.

---

## Daemon lifecycle

A GUI-first user never opens a terminal, so the GUI owns the daemon:

- **On launch** the GUI checks whether a `cloakd` is already reachable on the
  socket. If one is, it adopts it. If not, it spawns the `cloakd` binary
  bundled inside the app (`src-tauri/src/daemon.rs`).
- **On quit** (tray *Quit* or ⌘Q) the GUI stops the daemon — but only if it
  was the one that started it. A daemon you started yourself is left running.
- A daemon the GUI spawned logs to `~/.cloak/cloakd.log`.

This means there is nothing to install or configure separately: the daemon
is an implementation detail the user never sees.

---

## Building & distributing for macOS

The `.app` bundle embeds `cloakd` (via Tauri's `externalBin`), so a single
file is all a user needs.

### One-shot build

From the repository root:

```bash
./scripts/build-macos.sh
```

It builds `cloakd` for Apple Silicon **and** Intel, merges them into a
universal binary, and produces a universal
`Cloak_<version>_universal.dmg` under
`apps/cloak-gui/src-tauri/target/universal-apple-darwin/release/bundle/dmg/`.
That one DMG runs on every Mac.

### Code signing & notarization

A build with no Apple credentials is **unsigned**. It runs, but macOS
Gatekeeper tells your users *"Cloak can't be opened because Apple cannot
check it for malicious software"* — a poor first impression for
non-technical users. For real distribution you must sign and notarize the
app, which needs a paid **Apple Developer account** (US $99/year) and a
**Developer ID Application** certificate.

Tauri signs and notarizes automatically during the build when these
environment variables are set:

| Variable | Purpose |
|---|---|
| `APPLE_SIGNING_IDENTITY` | Developer ID identity, e.g. `Developer ID Application: Your Name (TEAMID)`. |
| `APPLE_ID` | Apple ID email used for notarization. |
| `APPLE_PASSWORD` | An [app-specific password](https://support.apple.com/en-us/102654) for that Apple ID. |
| `APPLE_TEAM_ID` | Your 10-character Apple Developer Team ID. |

Export those, then re-run `./scripts/build-macos.sh`. Tauri signs the app
(the embedded `cloakd` included) with the hardened runtime, submits it to
Apple's notary service, and staples the ticket. The resulting DMG opens
cleanly on any Mac.

(For CI, `APPLE_CERTIFICATE` + `APPLE_CERTIFICATE_PASSWORD` import the
certificate from a base64 blob instead of the login keychain. An App Store
Connect API key — `APPLE_API_ISSUER` / `APPLE_API_KEY` — can stand in for
the Apple-ID notarization variables.)

### Sharing it

With a signed, notarized DMG in hand:

1. Host the `.dmg` where users can download it — a **GitHub Release** is the
   simplest: attach the DMG as a release asset.
2. The user opens the DMG and drags **Cloak** into **Applications**.
3. They launch Cloak. The first run walks them through setting a master
   password; the daemon starts behind the scenes. No terminal, ever.

If you distribute an unsigned build for testing, tell those testers to
right-click the app and choose **Open** the first time — but for real users,
notarize.

---

## Project layout

```
apps/cloak-gui/
├── package.json              # frontend dependencies + scripts
├── tsconfig.json             # strict-mode TS config
├── vite.config.ts            # Vite + Tailwind v4 plugin
├── svelte.config.js          # Svelte 5 runes mode
├── index.html                # webview entry point
├── src/                      # Svelte frontend
│   ├── App.svelte            # shell + dashboard placeholder
│   ├── app.css               # tailwind imports + theme tokens
│   ├── main.ts               # Svelte mount()
│   └── lib/
│       ├── api/              # typed wrappers around invoke()
│       │   ├── transport.ts  # the single `call<T>()` entry point
│       │   ├── types.ts      # TS mirrors of Cloak wire types
│       │   ├── daemon.ts
│       │   ├── vault.ts
│       │   ├── secrets.ts
│       │   ├── endpoints.ts
│       │   ├── tokens.ts
│       │   └── index.ts      # re-export by domain
│       ├── stores/           # Svelte 5 runes-based stores
│       │   ├── connection.svelte.ts  # daemon liveness
│       │   └── vault.svelte.ts       # vault status polling
│       └── components/
│           ├── ConnectionBanner.svelte
│           ├── VaultStateChip.svelte
│           └── StatTile.svelte
└── src-tauri/                # Rust shell
    ├── Cargo.toml
    ├── tauri.conf.json
    ├── build.rs
    ├── capabilities/
    │   └── default.json      # narrow permission set (no `"all": true`)
    ├── icons/                # placeholders; regenerate with `pnpm tauri icon`
    └── src/
        ├── main.rs           # thin entrypoint
        ├── lib.rs            # Tauri builder + plugin wiring
        ├── error.rs          # AppError → serialised wire envelope
        ├── state.rs          # lazy-connect Client behind Mutex
        ├── paths.rs          # mirrors internal/paths
        ├── commands.rs       # #[tauri::command] handlers
        └── client/           # Rust port of pkg/client
            ├── mod.rs        # Client = Arc<Transport> + auth helper
            ├── transport.rs  # newline-JSON-RPC over UnixStream
            ├── methods.rs    # one async fn per daemon RPC
            ├── types.rs      # wire types (must match Go + TS)
            └── error.rs      # ClientError + RpcError
```

---

## How it talks to the daemon

```
Svelte component
   │
   │  api.vault.status()
   ▼
src/lib/api/transport.ts   (call<T>("vault_status"))
   │
   │  Tauri invoke()
   ▼
src-tauri/src/commands.rs  (#[tauri::command] vault_status)
   │
   │  AppState::client()
   ▼
src-tauri/src/client/      (Client → Transport → UnixStream)
   │
   │  JSON-RPC 2.0 newline-framed
   ▼
~/.cloak/cloakd.sock
   │
   ▼
internal/ipc/methods.go::vaultStatusHandler
```

Every call follows that path. Adding a new endpoint requires:

1. A new method in `src-tauri/src/client/methods.rs` (Rust).
2. A new `#[tauri::command]` in `src-tauri/src/commands.rs` registered in
   `src-tauri/src/lib.rs::invoke_handler!`.
3. A new TS wrapper in `src/lib/api/<domain>.ts`.

The corresponding Go handler already exists in `internal/ipc`.

---

## Token bootstrap (Phase 0 behaviour)

The GUI reuses the CLI's saved token (`~/.cloak/cli_token` or
`$CLOAK_TOKEN`) when present. This makes the dev loop trivial:

```bash
cloak token create --name shell --save   # writes ~/.cloak/cli_token
pnpm tauri dev                           # GUI picks it up
```

In Phase 1 we replace this with the GUI's own bootstrap flow:

1. First-run wizard detects no token in the OS keychain.
2. Issues a new one named `cloak-gui`.
3. Persists it via `keyring-rs` (macOS Keychain / Windows Credential
   Manager / Linux Secret Service).
4. Subsequent launches re-read from the keychain.

The current `tokens_create` command already supports the `persist: true`
flag for this; the keychain integration is a Phase 1 wiring task, not a
protocol change.

---

## Scripts

| Command | What it does |
|---|---|
| `pnpm dev` | Frontend-only Vite server (no Tauri window). Useful for UI work without a daemon. |
| `pnpm build` | Run `svelte-check` then build the frontend bundle. |
| `pnpm tauri dev` | Full dev loop — opens a window, hot-reloads Svelte, rebuilds Rust incrementally. |
| `pnpm tauri build` | Produces a release-mode signed bundle for the host OS. |
| `pnpm check` | TypeScript + Svelte type-checking. |
| `pnpm format` | Prettier across `src/`. |
| `pnpm lint` | Prettier check (no writes). |

Rust-side:

```bash
cd src-tauri
cargo fmt
cargo clippy --all-targets -- --deny warnings
cargo test
```

---

## Conventions

- **Permissions are narrow.** `capabilities/default.json` lists *exactly*
  the permissions the GUI needs. No `"all": true` shortcuts.
- **All daemon talk happens from Rust.** The frontend has no direct socket
  access; everything goes via `#[tauri::command]`. This keeps the audit
  surface small and gives us one place to add tracing/observability.
- **Types stay in lockstep.** Wire types are defined in Go
  (`pkg/client/types.go`), Rust (`src-tauri/src/client/types.rs`), and
  TypeScript (`src/lib/api/types.ts`). Adding a field anywhere requires
  adding it in all three. The contract test in `tools/contract-tests/`
  will eventually enforce this in CI.
- **No `unsafe` in the Rust shell.** Forbidden via `Cargo.toml`.
- **Errors carry a stable `code`.** Frontend branches on
  `CommandError.code`; never parses `message`.

---

## Known limitations (Phase 0)

- Only the vault-status dashboard is wired in the UI. The other screens
  (Secrets / Endpoints / Audit / Tokens / Settings) are stubbed by the API
  layer but not yet rendered.
- No tray icon, notifications, or keychain integration yet — those are
  Phase 1/3 tasks.
- Icons are placeholders. `pnpm tauri icon ./path/to/source.png`
  regenerates the full set from any source PNG ≥ 1024×1024.
- Windows: socket discovery returns a Unix-style path. The named-pipe
  variant of `cloakd` is a v1.x deliverable.

---

## See also

- [`../../MANUAL.md`](../../MANUAL.md) — user manual for the daemon + CLI.
- [`../../cloak-architecture.md`](../../cloak-architecture.md) — v1 design spec.
