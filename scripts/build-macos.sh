#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
ARCH=""
FORMAT="dmg"

usage() {
  cat <<'EOF'
Usage: bash scripts/build-macos.sh --arch arm64|amd64 [--format dmg]

Builds a Wails 3 macOS test DMG. The script must run on macOS because the
native WebKit/CGo build and hdiutil packaging are platform tools.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --arch)
      [ "$#" -ge 2 ] || { echo "--arch requires arm64 or amd64" >&2; exit 2; }
      ARCH=$2; shift 2 ;;
    --format)
      [ "$#" -ge 2 ] || { echo "--format requires dmg" >&2; exit 2; }
      FORMAT=$2; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

[ "$ARCH" = arm64 ] || [ "$ARCH" = amd64 ] || { echo "--arch must be arm64 or amd64" >&2; exit 2; }
[ "$FORMAT" = dmg ] || { echo "only --format dmg is supported" >&2; exit 2; }
[ "$(uname -s)" = Darwin ] || { echo "BLOCKED_BY_EXTERNAL_ENV: macOS is required for native Wails 3 DMG packaging" >&2; exit 3; }

for tool in go node pnpm hdiutil plutil lipo codesign; do
  command -v "$tool" >/dev/null 2>&1 || { echo "missing required tool: $tool" >&2; exit 3; }
done

WAILS3_BIN=${WAILS3_BIN:-wails3}
command -v "$WAILS3_BIN" >/dev/null 2>&1 || { echo "missing wails3 CLI (v3.0.0-beta.18)" >&2; exit 3; }
case "$($WAILS3_BIN version)" in
  v3.0.0-beta.18) ;;
  *) echo "wails3 v3.0.0-beta.18 is required" >&2; exit 3 ;;
esac

cd "$REPO_ROOT/apps/desktop"
export GOWORK=off
pnpm install --frozen-lockfile
"$WAILS3_BIN" generate bindings -ts -i -clean=true
"$WAILS3_BIN" task common:generate:icons

OUT_DIR="bin/macos-$ARCH"
mkdir -p "$OUT_DIR"
"$WAILS3_BIN" task darwin:package:dmg ARCH="$ARCH" BIN_DIR="$OUT_DIR"

DMG="$OUT_DIR/LinkSend.dmg"
[ -s "$DMG" ] || { echo "DMG was not created: $DMG" >&2; exit 1; }
MOUNT_DIR=$(mktemp -d "${TMPDIR:-/tmp}/linksend-dmg.XXXXXX")
cleanup() {
  hdiutil detach "$MOUNT_DIR" -quiet >/dev/null 2>&1 || true
  rmdir "$MOUNT_DIR" >/dev/null 2>&1 || true
}
trap cleanup EXIT
hdiutil attach -nobrowse -readonly -mountpoint "$MOUNT_DIR" "$DMG" >/dev/null
APP="$MOUNT_DIR/LinkSend.app"
[ -x "$APP/Contents/MacOS/LinkSend" ] || { echo "DMG app executable missing" >&2; exit 1; }
plutil -extract CFBundleIdentifier raw -o - "$APP/Contents/Info.plist" | grep -Fx 'com.linksend.desktop' >/dev/null
lipo -info "$APP/Contents/MacOS/LinkSend"
codesign --verify --deep --strict "$APP"

HASH=$(shasum -a 256 "$DMG" | awk '{print $1}')
printf '%s  %s\n' "$HASH" "$(basename "$DMG")" > "$OUT_DIR/SHA256SUMS.txt"
printf 'DMG=%s\nSHA256=%s\nARCH=%s\n' "$REPO_ROOT/apps/desktop/$DMG" "$HASH" "$ARCH"
