#!/usr/bin/env bash
#
# Build the Cloak desktop app for macOS as a single universal .dmg.
#
# It builds the Go daemon (cloakd) for both Apple Silicon and Intel, merges
# them into a universal binary, drops the per-arch binaries where Tauri's
# `externalBin` bundler expects them, and builds the universal app.
#
# Code signing + notarization happen automatically *if* the relevant Apple
# environment variables are set (see the heads-up printed below). Without
# them the build still succeeds, but the resulting app is unsigned and
# macOS Gatekeeper will warn your users — fine for testing, not for
# distribution. See apps/cloak-gui/README.md for the signing setup.
#
# Usage:  ./scripts/build-macos.sh

set -euo pipefail

cd "$(dirname "$0")/.."
REPO_ROOT="$(pwd)"
BIN_DIR="$REPO_ROOT/apps/cloak-gui/src-tauri/binaries"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "error: this script builds the macOS app and must run on macOS." >&2
  exit 1
fi

echo "==> Building cloakd for arm64 and amd64"
mkdir -p "$BIN_DIR"
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" \
  -o "$BIN_DIR/cloakd-aarch64-apple-darwin" ./cmd/cloakd
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w" \
  -o "$BIN_DIR/cloakd-x86_64-apple-darwin" ./cmd/cloakd

echo "==> Merging into a universal cloakd binary"
lipo -create -output "$BIN_DIR/cloakd-universal-apple-darwin" \
  "$BIN_DIR/cloakd-aarch64-apple-darwin" \
  "$BIN_DIR/cloakd-x86_64-apple-darwin"
chmod +x "$BIN_DIR"/cloakd-*

if [[ -n "${APPLE_SIGNING_IDENTITY:-}${APPLE_CERTIFICATE:-}" ]]; then
  echo "==> Code signing: ENABLED (Apple credentials detected)"
else
  echo "==> Code signing: DISABLED — the app will be unsigned."
  echo "    Set APPLE_SIGNING_IDENTITY (+ APPLE_ID / APPLE_PASSWORD / APPLE_TEAM_ID"
  echo "    for notarization) to produce a distributable build."
fi

echo "==> Building the Cloak app (universal)"
cd "$REPO_ROOT/apps/cloak-gui"
pnpm install --frozen-lockfile
pnpm tauri build --target universal-apple-darwin

DMG_DIR="$REPO_ROOT/apps/cloak-gui/src-tauri/target/universal-apple-darwin/release/bundle/dmg"
echo
echo "==> Done."
if compgen -G "$DMG_DIR/*.dmg" > /dev/null; then
  echo "    DMG: $(ls "$DMG_DIR"/*.dmg)"
else
  echo "    Bundle output: $REPO_ROOT/apps/cloak-gui/src-tauri/target/universal-apple-darwin/release/bundle/"
fi
