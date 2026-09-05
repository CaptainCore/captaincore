#!/usr/bin/env bash
#
# CaptainCore CLI installer
#
#   curl -fsSL https://raw.githubusercontent.com/CaptainCore/captaincore/master/install.sh | bash
#
# Downloads the latest release binary for this platform from GitHub, verifies it
# against the release's checksums.txt, and installs it as `captaincore`. The binary
# carries its own runtime scripts and unpacks them into ~/.captaincore on first run.
#
# Options (environment variables, or flags after `bash -s --`):
#   INSTALL_DIR=/usr/local/bin      Where to put the binary (default /usr/local/bin)
#   CAPTAINCORE_VERSION=v1.2.3      Install a specific release instead of the latest
#   CAPTAINCORE_FORCE=1 | --force   Install even when ~/.captaincore is a source checkout
#
# Requirements: curl, tar, and sha256sum or shasum. rclone, restic and git are
# needed by parts of the CLI but are not installed here.

set -euo pipefail

REPO="CaptainCore/captaincore"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
VERSION="${CAPTAINCORE_VERSION:-latest}"
FORCE="${CAPTAINCORE_FORCE:-0}"
BASE_URL="${CAPTAINCORE_BASE_URL:-https://github.com/${REPO}/releases}"

for arg in "$@"; do
    case "$arg" in
        --force) FORCE=1 ;;
        --version=*) VERSION="${arg#--version=}" ;;
        --dir=*) INSTALL_DIR="${arg#--dir=}" ;;
        -h|--help)
            sed -n '2,19p' "$0" 2>/dev/null || true
            exit 0 ;;
        *) echo "Unknown option: $arg" >&2; exit 1 ;;
    esac
done

say()  { printf '%s\n' "$*"; }
fail() { printf 'Error: %s\n' "$*" >&2; exit 1; }

need() {
    command -v "$1" >/dev/null 2>&1 || fail "'$1' is required but not installed."
}
need curl
need tar

detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "Linux" ;;
        Darwin*) echo "Darwin" ;;
        *)       fail "Unsupported operating system: $(uname -s) (Linux and macOS only)" ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)   echo "x86_64" ;;
        arm64|aarch64)  echo "arm64" ;;
        armv7*|armv6*)  echo "armv7" ;;
        *)              fail "Unsupported architecture: $(uname -m)" ;;
    esac
}

sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | awk '{print $1}'
    else
        fail "Need sha256sum or shasum to verify the download."
    fi
}

OS="$(detect_os)"
ARCH="$(detect_arch)"
ASSET="captaincore_${OS}_${ARCH}.tar.gz"

if [ "$VERSION" = "latest" ]; then
    DOWNLOAD_BASE="${BASE_URL}/latest/download"
else
    case "$VERSION" in v*) ;; *) VERSION="v${VERSION}" ;; esac
    DOWNLOAD_BASE="${BASE_URL}/download/${VERSION}"
fi

# A source checkout builds its own binary in place (the CaptainCore server does this).
# Installing a release binary next to it would shadow that build on PATH.
if [ -d "${HOME}/.captaincore/.git" ] && [ "$FORCE" != "1" ]; then
    say "~/.captaincore is a git checkout, so this machine builds captaincore from source."
    say "Update it with:  cd ~/.captaincore && git pull && go build -o captaincore"
    say "To install a release binary anyway, re-run with --force (or CAPTAINCORE_FORCE=1)."
    exit 1
fi

say "CaptainCore CLI installer"
say "  platform:  ${OS} ${ARCH}"
say "  release:   ${VERSION}"
say "  install:   ${INSTALL_DIR}/captaincore"
say ""

TMP="$(mktemp -d 2>/dev/null || mktemp -d -t captaincore)"
trap 'rm -rf "$TMP"' EXIT

say "Downloading ${ASSET}..."
curl -fsSL --retry 3 "${DOWNLOAD_BASE}/${ASSET}" -o "${TMP}/${ASSET}" \
    || fail "Download failed: ${DOWNLOAD_BASE}/${ASSET}"
curl -fsSL --retry 3 "${DOWNLOAD_BASE}/checksums.txt" -o "${TMP}/checksums.txt" \
    || fail "Download failed: ${DOWNLOAD_BASE}/checksums.txt"

say "Verifying checksum..."
expected="$(awk -v f="$ASSET" '$2 == f {print $1}' "${TMP}/checksums.txt")"
[ -n "$expected" ] || fail "${ASSET} is not listed in checksums.txt"
actual="$(sha256_of "${TMP}/${ASSET}")"
[ "$expected" = "$actual" ] || fail "Checksum mismatch for ${ASSET} (expected ${expected}, got ${actual})"

say "Extracting..."
tar -xzf "${TMP}/${ASSET}" -C "$TMP" captaincore
chmod +x "${TMP}/captaincore"

mkdir -p "$INSTALL_DIR" 2>/dev/null || true
if [ -w "$INSTALL_DIR" ]; then
    mv -f "${TMP}/captaincore" "${INSTALL_DIR}/captaincore"
elif command -v sudo >/dev/null 2>&1; then
    say "${INSTALL_DIR} is not writable, using sudo..."
    sudo mkdir -p "$INSTALL_DIR"
    sudo mv -f "${TMP}/captaincore" "${INSTALL_DIR}/captaincore"
else
    fail "${INSTALL_DIR} is not writable and sudo is unavailable. Re-run with INSTALL_DIR=<writable dir>."
fi

say ""
say "Installed: $("${INSTALL_DIR}/captaincore" version | head -1)"

case ":${PATH}:" in
    *":${INSTALL_DIR}:"*) ;;
    *) say "Note: ${INSTALL_DIR} is not on your PATH." ;;
esac

missing=""
for dep in rclone restic git; do
    command -v "$dep" >/dev/null 2>&1 || missing="${missing} ${dep}"
done
[ -z "$missing" ] || say "Note: not installed yet, needed for backups and quicksaves:${missing}"

say ""
say "Run 'captaincore connect' to get started."
