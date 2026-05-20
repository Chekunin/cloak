#!/usr/bin/env bash
#
# Build the Cloak desktop app for macOS as a single universal .dmg.
#
# It builds the Go daemon (cloakd) for Apple Silicon and Intel, merges them
# into a universal binary where Tauri's `externalBin` bundler expects it, and
# builds the universal app.
#
# Two optional, independent signing layers kick in from environment variables:
#
#   * Apple code signing + notarization — set APPLE_SIGNING_IDENTITY (and the
#     APPLE_ID / APPLE_PASSWORD / APPLE_TEAM_ID notarization vars). Without
#     these the app is unsigned and macOS Gatekeeper warns your users.
#
#   * In-app updater artifacts — set TAURI_SIGNING_PRIVATE_KEY (the minisign
#     update-signing key, see apps/cloak-gui/README.md). With it the script
#     also produces Cloak.app.tar.gz + .sig and a latest.json manifest so the
#     app's "Check for Updates" can upgrade existing installs.
#
# Usage:  ./scripts/build-macos.sh

set -euo pipefail

cd "$(dirname "$0")/.."
REPO_ROOT="$(pwd)"
GUI_DIR="$REPO_ROOT/apps/cloak-gui"
BIN_DIR="$GUI_DIR/src-tauri/binaries"
CONF="$GUI_DIR/src-tauri/tauri.conf.json"

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
lipo -create -output "$BIN_DIR/cloakd-universal-apple-darwin" \
  "$BIN_DIR/cloakd-aarch64-apple-darwin" \
  "$BIN_DIR/cloakd-x86_64-apple-darwin"
chmod +x "$BIN_DIR"/cloakd-*

UPDATER=0
if [[ -n "${TAURI_SIGNING_PRIVATE_KEY:-}" ]]; then
  UPDATER=1
  echo "==> Updater artifacts: ENABLED (TAURI_SIGNING_PRIVATE_KEY detected)"
  PUBKEY="$(node -p "require('$CONF').plugins.updater.pubkey")"
  if [[ "$PUBKEY" == REPLACE_WITH* ]]; then
    echo "error: plugins.updater.pubkey in tauri.conf.json is still the placeholder." >&2
    echo "       Run 'pnpm tauri signer generate' and paste its PUBLIC key there" >&2
    echo "       before building updater artifacts — otherwise the shipped app" >&2
    echo "       cannot verify any update." >&2
    exit 1
  fi
else
  echo "==> Updater artifacts: disabled (set TAURI_SIGNING_PRIVATE_KEY to enable)"
fi

if [[ -n "${APPLE_SIGNING_IDENTITY:-}${APPLE_CERTIFICATE:-}" ]]; then
  echo "==> Apple code signing: ENABLED"
else
  echo "==> Apple code signing: disabled — the app will be unsigned."
fi

echo "==> Building the Cloak app (universal)"
cd "$GUI_DIR"
pnpm install --frozen-lockfile
if [[ "$UPDATER" == "1" ]]; then
  pnpm tauri build --target universal-apple-darwin \
    --config '{"bundle":{"createUpdaterArtifacts":true}}'
else
  pnpm tauri build --target universal-apple-darwin
fi

BUNDLE="$GUI_DIR/src-tauri/target/universal-apple-darwin/release/bundle"
echo
echo "==> Done."
for dmg in "$BUNDLE"/dmg/*.dmg; do
  [[ -f "$dmg" ]] && echo "    DMG: $dmg"
done

if [[ "$UPDATER" == "1" ]]; then
  VERSION="$(node -p "require('$CONF').version")"
  TARBALL="$BUNDLE/macos/Cloak.app.tar.gz"
  if [[ -f "$TARBALL.sig" ]]; then
    SIG="$(cat "$TARBALL.sig")"
    URL="https://github.com/Chekunin/cloak/releases/download/v$VERSION/Cloak.app.tar.gz"
    cat > "$BUNDLE/latest.json" <<JSON
{
  "version": "$VERSION",
  "notes": "Cloak $VERSION — see the release page for details.",
  "pub_date": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "platforms": {
    "darwin-aarch64": { "signature": "$SIG", "url": "$URL" },
    "darwin-x86_64": { "signature": "$SIG", "url": "$URL" }
  }
}
JSON
    echo "    Updater tarball: $TARBALL"
    echo "    Update manifest: $BUNDLE/latest.json"
    echo
    echo "    To publish the update: create a GitHub release tagged v$VERSION"
    echo "    and upload these three assets — the .dmg, Cloak.app.tar.gz, and"
    echo "    latest.json. Existing users get it via 'Check for Updates'."
  else
    echo "    WARNING: updater signature not found ($TARBALL.sig)" >&2
  fi
fi
