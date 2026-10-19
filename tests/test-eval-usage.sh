#!/usr/bin/env bash

# Eval usage parser tests
#
# Deterministic coverage for scripts/eval-usage.sh. No model and no network:
# all fixtures are local logs or tiny hook scripts.

set -uo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT" || exit 1

USAGE="scripts/eval-usage.sh"

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

assert_jq() {
    local file="$1" filter="$2" label="$3"
    if jq -e "$filter" "$file" >/dev/null 2>&1; then
        pass "$label"
    else
        fail "$label"
        sed 's/^/    /' "$file" | tail -5
    fi
}

TMP="$(mktemp -d "${TMPDIR:-/tmp}/eval-usage-test-XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

echo "🧪 Eval Usage Tests"
echo "==================="
echo

cat >"$TMP/opencode.log" <<'EOF'
not json
{"type":"step_start","part":{}}
{"type":"step_finish","part":{"tokens":{"total":11182,"input":11168,"output":3,"reasoning":11,"cache":{"write":0,"read":0}},"cost":0.0016822}}
{"type":"step_finish","part":{"tokens":{"total":100,"input":90,"output":10,"reasoning":0,"cache":{"write":5,"read":7}},"cost":0.0001}}
EOF
"$USAGE" --log "$TMP/opencode.log" --output "$TMP/opencode.json"
assert_jq "$TMP/opencode.json" '
    .known == true and .source == "opencode-json"
    and .input_tokens == 11258 and .output_tokens == 13
    and .reasoning_tokens == 11 and .cache_write_tokens == 5
    and .cache_read_tokens == 7 and .total_tokens == 11282
    and .cost_usd == 0.0017822 and .currency == "USD"
' "opencode JSONL sums step_finish tokens and cost"

cat >"$TMP/claude.log" <<'EOF'
{"type":"system","subtype":"init"}
{"type":"result","total_cost_usd":0.1773532,"usage":{"input_tokens":2,"cache_creation_input_tokens":21946,"cache_read_input_tokens":8486,"output_tokens":4,"output_tokens_details":{"thinking_tokens":3}}}
EOF
"$USAGE" --log "$TMP/claude.log" --output "$TMP/claude.json"
assert_jq "$TMP/claude.json" '
    .known == true and .source == "claude-json"
    and .input_tokens == 2 and .output_tokens == 4
    and .reasoning_tokens == 3 and .cache_write_tokens == 21946
    and .cache_read_tokens == 8486 and .total_tokens == 30438
    and .cost_usd == 0.1773532 and .currency == "USD"
' "Claude result JSON uses the last result and includes thinking tokens"

printf 'nothing useful\n' >"$TMP/none.log"
"$USAGE" --log "$TMP/none.log" --output "$TMP/none.json"
assert_jq "$TMP/none.json" '.known == false and .source == "none"' \
    "unknown output is known:false, never zero"

cat >"$TMP/partial.log" <<'EOF'
{"type":"step_finish","part":{"tokens":{"input":5}}}
EOF
"$USAGE" --log "$TMP/partial.log" --output "$TMP/partial.json"
assert_jq "$TMP/partial.json" '
    .known == true and .input_tokens == 5 and .total_tokens == 5
    and (has("cost_usd") | not)
' "partial token-only usage is accepted"

cat >"$TMP/token-only.log" <<'EOF'
{"type":"step_finish","part":{"tokens":{"input":5,"reasoning":3}}}
EOF
"$USAGE" --log "$TMP/token-only.log" --output "$TMP/token-only.json"
assert_jq "$TMP/token-only.json" '
    .known == true and .input_tokens == 5 and .reasoning_tokens == 3
    and .total_tokens == 8
' "opencode-style token-only usage includes reasoning in the total"

cat >"$TMP/invalid.log" <<'EOF'
{"type":"step_finish","part":{"tokens":{"input":-3,"output":"x","total":10},"cost":-1}}
EOF
"$USAGE" --log "$TMP/invalid.log" --output "$TMP/invalid.json"
assert_jq "$TMP/invalid.json" '
    .known == true and .total_tokens == 10
    and (has("input_tokens") | not) and (has("output_tokens") | not)
    and (has("cost_usd") | not)
' "invalid numbers are ignored without inventing zero"

cat >"$TMP/hook-file.sh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' '{"known":true,"source":"hook-file","total_tokens":42,"cost_usd":0.5}' >"$USAGE_FILE"
EOF
chmod +x "$TMP/hook-file.sh"
"$USAGE" --log "$TMP/opencode.log" --cmd "$TMP/hook-file.sh" --output "$TMP/hook-file.json"
assert_jq "$TMP/hook-file.json" '
    .known == true and .source == "hook-file" and .total_tokens == 42 and .cost_usd == 0.5
' "a hook that writes USAGE_FILE overrides the built-ins"

cat >"$TMP/hook-stdout.sh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' '{"known":true,"source":"hook-stdout","input_tokens":7}'
EOF
chmod +x "$TMP/hook-stdout.sh"
"$USAGE" --log "$TMP/opencode.log" --cmd "$TMP/hook-stdout.sh" --output "$TMP/hook-stdout.json"
assert_jq "$TMP/hook-stdout.json" '
    .known == true and .source == "hook-stdout" and .input_tokens == 7 and .total_tokens == 7
' "a hook that prints JSON overrides the built-ins"

cat >"$TMP/hook-fail.sh" <<'EOF'
#!/usr/bin/env bash
exit 3
EOF
chmod +x "$TMP/hook-fail.sh"
"$USAGE" --log "$TMP/opencode.log" --cmd "$TMP/hook-fail.sh" \
    --output "$TMP/hook-fail.json" 2>"$TMP/hook-fail.err"
assert_jq "$TMP/hook-fail.json" '.known == true and .source == "opencode-json"' \
    "a failing hook falls back to built-in extraction"
if grep -q "usage command failed" "$TMP/hook-fail.err"; then
    pass "a failing hook warns on stderr"
else
    fail "a failing hook did not warn"
fi

cat >"$TMP/hook-stderr.sh" <<'EOF'
#!/usr/bin/env bash
echo "hook-diagnostic-9f3d: cannot read vendor usage" >&2
exit 2
EOF
chmod +x "$TMP/hook-stderr.sh"
"$USAGE" --log "$TMP/opencode.log" --cmd "$TMP/hook-stderr.sh" \
    --output "$TMP/hook-stderr.json" 2>"$TMP/hook-stderr.err"
assert_jq "$TMP/hook-stderr.json" '.known == true and .source == "opencode-json"' \
    "a hook that exits 2 still falls back to built-ins"
if grep -q "hook-diagnostic-9f3d" "$TMP/hook-stderr.err"; then
    pass "the hook's stderr reaches the caller's log"
else
    fail "the hook's stderr was discarded"
fi

cat >"$TMP/hook-bad.sh" <<'EOF'
#!/usr/bin/env bash
printf 'not json\n'
EOF
chmod +x "$TMP/hook-bad.sh"
"$USAGE" --log "$TMP/claude.log" --cmd "$TMP/hook-bad.sh" \
    --output "$TMP/hook-bad.json" 2>"$TMP/hook-bad.err"
assert_jq "$TMP/hook-bad.json" '.known == true and .source == "claude-json"' \
    "an invalid hook falls back to built-in extraction"

"$USAGE" --log "$TMP/missing.log" --output "$TMP/missing.json"
assert_jq "$TMP/missing.json" '.known == false and .source == "none"' \
    "a missing log is unknown rather than a failure"

if "$USAGE" >/dev/null 2>&1; then
    fail "--log is not required"
else
    pass "--log is required"
fi

echo
echo "================================"
echo "Results: $PASSED passed, $FAILED failed"
[[ "$FAILED" -eq 0 ]] || exit 1
exit 0
