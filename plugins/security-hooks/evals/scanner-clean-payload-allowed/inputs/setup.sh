#!/usr/bin/env bash
set -euo pipefail

# Resolve the fixture directory before changing into it: the invocation path may be relative.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Snapshot the plugin hook exactly as shipped; patterns file beside hooks/
# per the scanner's documented layout, log lands next to it.
rm -rf hooks payloads
mkdir -p payloads

if [[ -f "$SCRIPT_DIR/../../../hooks/mcp-security-scanner.sh" ]]; then
    cp -R "$SCRIPT_DIR/../../../hooks" ./hooks
    cp "$SCRIPT_DIR/../../../security-patterns.conf" ./security-patterns.conf
else
    cp -R "$SCRIPT_DIR/hook-src/hooks" ./hooks
    cp "$SCRIPT_DIR/hook-src/security-patterns.conf" ./security-patterns.conf
fi
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
