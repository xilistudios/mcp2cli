#!/bin/sh
# mcp2cli universal installer
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/xilistudios/mcp2cli/main/install.sh | sh
#   curl -fsSL ... | sh -s -- --version v1.0.0
#   curl -fsSL ... | sh -s -- --prefix /usr/local
set -eu

REPO="xilistudios/mcp2cli"
BINARY="mcp2cli"
DEFAULT_PREFIX="${HOME}/.local"

# ── Helpers ──────────────────────────────────────────────────────────

log()   { printf '  %s\n' "$@"; }
info()  { printf '\033[1;34m==> %s\033[0m\n' "$@"; }
ok()    { printf '\033[1;32m==> %s\033[0m\n' "$@"; }
err()   { printf '\033[1;31mError: %s\033[0m\n' "$@" >&2; exit 1; }

need() {
  command -v "$1" >/dev/null 2>&1 || err "'$1' is required but not found."
}

http_get() {
  url="$1"; dst="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "$dst" "$url"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$dst" "$url"
  else
    err "curl or wget is required."
  fi
}

http_get_stdout() {
  url="$1"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO- "$url"
  else
    err "curl or wget is required."
  fi
}

# ── Detect platform ─────────────────────────────────────────────────

detect_os() {
  os="$(uname -s)"
  case "$os" in
    Linux*)   echo "Linux" ;;
    Darwin*)  echo "Darwin" ;;
    *) err "Unsupported OS: $os (only Linux and macOS are supported)" ;;
  esac
}

detect_arch() {
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64)   echo "x86_64" ;;
    aarch64|arm64)   echo "arm64" ;;
    *) err "Unsupported architecture: $arch (only x86_64 and arm64 are supported)" ;;
  esac
}

# ── Resolve latest version ──────────────────────────────────────────

latest_version() {
  resp=$(http_get_stdout "https://api.github.com/repos/${REPO}/releases/latest")
  tag=$(printf '%s' "$resp" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
  [ -n "$tag" ] || err "Could not determine latest release."
  echo "$tag"
}

# ── Parse arguments ─────────────────────────────────────────────────

VERSION=""
PREFIX=""
while [ $# -gt 0 ]; do
  case "$1" in
    --version|-v)   VERSION="$2"; shift 2 ;;
    --prefix|-p)    PREFIX="$2"; shift 2 ;;
    --help|-h)
      cat <<EOF
mcp2cli Installer

Options:
  --version, -v   Install a specific version (e.g. v1.0.0)
  --prefix,  -p   Installation prefix (default: ~/.local)
  --help,    -h   Show this help

The binary is placed in <prefix>/bin/mcp2cli.
EOF
      exit 0
      ;;
    *) err "Unknown option: $1" ;;
  esac
done

PREFIX="${PREFIX:-$DEFAULT_PREFIX}"
BIN_DIR="${PREFIX}/bin"

# ── Main ─────────────────────────────────────────────────────────────

OS=$(detect_os)
ARCH=$(detect_arch)

info "Detected platform: ${OS}/${ARCH}"

if [ -z "$VERSION" ]; then
  info "Fetching latest release..."
  VERSION=$(latest_version)
fi

# Strip leading 'v' for archive name
VERSION_NUM="${VERSION#v}"

ARCHIVE="${BINARY}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"

info "Downloading ${BINARY} ${VERSION} ..."
log "$URL"

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

http_get "$URL" "${TMPDIR}/${ARCHIVE}"

# Verify checksum
CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY}_${VERSION_NUM}_checksums.txt"
info "Verifying checksum..."
http_get "$CHECKSUMS_URL" "${TMPDIR}/checksums.txt"

EXPECTED=$(grep "${ARCHIVE}" "${TMPDIR}/checksums.txt" | awk '{print $1}')
if [ -z "$EXPECTED" ]; then
  err "Archive '${ARCHIVE}' not found in checksums file."
fi

if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL=$(sha256sum "${TMPDIR}/${ARCHIVE}" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL=$(shasum -a 256 "${TMPDIR}/${ARCHIVE}" | awk '{print $1}')
else
  log "Warning: sha256sum/shasum not found, skipping checksum verification."
  ACTUAL="$EXPECTED"
fi

if [ "$ACTUAL" != "$EXPECTED" ]; then
  err "Checksum mismatch!\n  Expected: ${EXPECTED}\n  Got:      ${ACTUAL}"
fi
log "Checksum OK"

# Extract
info "Extracting..."
need tar
mkdir -p "${TMPDIR}/out"
tar -xzf "${TMPDIR}/${ARCHIVE}" -C "${TMPDIR}/out"

# Install
mkdir -p "$BIN_DIR"

EXTRACTED="${TMPDIR}/out/${BINARY}"

if [ ! -f "$EXTRACTED" ]; then
  err "Binary not found in archive. Contents: $(ls "${TMPDIR}/out/")"
fi

cp "$EXTRACTED" "${BIN_DIR}/${BINARY}"
chmod +x "${BIN_DIR}/${BINARY}"

ok "Installed ${BINARY} ${VERSION} to ${BIN_DIR}/${BINARY}"

# Check PATH
case ":${PATH}:" in
  *":${BIN_DIR}:"*) ;;
  *)
    log ""
    log "Add the following to your shell profile to put mcp2cli on your PATH:"
    log ""
    log "  export PATH=\"${BIN_DIR}:\$PATH\""
    log ""
    ;;
esac

log "Run 'mcp2cli --help' to get started."
