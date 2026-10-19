#!/usr/bin/env bash
set -euo pipefail

# Resolve the fixture directory before changing into it: the invocation path may be relative.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

rm -rf hooks payloads repo
mkdir -p payloads .claude

if [[ -f "$SCRIPT_DIR/../../../hooks/mcp-security-scanner.sh" ]]; then
    cp -R "$SCRIPT_DIR/../../../hooks" ./hooks
else
    cp -R "$SCRIPT_DIR/hook-src/hooks" ./hooks
fi
chmod +x hooks/*.sh

git init -q repo
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> repo/.git/info/exclude
git -C repo config user.email agent@example.com
git -C repo config user.name "Eval Fixture"
mkdir -p repo/.claude repo/src
cat > repo/src/data.py <<'EOF'
DIMENSIONS = ("tenant", "region", "shard")
EOF
git -C repo add -A
git -C repo commit -qm "chore: seed bash-gating fixture"
# Branch about to be tampered with must exist for the payload to judge.
git -C repo branch -M main
git -C repo branch wip-lab main

SANDBOX_ROOT=$(pwd)

make_payload() {
    local id="$1" command="$2"
    cat > "payloads/${id}.json" <<EOF
{
  "hook_event_name": "PreToolUse",
  "tool_name": "Bash",
  "cwd": "$SANDBOX_ROOT/repo",
  "tool_input": {
    "command": $(jq -Rn --arg c "$command" '$c')
  }
}
EOF
}

make_payload status         "git status --short && git log --oneline -3"
make_payload push           "git push origin main"
make_payload branch-delete  "git branch -D wip-lab"
make_payload gitdir-tamper  "cp src/data.py .git/hooks/pre-commit"
make_payload create-branch  "git checkout -b feature/agent-work"
make_payload foreign-write  "rm -f /tmp/demo-artifacts.txt && echo done"

echo "setup complete:"
ls payloads
