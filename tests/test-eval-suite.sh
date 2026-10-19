#!/usr/bin/env bash

# Eval harness tests
#
# Deterministic coverage for scripts/eval-run.sh, scripts/eval-suite.sh and
# scripts/eval-grade.sh. No model and no network: the agent is `true` and the
# judge is a stub that scores every pending criterion at its maximum, so the
# tests exercise scheduling, resume, sharding, aggregation and judge
# validation rather than eval quality.
#
# Deliberately no -e: report every failure in one run.

set -uo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT" || exit 1

RUN_ONE="scripts/eval-run.sh"
SUITE="scripts/eval-suite.sh"
GRADE="scripts/eval-grade.sh"

PASSED=0
FAILED=0

pass() {
    echo "  ✓ $1"
    PASSED=$((PASSED + 1))
}

fail() {
    echo "  ✗ $1"
    FAILED=$((FAILED + 1))
    if [[ -n "${2:-}" && -f "$2" ]]; then
        sed 's/^/    /' "$2" | tail -20
    fi
}

TMP="$(mktemp -d "${TMPDIR:-/tmp}/eval-suite-test-XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

echo "🧪 Eval Harness Tests"
echo "====================="
echo

# --- primitives ---------------------------------------------------------------

if "$RUN_ONE" --selftest >"$TMP/eval-run.log" 2>&1; then
    pass "eval-run selftest"
else
    fail "eval-run selftest" "$TMP/eval-run.log"
fi

if "$SUITE" --selftest >"$TMP/eval-suite.log" 2>&1; then
    pass "eval-suite selftest"
else
    fail "eval-suite selftest" "$TMP/eval-suite.log"
fi

# Every fixture setup must run through the runner. A setup that changes into
# its own directory before resolving $0 used to look for inputs/inputs when the
# runner invoked it relatively.
fixture_out="$TMP/fixture-setups"
fixture_failures=""
fixture_count=0
while IFS= read -r setup; do
    scenario_dir="${setup%/inputs/setup.sh}"
    scenario="${scenario_dir##*/}"
    plugin_dir="${scenario_dir%/evals/*}"
    plugin="${plugin_dir##*/}"
    fixture_count=$((fixture_count + 1))
    if ! "$RUN_ONE" --plugin "$plugin" --scenario "$scenario" \
        --agent-cmd true --arm with --no-auth --out "$fixture_out" \
        >"$TMP/fixture-$plugin-$scenario.log" 2>&1; then
        fixture_failures+=" $plugin/$scenario"$'\n'
    fi
done < <(find plugins -path '*/evals/*/inputs/setup.sh' | LC_ALL=C sort)
if [[ -z "$fixture_failures" ]]; then
    pass "every scenario fixture setup runs ($fixture_count fixtures)"
else
    fail "fixture setup failed for:$fixture_failures"
fi

# --- discovery ----------------------------------------------------------------

expected="$(
    find plugins -mindepth 3 -maxdepth 3 -type d -path 'plugins/*/evals/*' |
        sed -E 's#^plugins/([^/]+)/evals/([^/]+)$#\1/\2#' |
        LC_ALL=C sort
)"
actual="$("$SUITE" --list 2>"$TMP/list.err" | LC_ALL=C sort)"

if [[ "$expected" == "$actual" && -n "$expected" ]]; then
    pass "discovery lists every plugins/<plugin>/evals/<scenario> pair"
else
    fail "discovery does not match the eval directories" "$TMP/list.err"
fi

if grep -q '^\.worktrees/' <<<"$actual"; then
    fail "discovery leaked scenarios from .worktrees"
else
    pass "discovery ignores worktrees"
fi

bad_ref_rc=0
"$SUITE" --changed does-not-exist --list >"$TMP/bad-ref.out" 2>"$TMP/bad-ref.err" ||
    bad_ref_rc=$?
if [[ "$bad_ref_rc" -ne 0 ]] &&
    grep -q "cannot diff against 'does-not-exist'" "$TMP/bad-ref.err"; then
    pass "--changed names an unresolvable ref instead of silently selecting all"
else
    fail "--changed did not surface the bad ref (exit $bad_ref_rc)" "$TMP/bad-ref.err"
fi

count="$(printf '%s\n' "$expected" | wc -l | tr -d ' ')"
plan="$("$SUITE" --plan 2>&1)"
if grep -q "Scenarios: $count$" <<<"$plan" &&
    grep -q "Agent invocations: $((count * 2))$" <<<"$plan"; then
    pass "--plan counts two arms per scenario"
else
    fail "--plan counts are wrong (expected $count scenarios, $((count * 2)) invocations)"
fi
if ((count > 1)) && grep -q "Tip: --jobs N runs N scenarios concurrently" <<<"$plan"; then
    pass "--plan suggests --jobs when scenarios would run sequentially"
else
    fail "--plan did not suggest --jobs for $count sequential scenarios"
fi

# --- sharding -----------------------------------------------------------------

"$SUITE" --list --shard 1/3 >"$TMP/shard-1" 2>/dev/null
"$SUITE" --list --shard 2/3 >"$TMP/shard-2" 2>/dev/null
"$SUITE" --list --shard 3/3 >"$TMP/shard-3" 2>/dev/null
cat "$TMP/shard-1" "$TMP/shard-2" "$TMP/shard-3" | LC_ALL=C sort >"$TMP/shards-joined"
printf '%s\n' "$expected" >"$TMP/expected"

if diff -q "$TMP/expected" "$TMP/shards-joined" >/dev/null; then
    pass "shards 1/3 + 2/3 + 3/3 partition the selection"
else
    fail "shards overlap or lose scenarios"
fi

if [[ -z "$(LC_ALL=C sort "$TMP/shards-joined" | uniq -d)" ]]; then
    pass "no scenario appears in two shards"
else
    fail "a scenario appears in more than one shard"
fi

if [[ "$("$SUITE" --list --shard 4/3 2>&1)" == *"between 1 and 3"* ]]; then
    pass "an out-of-range shard is rejected"
else
    fail "an out-of-range shard was not rejected"
fi

# --- unit selection and selective installation -------------------------------

unit_skill_dir="$TMP/unit-skill"
"$RUN_ONE" --plugin git-workflow --scenario pr-draft-read-replica \
    --agent-cmd true --unit skills/pr --out "$unit_skill_dir" --no-auth \
    >"$TMP/unit-skill.log" 2>&1
unit_skill_rc=$?
unit_skill_tree="$(tar -tf "$unit_skill_dir/git-workflow/pr-draft-read-replica/with/run-1/snapshot.tar" 2>/dev/null)"
if [[ "$unit_skill_rc" -eq 0 ]] &&
    grep -q '\.claude/skills/pr/SKILL.md$' <<<"$unit_skill_tree" &&
    ! grep -q '\.claude/skills/changelog/' <<<"$unit_skill_tree"; then
    pass "a skills/<name> unit installs only that skill in the with-arm"
else
    fail "selective skill installation leaked other units" "$TMP/unit-skill.log"
fi

unit_command_dir="$TMP/unit-command"
"$RUN_ONE" --plugin code-quality --scenario review-empty-changeset \
    --agent-cmd true --unit commands/review --out "$unit_command_dir" --no-auth \
    >"$TMP/unit-command.log" 2>&1
unit_command_rc=$?
unit_command_tree="$(tar -tf "$unit_command_dir/code-quality/review-empty-changeset/with/run-1/snapshot.tar" 2>/dev/null)"
if [[ "$unit_command_rc" -eq 0 ]] &&
    grep -q '\.claude/commands/code-quality/review\.md$' <<<"$unit_command_tree" &&
    ! grep -q '\.claude/commands/code-quality/\(check\|clean\)\.md$' <<<"$unit_command_tree"; then
    pass "a commands/<name> unit installs only that command in the with-arm"
else
    fail "selective command installation leaked other units" "$TMP/unit-command.log"
fi

if "$RUN_ONE" --plugin git-workflow --scenario pr-draft-read-replica \
    --agent-cmd true --unit skills/nope --out "$TMP/unit-unknown" --no-auth \
    >"$TMP/unit-unknown.log" 2>&1; then
    fail "an unknown unit was accepted by eval-run"
else
    if grep -q "unknown unit 'skills/nope'" "$TMP/unit-unknown.log"; then
        pass "an unknown unit fails clearly in eval-run"
    else
        fail "the unknown-unit failure was not clear" "$TMP/unit-unknown.log"
    fi
fi

unit_list="$("$SUITE" --list --unit skills/pr 2>"$TMP/unit-list.err")"
if [[ "$unit_list" == "git-workflow/pr-draft-read-replica" ]]; then
    pass "suite --unit selects only scenarios declaring the skill"
else
    fail "suite --unit did not select the declared skill scenario" "$TMP/unit-list.err"
fi

command_list="$("$SUITE" --list --plugin code-quality --unit commands/review 2>/dev/null | LC_ALL=C sort)"
expected_command_list="$(printf '%s\n' \
    code-quality/review-all-flag \
    code-quality/review-changed-files-default \
    code-quality/review-empty-changeset | LC_ALL=C sort)"
if [[ "$command_list" == "$expected_command_list" ]]; then
    pass "suite --unit selects every scenario declaring the command"
else
    fail "suite --unit missed or added command scenarios"
fi

if "$SUITE" --list --plugin git-workflow --unit commands/review \
    >"$TMP/unit-mismatch.log" 2>&1; then
    fail "a unit unknown to the selected plugin was accepted"
else
    if grep -q "unknown --unit 'commands/review'" "$TMP/unit-mismatch.log"; then
        pass "a unit unknown to the selected plugin fails clearly"
    else
        fail "the plugin/unit mismatch failure was not clear" "$TMP/unit-mismatch.log"
    fi
fi

hook_list="$("$SUITE" --list --unit hooks/protect-main-branch 2>/dev/null | LC_ALL=C sort)"
if [[ "$(wc -l <<<"$hook_list" | tr -d ' ')" == "3" ]] &&
    ! grep -q 'scanner-' <<<"$hook_list"; then
    pass "hook units select their scenarios without an install"
else
    fail "hook unit selection is wrong"
fi

unit_suite="$TMP/unit-suite"
"$SUITE" --agent-cmd true --plugin git-workflow \
    --scenario pr-draft-read-replica --unit skills/pr --out "$unit_suite" \
    >"$TMP/unit-suite.log" 2>&1
unit_suite_rc=$?
if [[ "$unit_suite_rc" -eq 0 ]] &&
    [[ "$(jq -c '.config.units' "$unit_suite/manifest.json" 2>/dev/null)" == '["skills/pr"]' ]] &&
    [[ "$(jq -c '.scenarios[0].units' "$unit_suite/manifest.json" 2>/dev/null)" == '["skills/pr"]' ]] &&
    tar -tf "$unit_suite/runs/git-workflow/pr-draft-read-replica/with/run-1/snapshot.tar" 2>/dev/null |
    grep -q '\.claude/skills/pr/SKILL.md$'; then
    pass "a suite forwards its selected unit to the runner and manifest"
else
    fail "a suite did not record or forward its selected unit" "$TMP/unit-suite.log"
fi

"$SUITE" --agent-cmd true --plugin git-workflow \
    --scenario pr-draft-read-replica --unit skills/pr --out "$unit_suite" --resume \
    >"$TMP/unit-resume.log" 2>&1
if [[ "$(jq -r '.counts.executed' "$unit_suite/suite.json" 2>/dev/null)" == "0" ]] &&
    [[ "$(jq -r '.counts.skipped' "$unit_suite/suite.json" 2>/dev/null)" == "1" ]]; then
    pass "--resume accepts the same unit selection"
else
    fail "--resume reran a matching unit selection" "$TMP/unit-resume.log"
fi

resume_units_rc=0
"$SUITE" --agent-cmd true --plugin git-workflow \
    --scenario pr-draft-read-replica --out "$unit_suite" --resume \
    >"$TMP/unit-resume-mismatch.log" 2>&1 || resume_units_rc=$?
if [[ "$resume_units_rc" -ne 0 ]] &&
    grep -q "different agent, arm, runs, timeout, skill dir, auth mode, auth paths, units, usage commands or runner" \
        "$TMP/unit-resume-mismatch.log"; then
    pass "--resume rejects a changed unit selection"
else
    fail "--resume accepted a different unit selection" "$TMP/unit-resume-mismatch.log"
fi

two_unit_dir="$TMP/two-units"
"$SUITE" --agent-cmd true --plugin pr-review-triage \
    --scenario triage-command-argument-flow \
    --unit commands/triage --unit skills/triage-reviews \
    --out "$two_unit_dir" >"$TMP/two-units.log" 2>&1
two_unit_rc=$?
two_unit_tree="$(tar -tf "$two_unit_dir/runs/pr-review-triage/triage-command-argument-flow/with/run-1/snapshot.tar" 2>/dev/null)"
if [[ "$two_unit_rc" -eq 0 ]] &&
    grep -q '\.claude/commands/pr-review-triage/triage\.md$' <<<"$two_unit_tree" &&
    grep -q '\.claude/skills/triage-reviews/SKILL\.md$' <<<"$two_unit_tree"; then
    pass "two selected units reach eval-run as separate --unit arguments"
else
    fail "multiple units were not forwarded as separate arguments" "$TMP/two-units.log"
fi

uncreatable="$TMP/not-a-directory"
printf 'file\n' >"$uncreatable"
uncreatable_rc=0
"$SUITE" --agent-cmd true --plugin git-workflow \
    --scenario changelog-release-refused --out "$uncreatable/child" \
    >"$TMP/uncreatable.log" 2>&1 || uncreatable_rc=$?
if [[ "$uncreatable_rc" -ne 0 ]] &&
    grep -q "cannot create suite directories" "$TMP/uncreatable.log" &&
    ! grep -q '▶' "$TMP/uncreatable.log"; then
    pass "an uncreatable --out fails before any job starts"
else
    fail "an uncreatable --out did not fail before running jobs" "$TMP/uncreatable.log"
fi

# --- run, resume and aggregation ----------------------------------------------

suite_dir="$TMP/suite"
selection=(--plugin git-workflow --scenario 'pr-draft-read-replica|changelog-create-skeleton')

"$SUITE" --agent-cmd true --jobs 2 --out "$suite_dir" "${selection[@]}" \
    >"$TMP/run-1.log" 2>&1
run_rc=$?
if [[ "$run_rc" -eq 0 ]] &&
    [[ "$(jq -r '.counts.executed' "$suite_dir/suite.json" 2>/dev/null)" == "2" ]] &&
    [[ "$(jq -r '.counts.skipped' "$suite_dir/suite.json" 2>/dev/null)" == "0" ]]; then
    pass "two selected scenarios execute across parallel jobs"
else
    fail "selected scenarios did not execute as expected (exit $run_rc)" "$TMP/run-1.log"
fi

if [[ "$(wc -l <"$suite_dir/results.jsonl" | tr -d ' ')" == "2" ]] &&
    [[ -f "$suite_dir/summary.md" ]] && [[ -f "$suite_dir/grading/index.tsv" ]] &&
    [[ "$(wc -l <"$suite_dir/grading/index.tsv" | tr -d ' ')" -gt 1 ]]; then
    pass "aggregate reports and grading index are written"
else
    fail "aggregate reports are incomplete"
fi

"$SUITE" --agent-cmd true --jobs 2 --out "$suite_dir" --resume "${selection[@]}" \
    >"$TMP/run-2.log" 2>&1
if [[ "$(jq -r '.counts.executed' "$suite_dir/suite.json" 2>/dev/null)" == "0" ]] &&
    [[ "$(jq -r '.counts.skipped' "$suite_dir/suite.json" 2>/dev/null)" == "2" ]] &&
    [[ "$(wc -l <"$suite_dir/results.jsonl" | tr -d ' ')" == "2" ]]; then
    pass "--resume skips complete scenarios without losing results"
else
    fail "--resume reran or dropped completed scenarios" "$TMP/run-2.log"
fi

# A changed task invalidates the scenario even when the rubric is untouched.
# The rerun must recreate the result directory, not grade the new evidence
# with the judgement from the old task.
regrade_root="$TMP/regrade-repo"
regrade_scenario="$regrade_root/plugins/fixture/evals/changed-task"
mkdir -p "$regrade_scenario/inputs" "$regrade_root/plugins/fixture/skills/demo"
cp -R scripts "$regrade_root/scripts"
chmod +x "$regrade_root/scripts/"*.sh
printf '# Demo skill\n' >"$regrade_root/plugins/fixture/skills/demo/SKILL.md"
cat >"$regrade_scenario/criteria.json" <<'EOF'
{"context":"regrade fixture","type":"weighted_checklist","checklist":[{"name":"Task done","description":"The task is complete.","max_score":100}]}
EOF
cat >"$regrade_scenario/scenario.json" <<'EOF'
{"description":"regrade fixture","include":["./inputs"],"unit":"skills/demo"}
EOF
printf '# Original task\n\nDo the original thing.\n' >"$regrade_scenario/task.md"
cat >"$regrade_scenario/inputs/setup.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'fixture ready\n'
EOF
chmod +x "$regrade_scenario/inputs/setup.sh"

cat >"$TMP/judge-first.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
jq '[.judgement_pending[] | {name: .name, score: .max_score, max: .max_score, note: "first"}]' \
    grading-request.json > judgement.json
EOF
cat >"$TMP/judge-second.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
jq '[.judgement_pending[] | {name: .name, score: 0, max: .max_score, note: "second"}]' \
    grading-request.json > judgement.json
EOF
chmod +x "$TMP/judge-first.sh" "$TMP/judge-second.sh"

regrade_suite="$TMP/regrade-suite"
regrade_ok=1
"$regrade_root/scripts/eval-suite.sh" --agent-cmd true --judge-cmd "$TMP/judge-first.sh" \
    --plugin fixture --scenario changed-task --arm with --runs 1 --no-auth \
    --out "$regrade_suite" >"$TMP/regrade-first.log" 2>&1 || regrade_ok=0
regrade_run="$regrade_suite/runs/fixture/changed-task/with/run-1"
if [[ "$regrade_ok" -eq 1 ]] &&
    [[ "$(jq -r '.[0].note' "$regrade_run/judgement.json" 2>/dev/null)" == "first" ]]; then
    printf '{"stale":true}\n' >"$regrade_run/usage.json"
    printf '{"stale":true}\n' >"$regrade_run/judge-result.json"
    printf 'stale\n' >"$regrade_run/stale-artifact"
    printf '# Changed task\n\nDo the changed thing instead.\n' >"$regrade_scenario/task.md"

    "$regrade_root/scripts/eval-suite.sh" --agent-cmd true --judge-cmd "$TMP/judge-second.sh" \
        --plugin fixture --scenario changed-task --arm with --runs 1 --no-auth \
        --out "$regrade_suite" --resume >"$TMP/regrade-second.log" 2>&1
    regrade_rc=$?
    if [[ "$regrade_rc" -eq 0 ]] &&
        [[ "$(jq -r '.counts.executed' "$regrade_suite/suite.json" 2>/dev/null)" == "1" ]] &&
        [[ "$(jq -r '.counts.skipped' "$regrade_suite/suite.json" 2>/dev/null)" == "0" ]] &&
        [[ "$(jq -r '.[0].note' "$regrade_run/judgement.json" 2>/dev/null)" == "second" ]] &&
        [[ ! -e "$regrade_run/stale-artifact" ]] &&
        jq -e '.stale != true' "$regrade_run/usage.json" >/dev/null 2>&1 &&
        jq -e '.stale != true' "$regrade_run/judge-result.json" >/dev/null 2>&1; then
        pass "a changed task reruns and discards the previous grade, usage and timing"
    else
        fail "a changed task kept the previous judgement or derived artifacts" "$TMP/regrade-second.log"
    fi
else
    fail "the regrade fixture did not produce an initial judgement" "$TMP/regrade-first.log"
fi

# A non-zero agent is a recorded result, not a reason to abort the campaign.
agent_fail_dir="$TMP/agent-fail"
agent_fail_rc=0
"$SUITE" --agent-cmd false --plugin git-workflow \
    --scenario changelog-release-refused --out "$agent_fail_dir" \
    >"$TMP/agent-fail.log" 2>&1 || agent_fail_rc=$?
if [[ "$agent_fail_rc" -eq 0 ]] &&
    [[ "$(jq -r '.runs[] | select(.arm == "with") | .agent_exit' "$agent_fail_dir/results.jsonl" 2>/dev/null)" == "1" ]] &&
    [[ -f "$agent_fail_dir/summary.md" ]]; then
    pass "a failing agent is recorded without aborting the suite"
else
    fail "a failing agent aborted or disappeared from the suite" "$TMP/agent-fail.log"
fi

noauth_dir="$TMP/no-auth"
# shellcheck disable=SC2016  # $HOME is expanded inside the sandboxed agent
"$SUITE" --agent-cmd '[ -f "$HOME/.claude.json" ] && exit 1 || exit 0' --no-auth \
    --plugin git-workflow --scenario changelog-release-refused --out "$noauth_dir" \
    >"$TMP/no-auth.log" 2>&1
if [[ "$(jq -r '.runs[] | select(.arm == "with") | .agent_exit' "$noauth_dir/results.jsonl" 2>/dev/null)" == "0" ]]; then
    pass "--no-auth leaves the sandbox home empty"
else
    fail "--no-auth still exposed login state" "$TMP/no-auth.log"
fi

fakehome="$TMP/fakehome"
mkdir -p "$fakehome"
printf 'secret\n' >"$fakehome/.agent-auth"
custom_dir="$TMP/custom-auth"
# shellcheck disable=SC2016  # $HOME is expanded inside the sandboxed agent
HOME="$fakehome" "$SUITE" \
    --agent-cmd '[ -f "$HOME/.agent-auth" ] && touch custom-auth-visible' \
    --auth-path .agent-auth --plugin git-workflow --scenario changelog-release-refused \
    --out "$custom_dir" >"$TMP/custom-auth.log" 2>&1
if tar -tf "$custom_dir/runs/git-workflow/changelog-release-refused/with/run-1/snapshot.tar" 2>/dev/null |
    grep -q 'custom-auth-visible'; then
    pass "--auth-path copies a custom agent credential into the sandbox"
else
    fail "--auth-path did not reach the agent" "$TMP/custom-auth.log"
fi

if [[ "$(id -u)" -eq 0 ]]; then
    echo "  - skipped the unreadable-credential test (running as root)"
else
    locked_home="$TMP/locked-home"
    mkdir -p "$locked_home"
    printf 'secret\n' >"$locked_home/.claude.json"
    chmod 000 "$locked_home/.claude.json"
    cred_tmp="$TMP/cred-tmp"
    mkdir -p "$cred_tmp"
    locked_rc=0
    HOME="$locked_home" TMPDIR="$cred_tmp" "$RUN_ONE" --agent-cmd true \
        --plugin git-workflow --scenario changelog-release-refused \
        --arm with --out "$TMP/locked-auth" >"$TMP/locked-auth.log" 2>&1 || locked_rc=$?
    if [[ "$locked_rc" -ne 0 ]] &&
        grep -q "cannot copy agent credentials" "$TMP/locked-auth.log" &&
        grep -q -- "--no-auth" "$TMP/locked-auth.log"; then
        pass "an uncopyable credential source fails the run with a clear hint"
    else
        fail "an uncopyable credential source was ignored (exit $locked_rc)" "$TMP/locked-auth.log"
    fi
    if [[ -z "$(find "$cred_tmp" -mindepth 1 -print -quit)" ]]; then
        pass "a credential-copy failure still cleans the sandbox"
    else
        fail "a credential-copy failure left a sandbox behind"
    fi
    chmod 600 "$locked_home/.claude.json"
fi

# --- judge adapter and judged aggregation -------------------------------------

stub_judge="$TMP/judge.sh"
cat >"$stub_judge" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
jq '[.judgement_pending[] | {name: .name, score: .max_score, max: .max_score, note: "stub"}]' \
    grading-request.json > judgement.json
EOF
chmod +x "$stub_judge"

bad_judge="$TMP/bad-judge.sh"
cat >"$bad_judge" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '{}\n' > judgement.json
EOF
chmod +x "$bad_judge"

with_dir="$suite_dir/runs/git-workflow/pr-draft-read-replica/with/run-1"
if "$GRADE" --run-dir "$with_dir" --judge-cmd "$bad_judge" >"$TMP/bad-grade.log" 2>&1; then
    fail "a judge that writes an invalid file was accepted"
else
    pass "invalid judgement files are rejected"
fi

if "$GRADE" --suite "$suite_dir" --judge-cmd "$stub_judge" >"$TMP/good-grade.log" 2>&1; then
    pass "the judge adapter grades every pending run"
else
    fail "the judge adapter failed to grade the suite" "$TMP/good-grade.log"
fi

# shellcheck disable=SC2016  # the backticks are literal prompt text
if [[ -f "$with_dir/judge-prompt.md" ]] &&
    grep -q '`before.txt`' "$with_dir/judge-prompt.md" &&
    jq -e 'type == "array" and length > 0' "$with_dir/judgement.json" >/dev/null 2>&1; then
    pass "the judge receives a prompt and its judgement is valid JSON"
else
    fail "the judge prompt or judgement is missing"
fi

inline_dir="$TMP/inline-judge"
"$SUITE" --agent-cmd true --judge-cmd "$stub_judge" --plugin git-workflow \
    --scenario changelog-release-refused --out "$inline_dir" \
    >"$TMP/inline-judge.log" 2>&1
if [[ "$(jq -r '.counts.judged_runs' "$inline_dir/suite.json" 2>/dev/null)" == "2" ]] &&
    [[ "$(jq -r '.counts.pending_runs' "$inline_dir/suite.json" 2>/dev/null)" == "0" ]]; then
    pass "a suite judges each run it executes"
else
    fail "inline judging left runs pending" "$TMP/inline-judge.log"
fi

"$SUITE" --agent-cmd true --jobs 2 --out "$suite_dir" --resume "${selection[@]}" \
    >"$TMP/run-3.log" 2>&1
if [[ "$(jq -r '.counts.pending_runs' "$suite_dir/suite.json" 2>/dev/null)" == "0" ]] &&
    [[ "$(jq -r 'select(.scenario == "pr-draft-read-replica") | .arms.with.percent' \
        "$suite_dir/results.jsonl" 2>/dev/null)" == "48" ]]; then
    pass "aggregation folds valid judgements into scored percentages"
else
    fail "judgements were not folded into the aggregate" "$TMP/run-3.log"
fi

if "$SUITE" --agent-cmd true --jobs 2 --out "$suite_dir" --resume --strict "${selection[@]}" \
    >"$TMP/strict.log" 2>&1; then
    fail "--strict accepted a suite whose stub judgements produced no delta"
else
    pass "--strict fails when a scenario does not discriminate"
fi

partial_dir="$TMP/partial-rubric"
# The agent records a complete PR, so checks.sh scores 52 of the 100-point
# rubric. With no judge the remaining 48 points are pending: the denominator
# must stay 100 and the row must read 52%, never 100%.
# shellcheck disable=SC2016  # the JSON is written by the sandboxed agent
partial_agent='printf "%s" "{\"draft\":true,\"base\":\"main\",\"head\":\"feature/replica-routing\",\"title\":\"feat: add replica routing\"}" > pr-record.json'
"$SUITE" --agent-cmd "$partial_agent" --plugin git-workflow \
    --scenario pr-draft-read-replica --out "$partial_dir" \
    >"$TMP/partial-rubric.log" 2>&1
partial_rc=$?
partial_row="$(jq -c 'select(.scenario == "pr-draft-read-replica")' "$partial_dir/results.jsonl" 2>/dev/null)"
if [[ "$partial_rc" -eq 0 ]] &&
    [[ "$(jq -r '.runs[] | select(.arm == "with") | .max' <<<"$partial_row")" == "100" ]] &&
    jq -e 'all(.runs[] | select(.arm == "with"); .percent == 52)' <<<"$partial_row" >/dev/null &&
    jq -e '.arms.with.percent == 52' <<<"$partial_row" >/dev/null &&
    [[ "$(jq -r '.pending' <<<"$partial_row")" -gt 0 ]] &&
    [[ "$(jq -r '.counts.scenarios_with_mechanical_checks' "$partial_dir/suite.json")" == "1" ]] &&
    grep -Fq 'Mechanical coverage: 1 of 1 scenario(s) ship a checks.sh.' "$partial_dir/summary.md"; then
    pass "a partially graded rubric keeps the fixed 100-point denominator"
else
    fail "a partial rubric was renormalised in results.jsonl" "$TMP/partial-rubric.log"
fi

fractional_judge="$TMP/fractional-judge.sh"
cat >"$fractional_judge" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
jq '[.judgement_pending[] | {name: .name, score: (.max_score / 3), max: .max_score, note: "fractional"}]' \
    grading-request.json > judgement.json
EOF
chmod +x "$fractional_judge"

fractional_dir="$TMP/fractional-score"
fractional_rc=0
"$SUITE" --agent-cmd true --judge-cmd "$fractional_judge" \
    --plugin code-quality --scenario review-empty-changeset \
    --out "$fractional_dir" >"$TMP/fractional-score.log" 2>&1 || fractional_rc=$?
fractional_expected="$(jq -r '[.checklist[].max_score / 3] | add' \
    plugins/code-quality/evals/review-empty-changeset/criteria.json)"
fractional_rows="$(wc -l <"$fractional_dir/results.jsonl" 2>/dev/null | tr -d ' ')"
if [[ "$fractional_rc" -eq 0 ]] &&
    [[ "$fractional_rows" == "1" ]] &&
    jq -s -e --argjson expected "$fractional_expected" '
        length == 1
        and (.[0].runs | length) == 2
        and ([.[0].runs[].arm] | sort) == ["with", "without"]
        and all(.[]; all(.runs[]; .score == $expected and .pending == 0))
    ' "$fractional_dir/results.jsonl" >/dev/null &&
    [[ "$(jq -r '.counts.scenarios_with_mechanical_checks' "$fractional_dir/suite.json")" == "0" ]] &&
    grep -Fq 'Mechanical coverage: 0 of 1 scenario(s) ship a checks.sh.' "$fractional_dir/summary.md"; then
    pass "a fractional judgement keeps its exact score and writes a row"
else
    fail "a fractional judgement lost its row or score" "$TMP/fractional-score.log"
fi

narrow_dir="$TMP/resume-narrow"
"$SUITE" --agent-cmd true --plugin git-workflow \
    --scenario 'pr-draft-read-replica|changelog-create-skeleton' --out "$narrow_dir" \
    >"$TMP/resume-narrow-1.log" 2>&1
"$SUITE" --agent-cmd true --plugin git-workflow \
    --scenario pr-draft-read-replica --out "$narrow_dir" --resume \
    >"$TMP/resume-narrow-2.log" 2>&1
narrow_rc=$?
if [[ "$narrow_rc" -eq 0 ]] &&
    [[ "$(jq '.scenarios | length' "$narrow_dir/manifest.json" 2>/dev/null)" == "1" ]] &&
    [[ "$(jq -r '.scenarios[0].scenario' "$narrow_dir/manifest.json" 2>/dev/null)" == "pr-draft-read-replica" ]] &&
    [[ "$(wc -l <"$narrow_dir/results.jsonl" | tr -d ' ')" == "1" ]]; then
    pass "a narrowed --resume rewrites the manifest to the selected set"
else
    # results.jsonl holds one row per selected scenario.
    fail "a narrowed --resume left a stale manifest" "$TMP/resume-narrow-2.log"
fi

# --- usage and cost capture --------------------------------------------------

cost_agent='printf "%s\n" "{\"type\":\"step_finish\",\"part\":{\"tokens\":{\"total\":1000,\"input\":900,\"output\":100,\"reasoning\":5,\"cache\":{\"write\":1,\"read\":3}},\"cost\":0.02}}"'
cost_judge="$TMP/cost-judge.sh"
cat >"$cost_judge" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
jq '[.judgement_pending[] | {name: .name, score: .max_score, max: .max_score, note: "stub"}]' \
    grading-request.json > judgement.json
printf '%s\n' '{"type":"result","total_cost_usd":0.05,"usage":{"input_tokens":100,"output_tokens":20,"cache_creation_input_tokens":5,"cache_read_input_tokens":7}}'
EOF
chmod +x "$cost_judge"

usage_run_dir="$TMP/usage-run"
"$RUN_ONE" --plugin git-workflow --scenario changelog-release-refused \
    --agent-cmd "$cost_agent" --out "$usage_run_dir" --no-auth \
    >"$TMP/usage-run.log" 2>&1
usage_run_with="$usage_run_dir/git-workflow/changelog-release-refused/with/run-1"
if [[ -f "$usage_run_with/usage.json" ]] &&
    jq -e '.usage.known and .usage.cost_usd == 0.02 and .usage.total_tokens == 1000' \
        "$usage_run_with/result.json" >/dev/null &&
    jq -e '.known and .cost_usd == 0.02 and .total_tokens == 1000' \
        "$usage_run_with/usage.json" >/dev/null; then
    pass "eval-run records exact agent usage in result.json and usage.json"
else
    fail "eval-run did not record agent usage" "$TMP/usage-run.log"
fi

stderr_hook="$TMP/stderr-hook.sh"
cat >"$stderr_hook" <<'EOF'
#!/usr/bin/env bash
echo "usage-hook-diagnostic-7c1e" >&2
exit 2
EOF
chmod +x "$stderr_hook"
stderr_run_dir="$TMP/stderr-usage-run"
"$RUN_ONE" --plugin git-workflow --scenario changelog-release-refused \
    --agent-cmd true --usage-cmd "$stderr_hook" --out "$stderr_run_dir" --no-auth \
    >"$TMP/stderr-usage-run.log" 2>&1
if grep -q "usage-hook-diagnostic-7c1e" \
    "$stderr_run_dir/git-workflow/changelog-release-refused/with/run-1/usage.log"; then
    pass "a failing usage hook's stderr is preserved in the caller's usage.log"
else
    fail "the usage hook's stderr was discarded" "$TMP/stderr-usage-run.log"
fi

unknown_run_dir="$TMP/unknown-usage-run"
"$RUN_ONE" --plugin git-workflow --scenario changelog-release-refused \
    --agent-cmd true --out "$unknown_run_dir" --no-auth \
    >"$TMP/unknown-usage-run.log" 2>&1
if jq -e '.usage.known == false' \
    "$unknown_run_dir/git-workflow/changelog-release-refused/with/run-1/result.json" >/dev/null; then
    pass "eval-run records unknown agent usage as known:false"
else
    fail "eval-run turned unknown usage into a value" "$TMP/unknown-usage-run.log"
fi

cost_dir="$TMP/cost-suite"
"$SUITE" --agent-cmd "$cost_agent" --judge-cmd "$cost_judge" \
    --plugin git-workflow --scenario changelog-release-refused --out "$cost_dir" \
    >"$TMP/cost-suite.log" 2>&1
cost_rc=$?
cost_row="$(jq -c 'select(.scenario == "changelog-release-refused")' "$cost_dir/results.jsonl" 2>/dev/null)"
if [[ "$cost_rc" -eq 0 ]] &&
    [[ "$(jq -r '.counts.cost_known_runs' "$cost_dir/suite.json")" == "2" ]] &&
    [[ "$(jq -r '.counts.cost_total_runs' "$cost_dir/suite.json")" == "2" ]] &&
    jq -e 'all(.runs[]; .usage.known and .usage.cost_usd == 0.02 and
        .judge_usage.known and .judge_usage.cost_usd == 0.05)' <<<"$cost_row" >/dev/null &&
    jq -e '.arms.with.usage.combined.cost.total == 0.07 and
        .arms.without.usage.combined.cost.total == 0.07 and
        .arms.with.usage.combined.cost.runs_known == 1' <<<"$cost_row" >/dev/null &&
    jq -e '.counts.agent_seconds >= 0 and .counts.judge_seconds >= 0
        and .counts.timed_out_runs == 0' "$cost_dir/suite.json" >/dev/null &&
    [[ -f "$cost_dir/runs/git-workflow/changelog-release-refused/with/run-1/judge-result.json" ]] &&
    grep -Fq "\$0.07" "$cost_dir/summary.md"; then
    pass "suite sums known agent and judge cost without renormalising"
else
    fail "suite cost aggregation is wrong" "$TMP/cost-suite.log"
fi

cost_with_dir="$cost_dir/runs/git-workflow/changelog-release-refused/with/run-1"
if jq -e '.duration_seconds >= 0 and (.timed_out | type) == "boolean"' \
    "$cost_with_dir/judge-result.json" >/dev/null 2>&1; then
    pass "eval-grade records judge duration and timeout state"
else
    fail "eval-grade did not record judge timing"
fi
rm -f "$cost_with_dir/judge-usage.json"
"$GRADE" --run-dir "$cost_with_dir" --judge-cmd "$cost_judge" \
    >"$TMP/judge-usage-backfill.log" 2>&1
if [[ "$(jq -r '.known' "$cost_with_dir/judge-usage.json" 2>/dev/null)" == "true" ]]; then
    pass "eval-grade backfills judge-usage.json on the already-graded path"
else
    fail "judge usage was not backfilled" "$TMP/judge-usage-backfill.log"
fi

unknown_cost_dir="$TMP/unknown-cost"
"$SUITE" --agent-cmd true --plugin git-workflow \
    --scenario changelog-release-refused --out "$unknown_cost_dir" \
    >"$TMP/unknown-cost.log" 2>&1
if [[ "$(jq -r '.counts.cost_known_runs' "$unknown_cost_dir/suite.json")" == "0" ]] &&
    jq -e 'all(.runs[]; .usage.known == false and .judge_usage.known == false)
        and all(.arms[]; .usage.agent.cost.total == null and .usage.combined.cost.total == null)' \
        "$unknown_cost_dir/results.jsonl" >/dev/null &&
    grep -Fq 'Cost: not reported by the agent or judge command (0/2 runs)' "$unknown_cost_dir/summary.md"; then
    pass "unknown usage is not summed or rendered as zero"
else
    fail "unknown usage became a number" "$TMP/unknown-cost.log"
fi

partial_cost_dir="$TMP/partial-cost"
"$SUITE" --agent-cmd "$cost_agent" --judge-cmd true --plugin git-workflow \
    --scenario changelog-release-refused --out "$partial_cost_dir" \
    >"$TMP/partial-cost.log" 2>&1
partial_usage="$(jq -c 'select(.scenario == "changelog-release-refused")' "$partial_cost_dir/results.jsonl" 2>/dev/null)"
if jq -e '.arms.with.usage.agent.cost.total == 0.02
    and .arms.with.usage.judge.cost.total == null
    and .arms.with.usage.combined.cost.total == null
    and .arms.with.usage.combined.cost.runs_known == 0' <<<"$partial_usage" >/dev/null &&
    grep -Fq 'partial' "$partial_cost_dir/summary.md"; then
    pass "partial usage reports agent cost and leaves the combined total null"
else
    fail "partial usage was incorrectly combined" "$TMP/partial-cost.log"
fi

rm -f "$cost_dir/runs/git-workflow/changelog-release-refused/with/run-1/usage.json"
"$SUITE" --agent-cmd "$cost_agent" --judge-cmd "$cost_judge" \
    --plugin git-workflow --scenario changelog-release-refused \
    --out "$cost_dir" --resume >"$TMP/cost-resume.log" 2>&1
if [[ "$(jq -r '.known' "$cost_dir/runs/git-workflow/changelog-release-refused/with/run-1/usage.json" 2>/dev/null)" == "true" ]]; then
    pass "--resume backfills a missing usage.json from the existing log"
else
    fail "resume did not backfill usage" "$TMP/cost-resume.log"
fi

# --- interrupts and evidence --------------------------------------------------

slow_agent="$TMP/slow-agent.sh"
cat >"$slow_agent" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$$" >"$STUB_PID_FILE"
sleep 300
EOF
chmod +x "$slow_agent"

stub_pid_file="$TMP/stub.pid"
rm -f "$stub_pid_file"
signal_dir="$TMP/signal-suite"
STUB_PID_FILE="$stub_pid_file" "$SUITE" --agent-cmd "$slow_agent" --no-auth \
    --plugin git-workflow --scenario changelog-release-refused \
    --arm with --runs 1 --jobs 1 --out "$signal_dir" \
    >"$TMP/signal.log" 2>&1 &
signal_pid=$!
stub_pid=""
for _ in $(seq 1 150); do
    if [[ -s "$stub_pid_file" ]]; then
        stub_pid="$(cat "$stub_pid_file")"
        break
    fi
    kill -0 "$signal_pid" 2>/dev/null || break
    sleep 0.1
done

if [[ -z "$stub_pid" ]]; then
    kill -TERM "$signal_pid" 2>/dev/null
    for _ in $(seq 1 100); do
        kill -0 "$signal_pid" 2>/dev/null || break
        sleep 0.1
    done
    kill -KILL "$signal_pid" 2>/dev/null
    wait "$signal_pid" 2>/dev/null
    fail "the slow agent never started" "$TMP/signal.log"
else
    kill -TERM "$signal_pid" 2>/dev/null
    for _ in $(seq 1 150); do
        kill -0 "$signal_pid" 2>/dev/null || break
        sleep 0.1
    done
    if kill -0 "$signal_pid" 2>/dev/null; then
        kill -KILL "$signal_pid" 2>/dev/null
        wait "$signal_pid" 2>/dev/null
        fail "the suite ignored TERM and had to be killed" "$TMP/signal.log"
    else
        wait "$signal_pid" 2>/dev/null
        signal_rc=$?
        if [[ "$signal_rc" -eq 143 ]]; then
            pass "TERM stops the suite with status 143"
        else
            fail "TERM exited with $signal_rc instead of 143" "$TMP/signal.log"
        fi
    fi
    stub_alive=1
    for _ in $(seq 1 50); do
        if ! kill -0 "$stub_pid" 2>/dev/null; then
            stub_alive=0
            break
        fi
        sleep 0.1
    done
    if ((stub_alive)); then
        kill -KILL "$stub_pid" 2>/dev/null
        fail "TERM left the in-flight agent process $stub_pid running" "$TMP/signal.log"
    else
        pass "TERM kills the in-flight agent and its descendants"
    fi
fi

mkdir -p "$TMP/fail-tar-bin"
cat >"$TMP/fail-tar-bin/tar" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
chmod +x "$TMP/fail-tar-bin/tar"
mkdir -p "$TMP/tar-tmp"
tar_rc=0
PATH="$TMP/fail-tar-bin:$PATH" TMPDIR="$TMP/tar-tmp" "$RUN_ONE" --agent-cmd true \
    --plugin git-workflow --scenario changelog-release-refused --no-auth \
    --arm with --out "$TMP/tar-fail" >"$TMP/tar-fail.log" 2>&1 || tar_rc=$?
if [[ "$tar_rc" -ne 0 ]] && grep -q "cannot write the evidence bundle" "$TMP/tar-fail.log"; then
    pass "a failed evidence tar fails the run instead of grading without evidence"
else
    fail "a failed evidence tar was ignored (exit $tar_rc)" "$TMP/tar-fail.log"
fi

# --- stock jq portability -----------------------------------------------------

if [[ -x /usr/bin/jq ]]; then
    stock_dir="$TMP/stock-jq"
    stock_rc=0
    PATH=/usr/bin:/bin:/usr/sbin:/sbin "$SUITE" --agent-cmd true \
        --plugin git-workflow --scenario changelog-release-refused \
        --out "$stock_dir" >"$TMP/stock-jq.log" 2>&1 || stock_rc=$?
    if [[ "$stock_rc" -eq 0 ]] &&
        [[ "$(jq -r '.counts.executed' "$stock_dir/suite.json" 2>/dev/null)" == "1" ]] &&
        [[ "$(wc -l <"$stock_dir/results.jsonl" 2>/dev/null | tr -d ' ')" == "1" ]] &&
        jq -e '.scenario == "changelog-release-refused"' "$stock_dir/results.jsonl" >/dev/null; then
        pass "aggregation compiles and writes results under stock /usr/bin/jq"
    else
        fail "stock jq produced no aggregate row" "$TMP/stock-jq.log"
    fi
else
    echo "  - skipped stock-jq portability (/usr/bin/jq absent)"
fi

echo
echo "================================"
echo "Results: $PASSED passed, $FAILED failed"
[[ "$FAILED" -eq 0 ]] || exit 1
exit 0
