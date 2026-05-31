#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
CODEX_SSH_HOME_DIR="${CODEX_SSH_HOME:-$HOME/.codex/ssh}"

mkdir -p "$CODEX_SSH_HOME_DIR"
mkdir -p \
  "$CODEX_SSH_HOME_DIR/run/control" \
  "$CODEX_SSH_HOME_DIR/run/tunnels" \
  "$CODEX_SSH_HOME_DIR/run/proxies" \
  "$CODEX_SSH_HOME_DIR/run/jobs" \
  "$CODEX_SSH_HOME_DIR/run/askpass"

copy_if_missing() {
  local src="$1"
  local dst="$2"
  if [[ -f "$dst" ]]; then
    echo "keep existing $dst"
  else
    cp "$src" "$dst"
    echo "created $dst"
  fi
}

copy_if_missing "$REPO_DIR/defaults/config.toml" "$CODEX_SSH_HOME_DIR/config.toml"
copy_if_missing "$REPO_DIR/defaults/hosts.example.toml" "$CODEX_SSH_HOME_DIR/hosts.toml"

echo "Runtime files ready in $CODEX_SSH_HOME_DIR"
