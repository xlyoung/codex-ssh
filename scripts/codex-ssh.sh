#!/usr/bin/env bash
set -euo pipefail

# Auto-detect repo directory from script location
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Allow override via environment variable
if [[ -n "${CODEX_SSH_REPO:-}" ]]; then
  REPO_DIR="$CODEX_SSH_REPO"
fi

if [[ ! -d "$REPO_DIR" ]]; then
  echo "codex-ssh repo not found: $REPO_DIR" >&2
  echo "Set CODEX_SSH_REPO to the repository root before using this skill." >&2
  exit 1
fi

if [[ ! -f "$REPO_DIR/go.mod" || ! -f "$REPO_DIR/cmd/codex-ssh/main.go" ]]; then
  echo "codex-ssh repo is incomplete: $REPO_DIR" >&2
  exit 1
fi

repo_cache_key() {
  printf '%s' "$REPO_DIR" | shasum -a 256 | awk '{print substr($1, 1, 16)}'
}

DEFAULT_BIN_DIR="${HOME}/.codex/ssh/build-cache/$(repo_cache_key)"
BIN_DIR="${CODEX_SSH_BIN_DIR:-$DEFAULT_BIN_DIR}"
BIN_PATH="$BIN_DIR/codex-ssh"
STAMP_PATH="$BIN_DIR/codex-ssh.sha256"

mkdir -p "$BIN_DIR"

source_fingerprint() {
  local files=("$REPO_DIR/go.mod")
  if [[ -f "$REPO_DIR/go.sum" ]]; then
    files+=("$REPO_DIR/go.sum")
  fi

  local dir
  for dir in "$REPO_DIR/cmd" "$REPO_DIR/internal" "$REPO_DIR/pkg"; do
    if [[ -d "$dir" ]]; then
      while IFS= read -r file; do
        files+=("$file")
      done < <(find "$dir" -name '*.go' -type f | sort)
    fi
  done

  if [[ ${#files[@]} -eq 0 ]]; then
    return 1
  fi

  shasum -a 256 "${files[@]}" | shasum -a 256 | awk '{print $1}'
}

needs_rebuild() {
  if [[ ! -x "$BIN_PATH" ]]; then
    return 0
  fi

  local current_fingerprint=""
  current_fingerprint="$(source_fingerprint)"
  if [[ -z "$current_fingerprint" ]]; then
    return 0
  fi
  if [[ ! -f "$STAMP_PATH" ]]; then
    return 0
  fi
  if [[ "$(cat "$STAMP_PATH")" != "$current_fingerprint" ]]; then
    return 0
  fi
  return 1
}

print_doctor() {
  local repo_in_icloud="no"
  if [[ "$REPO_DIR" == *"/Mobile Documents/"* || "$REPO_DIR" == *"com~apple~CloudDocs"* ]]; then
    repo_in_icloud="yes"
  fi

  local fingerprint_status="missing"
  local current_fingerprint=""
  current_fingerprint="$(source_fingerprint || true)"
  if [[ -x "$BIN_PATH" && -f "$STAMP_PATH" && -n "$current_fingerprint" ]]; then
    if [[ "$(cat "$STAMP_PATH")" == "$current_fingerprint" ]]; then
      fingerprint_status="match"
    else
      fingerprint_status="mismatch"
    fi
  fi

  cat <<EOF
codex-ssh wrapper doctor
repo_dir=$REPO_DIR
repo_in_icloud=$repo_in_icloud
bin_dir=$BIN_DIR
bin_path=$BIN_PATH
stamp_path=$STAMP_PATH
bin_exists=$([[ -x "$BIN_PATH" ]] && echo yes || echo no)
stamp_status=$fingerprint_status
config_path=${HOME}/.codex/ssh/config.toml
hosts_path=${HOME}/.codex/ssh/hosts.toml
logs_dir=${HOME}/.codex/ssh/logs
keychain_backend=$([[ "$(uname -s)" == "Darwin" ]] && echo available || echo unsupported)
hint=iCloud 不是主因；更常见的是 stale binary、stale control socket、audit 大日志行或前台展示卡住
next_step: use 'codex-ssh diagnose <alias>' to verify host/auth/via, then 'codex-ssh audit query --format text --host <alias>' for execution truth
EOF
}

if needs_rebuild; then
  (
    cd "$REPO_DIR"
    CGO_ENABLED=0 go build -o "$BIN_PATH" ./cmd/codex-ssh
  )
  source_fingerprint > "$STAMP_PATH"
fi

if [[ "${1:-}" == "doctor" ]]; then
  print_doctor
  if [[ $# -ge 2 ]]; then
    echo
    echo "remote_diagnose:"
    "$BIN_PATH" diagnose "$2"
  fi
  exit 0
fi

if [[ "${1:-}" == "--help" || "${1:-}" == "help" ]]; then
  "$BIN_PATH" "$@"
  cat <<'EOF'

Quick examples:
  (if codex-ssh is not in PATH, replace "codex-ssh" with this wrapper path)
  codex-ssh doctor [<alias>]
  codex-ssh hosts set myserver --host 192.168.1.101 --user admin
  codex-ssh secret set --host 192.168.1.101 --user admin
  codex-ssh shell myserver
  codex-ssh exec myserver -- "uname -a"
EOF
  exit 0
fi

exec "$BIN_PATH" "$@"
