#!/usr/bin/env bash
set -euo pipefail

# ── Config from env (set by mise tasks or CI) ──────────────────────
# OS, ARCH select the cross-compilation target (default: current host).
# CC selects the C compiler V shells out to (default: V's own choice).
# VERSION, COMMIT default to the GIT_* values exported by mise.
VERSION=${VERSION:-$GIT_VERSION}
COMMIT=${COMMIT:-$GIT_COMMIT}
OS=${OS:-$(uname -s)}
ARCH=${ARCH:-$(uname -m)}

# ── Normalise OS to V's -os tokens ─────────────────────────────────
case "$OS" in
  linux | Linux)                    vos="linux";   oslabel="linux" ;;
  darwin | Darwin | mac | macos)    vos="macos";   oslabel="macos" ;;
  windows | Windows | *MINGW* | *MSYS*) vos="windows"; oslabel="windows" ;;
  *) echo "unknown OS: $OS" >&2; exit 1 ;;
esac

# ── Normalise ARCH to V's -arch tokens plus a friendly label ───────
case "$ARCH" in
  amd64 | x86_64 | x64) varch="amd64"; arch="x64" ;;
  arm64 | aarch64)      varch="arm64"; arch="arm64" ;;
  *) echo "unknown ARCH: $ARCH" >&2; exit 1 ;;
esac

# ── Derive packaging details ───────────────────────────────────────
label="${oslabel}-${arch}"
if [ "$vos" = "windows" ]; then
  binary="looksy.exe"
  ext=".zip"
else
  binary="looksy"
  ext=".tar.gz"
fi

# ── Build (V cross-compiles to the target via the chosen C compiler) ─
staging="dist/staging"
mkdir -p "$staging"

cc_flag=()
[ -n "${CC:-}" ] && cc_flag=(-cc "$CC")

echo "→ Building $binary (os=$vos arch=$varch cc=${CC:-default})"
v -prod -os "$vos" -arch "$varch" "${cc_flag[@]}" \
  -d looksy_version="$VERSION" -d looksy_commit="$COMMIT" \
  . -o "$staging/$binary"

# ── Package ────────────────────────────────────────────────────────
archive="looksy-${VERSION}-${label}${ext}"
echo "→ Packaging $archive"
if [ "$ext" = ".zip" ]; then
  (cd "$staging" && zip -r "../$archive" .)
else
  tar -czf "dist/$archive" -C "$staging" .
fi

rm -rf "$staging"
echo "✓ dist/$archive"
