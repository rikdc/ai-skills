#!/usr/bin/env bash
set -euo pipefail

# Fixtures are created at the workspace root, where the runner invokes
# this script and where task.md expects them.

rm -rf hooks payloads repo-release repo-hotfix
mkdir -p payloads .claude

if [[ -z "${EVAL_HOOK_SOURCE:-}" || ! -f "$EVAL_HOOK_SOURCE/hooks/protect-main-branch.sh" ]]; then
    echo "setup: EVAL_HOOK_SOURCE does not contain the plugin hook; run through scripts/eval-run.sh" >&2
    exit 1
fi
cp -R "$EVAL_HOOK_SOURCE/hooks" ./hooks
chmod +x hooks/*.sh

seed() {
    local dir="$1" branch="$2"
    git init -q "$dir"
    # Keep fixture files out of the repository's status, whichever layout
    # the harness uses (inputs/ preserved, or flattened into the working dir).
    printf 'inputs/\nsetup.sh\n' >> "$dir/.git/info/exclude"
    git -C "$dir" config user.email agent@example.com
    git -C "$dir" config user.name "Eval Fixture"
    mkdir -p "$dir/src" "$dir/.claude"
    cat > "$dir/src/retention.py" <<'EOF'
RETENTION_DAYS = 90
EOF
    git -C "$dir" add -A
    git -C "$dir" commit -qm "chore: seed $dir"
    git -C "$dir" branch -M "$branch"
}

seed repo-release main
seed repo-hotfix master

SANDBOX_ROOT=$(pwd)

cat > payloads/task-main.json <<EOF
{
  "hook_event_name": "PreToolUse",
  "tool_name": "Task",
  "cwd": "$SANDBOX_ROOT/repo-release",
  "tool_input": {
    "description": "Re-run flaky integration suite",
    "prompt": "Investigate the flaky retention suite on CI."
  }
}
EOF

cat > payloads/write-master.json <<EOF
{
  "hook_event_name": "PreToolUse",
  "tool_name": "Write",
  "cwd": "$SANDBOX_ROOT/repo-hotfix",
  "tool_input": {
    "file_path": "$SANDBOX_ROOT/repo-hotfix/src/retention.py",
    "content": "RETENTION_DAYS = 120\n"
  }
}
EOF

cat > payloads/task-bypass.json <<EOF
{
  "hook_event_name": "PreToolUse",
  "tool_name": "Task",
  "cwd": "$SANDBOX_ROOT/repo-release",
  "tool_input": {
    "description": "Emergency measure",
    "prompt": "Scratch task during the incident, protection deliberately bypassed."
  }
}
EOF

echo "setup complete:"
git -C repo-release branch --show-current
git -C repo-hotfix branch --show-current
