#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
CODEX_HOME_DIR="${CODEX_HOME:-$HOME/.codex}"
TARGET_DIR="$CODEX_HOME_DIR/skills/codex-ssh"

mkdir -p "$TARGET_DIR" "$TARGET_DIR/agents" "$TARGET_DIR/scripts"
mkdir -p "$TARGET_DIR/defaults"

cp "$REPO_DIR/SKILL.md" "$TARGET_DIR/SKILL.md"
cp "$REPO_DIR/agents/openai.yaml" "$TARGET_DIR/agents/openai.yaml"
cp "$REPO_DIR/scripts/codex-ssh.sh" "$TARGET_DIR/scripts/codex-ssh.sh"
cp "$REPO_DIR/scripts/bootstrap_runtime_files.sh" "$TARGET_DIR/scripts/bootstrap_runtime_files.sh"
cp "$REPO_DIR/defaults/config.toml" "$TARGET_DIR/defaults/config.toml"
cp "$REPO_DIR/defaults/hosts.example.toml" "$TARGET_DIR/defaults/hosts.example.toml"

chmod +x "$TARGET_DIR/scripts/codex-ssh.sh"
chmod +x "$TARGET_DIR/scripts/bootstrap_runtime_files.sh"

required_files=(
  "$TARGET_DIR/SKILL.md"
  "$TARGET_DIR/agents/openai.yaml"
  "$TARGET_DIR/scripts/codex-ssh.sh"
  "$TARGET_DIR/scripts/bootstrap_runtime_files.sh"
  "$TARGET_DIR/defaults/config.toml"
  "$TARGET_DIR/defaults/hosts.toml"
)

for file in "${required_files[@]}"; do
  if [[ ! -f "$file" ]]; then
    echo "install failed: missing $file" >&2
    exit 1
  fi
done

echo "Installed codex-ssh skill to $TARGET_DIR"
echo "Next steps:"
echo "  1) Restart Codex to reload skills."
echo "  2) Verify wrapper help (use wrapper path if codex-ssh is not in PATH):"
echo "     $TARGET_DIR/scripts/codex-ssh.sh --help"
echo "  3) For docs examples using 'codex-ssh ...', replace it with:"
echo "     $TARGET_DIR/scripts/codex-ssh.sh ..."
