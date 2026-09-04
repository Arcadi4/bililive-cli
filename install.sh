#!/usr/bin/env bash
# bililive installer
#
#   curl -fsSL https://raw.githubusercontent.com/Arcadi4/bililive-cli/HEAD/install.sh | bash
#
# Pin a release:  curl -fsSL .../install.sh | bash -s -- v0.1.0
#
# Env overrides:
#   BILILIVE_VERSION      release tag (default: latest release)
#   BILILIVE_INSTALL_DIR  install dir (default: /usr/local/bin or ~/.local/bin)

# Re-exec under bash when launched from sh or zsh. The script uses bash
# features such as pipefail. `sh install.sh` fails on dash-based systems.
if [ -z "${BASH_VERSION:-}" ]; then
  exec bash "$0" "$@"
fi

set -euo pipefail

REPO="Arcadi4/bililive-cli"
BINARY="bililive"

log() { printf '==> %s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
fail() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

have() { command -v "$1" >/dev/null 2>&1; }
need() { have "$1" || fail "$1 is required to install ${BINARY}"; }

fetch() { # fetch takes <url> and <outfile>
  if have curl; then
    curl -fsSL --proto '=https' --retry 3 --retry-delay 2 -o "$2" "$1"
  elif have wget; then
    wget -q -O "$2" "$1"
  else
    fail "curl or wget is required to install ${BINARY}"
  fi
}

latest_tag() {
  local tag=""
  if have curl; then
    tag=$(curl -fsSL -H 'User-Agent: bililive-installer' \
      "https://api.github.com/repos/${REPO}/releases/latest" |
      sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1) || tag=""
    if [ -z "$tag" ]; then
      # The API can be down or rate limited. Fall back to the
      # releases/latest redirect, which needs no API quota.
      tag=$(curl -fsSI -o /dev/null -w '%{redirect_url}' \
        "https://github.com/${REPO}/releases/latest" |
        sed -n 's#^.*/tag/\([^/[:space:]]*\).*$#\1#p') || tag=""
    fi
  elif have wget; then
    tag=$(wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" |
      sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1) || tag=""
    if [ -z "$tag" ]; then
      tag=$(wget -q --server-response --spider "https://github.com/${REPO}/releases/latest" 2>&1 |
        sed -n 's#.*[Ll]ocation:.*releases/tag/##p' | head -n 1 | tr -d '\r\n ') || tag=""
    fi
  fi
  [ -n "$tag" ] || fail "could not determine the latest release; pin one with BILILIVE_VERSION (e.g. v0.1.0)"
  printf '%s\n' "$tag"
}

case "${1:-}" in
-h | --help)
  printf 'usage: curl -fsSL https://raw.githubusercontent.com/%s/HEAD/install.sh | bash\n' "$REPO"
  printf '       install.sh [tag]           # pin a release, e.g. install.sh v0.1.0\n'
  printf '\nenv: BILILIVE_VERSION, BILILIVE_INSTALL_DIR\n'
  exit 0
  ;;
esac

need uname
need mktemp
need tar
need install

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# --- platform ---------------------------------------------------------------
case "$(uname -s)" in
Darwin) goos=darwin ;;
Linux) goos=linux ;;
MINGW* | MSYS* | CYGWIN* | "Windows_NT")
  fail "windows is not covered by this installer; grab the zip from https://github.com/${REPO}/releases"
  ;;
*)
  fail "unsupported operating system: $(uname -s)"
  ;;
esac

case "$(uname -m)" in
x86_64 | amd64) goarch=amd64 ;;
arm64 | aarch64) goarch=arm64 ;;
*)
  fail "unsupported architecture: $(uname -m)"
  ;;
esac

# macOS under Rosetta reports x86_64. Prefer the native arm64 build.
if [ "$goos" = darwin ] && [ "$goarch" = amd64 ] &&
  [ "$(sysctl -n hw.optional.arm64 2>/dev/null || echo 0)" = 1 ]; then
  goarch=arm64
fi

# --- version ----------------------------------------------------------------
tag="${BILILIVE_VERSION:-${1:-}}"
if [ -z "$tag" ]; then
  log "resolving latest release"
  tag=$(latest_tag)
fi
case "$tag" in
v*) ;;
*) tag="v${tag}" ;;
esac
case "$tag" in
*[!v0-9A-Za-z._-]*) fail "invalid version tag: ${tag}" ;;
esac

# --- download & verify ------------------------------------------------------
base="https://github.com/${REPO}/releases/download/${tag}"
asset="${BINARY}_${tag}_${goos}_${goarch}.tar.gz"

log "installing ${REPO} ${tag} (${goos}/${goarch})"
fetch "${base}/${asset}" "${tmp}/${asset}" ||
  fail "failed to download ${base}/${asset} (bad tag or missing asset?)"
fetch "${base}/checksums.txt" "${tmp}/checksums.txt" ||
  fail "failed to download ${base}/checksums.txt"

if [ "$goos" = darwin ]; then
  need shasum
  actual=$(shasum -a 256 "${tmp}/${asset}" | awk '{print $1}')
else
  need sha256sum
  actual=$(sha256sum "${tmp}/${asset}" | awk '{print $1}')
fi
expected=$(awk -v f="${asset}" '$2 == f {print $1; exit}' "${tmp}/checksums.txt")
[ -n "$expected" ] || fail "${asset} is not listed in checksums.txt of release ${tag}"
if [ "$actual" != "$expected" ]; then
  fail "checksum mismatch for ${asset}
  expected: ${expected}
  actual:   ${actual}"
fi

# --- install ----------------------------------------------------------------
install_dir="${BILILIVE_INSTALL_DIR:-}"
if [ -n "$install_dir" ]; then
  mkdir -p "$install_dir"
elif [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
  install_dir=/usr/local/bin
else
  install_dir="${HOME}/.local/bin"
  mkdir -p "$install_dir"
fi

tar -xzf "${tmp}/${asset}" -C "$tmp"
install -m 0755 "${tmp}/${BINARY}" "${install_dir}/${BINARY}"

case ":$PATH:" in
*":${install_dir}:"*) ;;
*)
  warn "${install_dir} is not on your PATH"
  printf '    add this to your shell profile: export PATH="%s:$PATH"\n' "$install_dir" >&2
  ;;
esac

log "installed ${install_dir}/${BINARY}"
"$install_dir/${BINARY}" --version
