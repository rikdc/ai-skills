#!/usr/bin/env bash
set -euo pipefail

# Fixtures are created at the workspace root, where the runner invokes
# this script and where task.md expects them.

# Snapshot the plugin hook exactly as shipped; patterns file beside hooks/
# per the scanner's documented layout, log lands next to it.
rm -rf hooks payloads
mkdir -p payloads

if [[ -z "${EVAL_HOOK_SOURCE:-}" || ! -f "$EVAL_HOOK_SOURCE/hooks/mcp-security-scanner.sh" ]]; then
    echo "setup: EVAL_HOOK_SOURCE does not contain the plugin hook; run through bin/evalctl run" >&2
    exit 1
fi
cp -R "$EVAL_HOOK_SOURCE/hooks" ./hooks
if [[ ! -f "$EVAL_HOOK_SOURCE/security-patterns.conf" ]]; then
    echo "setup: EVAL_HOOK_SOURCE does not contain security-patterns.conf" >&2
    exit 1
fi
cp "$EVAL_HOOK_SOURCE/security-patterns.conf" ./security-patterns.conf
chmod +x hooks/*.sh

# Benign Notion page creation: repo-local changelog deltas, no credential
# shapes that any configured pattern would match.
cat > payloads/release-notes.json <<'EOF'
{
  "hook_event_name": "PreToolUse",
  "tool_name": "mcp__notion__create-page",
  "tool_input": {
    "parent_id": "team-notes",
    "title": "Release 2.3 notes",
    "content": "Rate limiter now handles bursty tenants; docs page for the webhook retries pushed after review."
  }
}
EOF

# Non-MCP tool call, same event name: deliberately outside the hook's scope.
cat > payloads/non-mcp.json <<'EOF'
{
  "hook_event_name": "PreToolUse",
  "tool_name": "Bash",
  "tool_input": {
    "command": "make test"
  }
}
EOF

echo "setup complete: hooks and two payloads staged"
