#!/usr/bin/env bash
set -euo pipefail

# Fixtures are created at the workspace root, where the runner invokes
# this script and where task.md expects them.

# Snapshot the plugin's hook script and its patterns file, laid out exactly
# as the plugin ships them: <workspace>/hooks/*.sh, config beside hooks/ so the
# scanner finds its CONFIG_FILE via its documented relative layout.
rm -rf hooks payloads
mkdir -p payloads

if [[ -z "${EVAL_HOOK_SOURCE:-}" || ! -f "$EVAL_HOOK_SOURCE/hooks/mcp-security-scanner.sh" ]]; then
    echo "setup: EVAL_HOOK_SOURCE does not contain the plugin hook; run through scripts/eval-run.sh" >&2
    exit 1
fi
cp -R "$EVAL_HOOK_SOURCE/hooks" ./hooks
if [[ ! -f "$EVAL_HOOK_SOURCE/security-patterns.conf" ]]; then
    echo "setup: EVAL_HOOK_SOURCE does not contain security-patterns.conf" >&2
    exit 1
fi
cp "$EVAL_HOOK_SOURCE/security-patterns.conf" ./security-patterns.conf
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
