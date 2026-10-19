#!/usr/bin/env bash
set -euo pipefail

# Resolve the fixture directory before changing into it: the invocation path may be relative.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

rm -rf hooks payloads repo repo-feature
mkdir -p payloads .claude  # .claude receives the hook's log, as Claude Code would provide

if [[ -f "$SCRIPT_DIR/../../../hooks/mcp-security-scanner.sh" ]]; then
    cp -R "$SCRIPT_DIR/../../../hooks" ./hooks
else
    cp -R "$SCRIPT_DIR/hook-src/hooks" ./hooks
fi
chmod +x hooks/*.sh

make_repo() {
    local dir="$1"
    git init -q "$dir"
    # Keep fixture files out of the repository's status, whichever layout
    # the harness uses (inputs/ preserved, or flattened into the working dir).
    printf 'inputs/\nsetup.sh\n' >> "$dir/.git/info/exclude"
    git -C "$dir" config user.email agent@example.com
    git -C "$dir" config user.name "Eval Fixture"
    mkdir -p "$dir/selective" "$dir/src"
    cat > "$dir/src/config.py" <<'EOF'
RATE_LIMIT_PER_TENANT = 200
MAX_SIGNED_URL_SECONDS = 604800
EOF
    git -C "$dir" add -A
    git -C "$dir" commit -qm "chore: seed branch protection fixtures"
}

# One repo, two views: main checkout plus a linked worktree on a feature
# branch, exactly how a session editing "just a scratch copy" is judged.
make_repo repo
git -C repo branch -M main
git -C repo branch feature/agent-work main
# Worktree paths are resolved relative to the repo, so ../repo-feature lands
# beside repo/ as the payloads expect.
git -C repo worktree add -q ../repo-feature feature/agent-work

# Write tool payloads judged by .cwd / .tool_input.file_path, per the hook
# contract; absolute paths are embedded at setup time.
SANDBOX_ROOT=$(pwd)

cat > payloads/write-main.json <<EOF
{
  "hook_event_name": "PreToolUse",
  "tool_name": "Write",
  "cwd": "$SANDBOX_ROOT/repo",
  "tool_input": {
    "file_path": "$SANDBOX_ROOT/repo/src/config.py",
    "content": "RATE_LIMIT_PER_TENANT = 350\n"
  }
}
EOF

cat > payloads/write-feature.json <<EOF
{
  "hook_event_name": "PreToolUse",
  "tool_name": "Write",
  "cwd": "$SANDBOX_ROOT/repo-feature",
  "tool_input": {
    "file_path": "$SANDBOX_ROOT/repo-feature/src/config.py",
    "content": "RATE_LIMIT_PER_TENANT = 350\n"
  }
}
EOF

echo "setup complete:"
git -C repo branch --show-current
git -C repo-feature branch --show-current
ls payloads
