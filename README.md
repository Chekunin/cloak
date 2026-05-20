# Cloak

Cloak is a local secret broker. It keeps real credentials inside an encrypted
local vault and exposes them to applications and AI agents as **local network
endpoints** that speak the native protocol (Postgres wire, MySQL wire, SSH,
HTTP). The real secret never reaches the client.

For CLI tools that can't be proxied this way — the AWS CLI, `gcloud`,
`kubectl`, `terraform`, and the like — the `env` secret type injects stored
credentials into a child process instead. See the
[user manual](./MANUAL.md#env) for the trade-offs.

This repository contains the **v1** implementation written in Go. See
[`cloak-architecture.md`](./cloak-architecture.md) for the full design
specification.

## Binaries

- `cloakd` — long-running daemon. Holds the unlocked vault and the local
  endpoint listeners.
- `cloak` — stateless CLI client. Talks to `cloakd` over a Unix domain socket.

## Build

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o cloakd ./cmd/cloakd
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o cloak  ./cmd/cloak
```

Both binaries cross-compile cleanly because the project depends on no CGo
packages.

## Quick start

```bash
cloak init                        # create vault, set master password
cloak daemon start                # start the daemon in the background
cloak unlock                      # unlock the vault
cloak secret add postgres prod-db # interactive prompt for connection details
cloak token create --name shell   # issue a client token for this shell
cloak connect prod-db             # opens psql against the local endpoint
```

## License

Apache-2.0. See [`LICENSE`](./LICENSE).
