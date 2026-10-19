#!/usr/bin/env bash
set -euo pipefail

# Fixtures are created at the workspace root, where the runner invokes
# this script and where task.md expects them.

rm -rf hooks payloads repo repo-feature
mkdir -p payloads .claude  # .claude receives the hook's log, as Claude Code would provide

if [[ -z "${EVAL_HOOK_SOURCE:-}" || ! -f "$EVAL_HOOK_SOURCE/hooks/protect-main-branch.sh" ]]; then
    echo "setup: EVAL_HOOK_SOURCE does not contain the plugin hook; run through scripts/eval-run.sh" >&2
    exit 1
fi
cp -R "$EVAL_HOOK_SOURCE/hooks" ./hooks
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
