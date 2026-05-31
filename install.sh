#!/usr/bin/env bash
set -euo pipefail

# codex-ssh installer
# Usage: curl -fsSL https://raw.githubusercontent.com/xlyoung/codex-ssh/main/install.sh | bash
#
# Detects OS and architecture, downloads the latest release from GitHub,
# and installs the binary to ~/.local/bin.

REPO="xlyoung/codex-ssh"
BINARY="codex-ssh"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# --- Helpers ---

info()  { printf '\033[1;34m[INFO]\033[0m  %s\n' "$*"; }
ok()    { printf '\033[1;32m[OK]\033[0m    %s\n' "$*"; }
warn()  { printf '\033[1;33m[WARN]\033[0m  %s\n' "$*" >&2; }
error() { printf '\033[1;31m[ERROR]\033[0m %s\n' "$*" >&2; exit 1; }

need() {
  command -v "$1" >/dev/null 2>&1 || error "Required command '$1' not found. Please install it."
}

# --- Detect platform ---

detect_os() {
  local os
  os="$(uname -s)"
  case "$os" in
    Linux*)  echo "linux" ;;
    Darwin*) echo "darwin" ;;
    *)       error "Unsupported OS: $os" ;;
  esac
}

detect_arch() {
  local arch
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64)   echo "amd64" ;;
    aarch64|arm64)  echo "arm64" ;;
    armv7l|armhf)   echo "armv7" ;;
    *)               error "Unsupported architecture: $arch" ;;
  esac
}

# --- Resolve latest version ---

resolve_latest_version() {
  local version
  # GitHub API: latest release tag
  version=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | head -1 | cut -d '"' -f 4)
  if [ -z "$version" ]; then
    error "Failed to resolve latest release version from GitHub."
  fi
  echo "$version"
}

# --- Main ---

main() {
  need curl
  need uname

  local os arch version archive_url tmp_dir tmp_file

  os="$(detect_os)"
  arch="$(detect_arch)"
  version="${1:-$(resolve_latest_version)}"

  # Strip leading 'v' for filename
  local version_tag="${version#v}"

  info "Detected platform: ${os}/${arch}"
  info "Version to install: ${version}"

  archive_url="https://github.com/${REPO}/releases/download/${version}/${BINARY}_${version_tag}_${os}_${arch}.tar.gz"
  info "Downloading: ${archive_url}"

  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "$tmp_dir"' EXIT

  tmp_file="${tmp_dir}/${BINARY}.tar.gz"
  curl -fsSL -o "$tmp_file" "$archive_url" || error "Download failed. Check that the release exists: ${archive_url}"

  info "Extracting..."
  tar -xzf "$tmp_file" -C "$tmp_dir"

  # Find the binary (may be nested in archive)
  local binary_path
  binary_path="$(find "$tmp_dir" -name "$BINARY" -type f -perm +111 | head -1)"
  if [ -z "$binary_path" ]; then
    # Fallback: look for the binary in root of extracted tar
    binary_path="${tmp_dir}/${BINARY}"
  fi

  if [ ! -f "$binary_path" ]; then
    error "Binary not found in archive. Contents:\n$(ls -la "$tmp_dir")"
  fi

  chmod +x "$binary_path"

  mkdir -p "$INSTALL_DIR"
  mv "$binary_path" "${INSTALL_DIR}/${BINARY}"

  ok "Installed ${BINARY} ${version} to ${INSTALL_DIR}/${BINARY}"

  # Check if in PATH
  if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
    warn "${INSTALL_DIR} is not in your PATH."
    warn "Add this to your shell profile:"
    warn "  export PATH=\"\$HOME/.local/bin:\$PATH\""
  fi

  echo ""
  ok "Done! Run '${BINARY} --help' to get started."
}

main "$@"
