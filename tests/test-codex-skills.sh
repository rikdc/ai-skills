#!/usr/bin/env bash

# Codex Skill Symlink Tests
#
# .codex/skills/* must mirror plugins/*/skills/* exactly, so Codex never
# sees a stale or missing skill. scripts/sync-codex-skills.sh --check is the
# single source of truth for what "in sync" means; this just wraps its
# result in the project's usual pass/fail test output.

set -uo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT" || exit 1

readonly SYNC_SCRIPT="scripts/sync-codex-skills.sh"

PASSED=0
FAILED=0

pass() {
    echo "  ✓ $1"
    PASSED=$((PASSED + 1))
}

fail() {
    echo "  ✗ $1"
    FAILED=$((FAILED + 1))
}

echo "🧪 Codex Skill Symlink Tests"
echo "============================"
echo

if [[ ! -x "$SYNC_SCRIPT" ]]; then
    echo "❌ $SYNC_SCRIPT is missing or not executable"
    exit 1
fi

if output="$("$SYNC_SCRIPT" --check 2>&1)"; then
    pass ".codex/skills is in sync with plugins/*/skills"
else
    fail ".codex/skills is out of sync with plugins/*/skills (run: $SYNC_SCRIPT)"
    echo "    ${output//$'\n'/$'\n'    }"
fi

echo
echo "================================"
echo "Results: $PASSED passed, $FAILED failed"

if [[ $FAILED -gt 0 ]]; then
    exit 1
fi
exit 0
