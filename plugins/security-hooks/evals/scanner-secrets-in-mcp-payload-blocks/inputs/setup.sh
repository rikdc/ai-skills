#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

# Snapshot the plugin's hook script and its patterns file, laid out exactly
# as the plugin ships them: <sandbox>/hooks/*.sh, config beside hooks/ so the
# scanner finds its CONFIG_FILE via its documented relative layout.
rm -rf hooks payloads
mkdir -p payloads

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [[ -f "$SCRIPT_DIR/../../../hooks/mcp-security-scanner.sh" ]]; then
    cp -R "$SCRIPT_DIR/../../../hooks" ./hooks
    cp "$SCRIPT_DIR/../../../security-patterns.conf" ./security-patterns.conf
else
    cp -R "$SCRIPT_DIR/hook-src/hooks" ./hooks
    cp "$SCRIPT_DIR/hook-src/security-patterns.conf" ./security-patterns.conf
fi
chmod +x hooks/*.sh

# The payload a developer is about to send through an MCP tool: a bug report
# body that embeds real-looking credentials.
cat > payloads/bug-report.json <<'EOF'
{
  "hook_event_name": "PreToolUse",
  "tool_name": "mcp__github__add_issue_comment",
  "tool_input": {
    "issue_number": 571,
    "body": "Reproduces on CI. The runner still exports yesterday's api_key = CIKJv0nXdvBX9dSCY2C and the failure artifact shows AWS key AKIAIOSFODNN7EXAMPLE being read from the environment. Attaching traces in the morning."
  }
}
EOF

echo "setup complete."
ls -l hooks payloads
echo "--- fixtures: hooks come from the plugin, patterns beside them, payload staged"
