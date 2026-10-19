#!/usr/bin/env bash

# Eval report tests
#
# Deterministic coverage for scripts/eval-report.sh and the `make eval`
# selection wrapper. No model and no network: fixtures are synthetic suite
# directories, and the Make checks stop at --plan.
#
# Deliberately no -e: report every failure in one run.

set -uo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT" || exit 1

REPORT="scripts/eval-report.sh"

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

assert_eq() {
    local expected="$1" actual="$2" label="$3"
    if [[ "$expected" == "$actual" ]]; then
        pass "$label"
    else
        fail "$label (expected '$expected', got '$actual')"
    fi
}

assert_contains() {
    local file="$1" text="$2" label="$3"
    if grep -Fq -- "$text" "$file"; then
        pass "$label"
    else
        fail "$label (missing '$text')" "$file"
    fi
}

assert_not_contains() {
    local file="$1" text="$2" label="$3"
    if grep -Fq -- "$text" "$file"; then
        fail "$label (unexpected '$text')" "$file"
    else
        pass "$label"
    fi
}

assert_not_matches() {
    local file="$1" regex="$2" label="$3"
    if grep -Eq -- "$regex" "$file"; then
        fail "$label" "$file"
    else
        pass "$label"
    fi
}

TMP="$(mktemp -d "${TMPDIR:-/tmp}/eval-report-test-XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

echo "🧪 Eval Report Tests"
echo "===================="
echo

# --- fixture helpers ----------------------------------------------------------

new_suite() {
    local dir="$1" runs="$2" unit_json="${3:-[]}"
    mkdir -p "$dir/runs" "$dir/criteria" "$dir/report"
    jq -n --argjson runs "$runs" --argjson units "$unit_json" '
        {
            version: 1,
            created_at: "2026-01-02T03:04:05Z",
            config: {
                agent_cmd: "claude -p --model fixture-model",
                judge_cmd: "",
                arm: "both",
                runs: $runs,
                timeout: 900,
                skill_dir: ".claude/skills",
                auth: 0,
                auth_paths: [],
                units: $units,
                harness: "fixture",
                git_sha: "deadbee",
                git_dirty: false
            },
            scenarios: []
        }' >"$dir/manifest.json"
    jq -n '{config: {}, counts: {agent_invocations: 0}, scenarios: []}' >"$dir/suite.json"
    : >"$dir/results.jsonl"
}

add_scenario() {
    local dir="$1" plugin="$2" scenario="$3" criteria="$4" units_json="${5:-[]}"
    mkdir -p "$dir/criteria/$plugin" "$dir/runs/$plugin/$scenario"
    printf '%s\n' "$criteria" | jq '.' >"$dir/criteria/$plugin/$scenario.json"
    jq --arg plugin "$plugin" --arg scenario "$scenario" --argjson units "$units_json" \
        '.scenarios += [{plugin: $plugin, scenario: $scenario, units: $units, fingerprint: "fp"}]' \
        "$dir/manifest.json" >"$dir/manifest.json.tmp"
    mv "$dir/manifest.json.tmp" "$dir/manifest.json"
}

add_run() {
    local dir="$1" plugin="$2" scenario="$3" arm="$4" run="$5"
    local mechanical="$6" judged="${7:-[]}"
    local run_dir="$dir/runs/$plugin/$scenario/$arm/run-$run"
    local criteria="$dir/criteria/$plugin/$scenario.json"
    local covered pending
    mkdir -p "$run_dir"
    printf '%s\n' "$mechanical" | jq '.' >"$run_dir/mechanical.json"
    covered="$(printf '%s\n' "$mechanical" | jq -c '[.[].name]')"

    jq --argjson covered "$covered" '
        {judgement_pending: [.checklist[]
            | select(.name as $n | $covered | index($n) | not)
            | {name: .name, max_score: .max_score}]}
    ' "$criteria" >"$run_dir/grading-request.json"

    if [[ "$judged" != "[]" && -n "$judged" ]]; then
        printf '%s\n' "$judged" | jq '.' >"$run_dir/judgement.json"
    fi

    jq -n --arg arm "$arm" --argjson run "$run" --arg evidence "$run_dir" '
        {plugin: "fixture", scenario: "fixture", arm: $arm, run: $run,
         agent_exit: 0, timed_out: false, duration_seconds: 10,
         evidence: $evidence}' >"$run_dir/result.json"
    jq -n '{exit_code: 0, timed_out: false, duration_seconds: 4}' \
        >"$run_dir/judge-result.json"
}

add_usage() {
    local dir="$1" plugin="$2" scenario="$3" arm="$4" run="$5"
    local kind="$6" json="$7"
    local run_dir="$dir/runs/$plugin/$scenario/$arm/run-$run"
    printf '%s\n' "$json" | jq '.' >"$run_dir/$kind.json"
}

finish_suite() {
    local dir="$1" runs
    runs="$(jq -r '.config.runs' "$dir/manifest.json")"
    jq --argjson runs "$runs" '
        .counts.agent_invocations = ((.scenarios | length) * $runs * 2)' \
        "$dir/suite.json" >"$dir/suite.json.tmp"
    mv "$dir/suite.json.tmp" "$dir/suite.json"
}

run_report() {
    local dir="$1" out="$2"
    shift 2
    "$REPORT" "$dir" --no-color "$@" >"$out" 2>"$out.err"
}

# --- 1. fixed denominator and pending state ----------------------------------

pending="$TMP/pending"
new_suite "$pending" 1 '["skills/fixture"]'
add_scenario "$pending" demo pending \
    '{"context":"x","type":"weighted_checklist","checklist":[{"name":"Scored","description":"d","max_score":52},{"name":"Unjudged","description":"d","max_score":48}]}' \
    '["skills/fixture"]'
mech='[{"name":"Scored","score":52,"max":52}]'
add_run "$pending" demo pending with 1 "$mech"
add_run "$pending" demo pending without 1 "$mech"
finish_suite "$pending"

if run_report "$pending" "$TMP/pending.json" --format json; then
    pass "json report renders for a partially graded run"
else
    fail "json report failed on the pending fixture" "$TMP/pending.json.err"
fi

assert_eq "incomplete" "$(jq -r '.verdict.state' "$TMP/pending.json")" \
    "pending criteria force an incomplete verdict"
assert_eq "52" "$(jq -r '.totals.with' "$TMP/pending.json")" \
    "the fixed denominator keeps a 52-point run at 52, not 100"
assert_eq "52" "$(jq -r '.scenarios[0].runs[0].percent' "$TMP/pending.json")" \
    "a run score is reported against the fixed 100-point denominator"
assert_eq "48" "$(jq -r '.scenarios[0].runs[0].pending_points' "$TMP/pending.json")" \
    "unjudged criteria are reported as pending points"
assert_eq "48" "$(jq -r '.scenarios[0].criteria[] | select(.name == "Unjudged") | .max' "$TMP/pending.json")" \
    "the pending criterion keeps its rubric maximum"
run_report "$pending" "$TMP/pending.cli" --format cli
assert_contains "$TMP/pending.cli" "52.0/100" "CLI shows the run as 52/100"
assert_contains "$TMP/pending.cli" "Pending: 96 points (both arms)" \
    "CLI shows the 48 pending points per arm"
assert_not_contains "$TMP/pending.cli" "100.0/100" "CLI never renormalises 52/52 to 100%"
assert_contains "$TMP/pending.cli" "Cost: not reported by the agent command (0/2 runs)"     "all-unknown cost uses the concise hint"

# --- 2. per-criterion pairing, deltas and helps -------------------------------

helps="$TMP/helps"
new_suite "$helps" 2 '["skills/fixture"]'
add_scenario "$helps" demo helps \
    '{"context":"x","type":"weighted_checklist","checklist":[{"name":"A","description":"d","max_score":60},{"name":"B","description":"d","max_score":40}]}' \
    '["skills/fixture"]'
add_run "$helps" demo helps with 1 '[{"name":"A","score":50,"max":60},{"name":"B","score":40,"max":40}]'
add_run "$helps" demo helps with 2 '[{"name":"A","score":60,"max":60},{"name":"B","score":40,"max":40}]'
add_run "$helps" demo helps without 1 '[{"name":"A","score":40,"max":60},{"name":"B","score":40,"max":40}]'
add_run "$helps" demo helps without 2 '[{"name":"A","score":45,"max":60},{"name":"B","score":40,"max":40}]'
finish_suite "$helps"
run_report "$helps" "$TMP/helps.json" --format json

assert_eq "helps" "$(jq -r '.verdict.state' "$TMP/helps.json")" \
    "a positive delta with no regressions is a helps verdict"
assert_eq "12.5" "$(jq -r '.criteria[] | select(.name == "A") | .delta' "$TMP/helps.json")" \
    "criteria are paired by name and averaged across matched runs"
assert_eq "2" "$(jq -r '.criteria[] | select(.name == "A") | .paired_runs' "$TMP/helps.json")" \
    "a criterion reports how many run pairs contributed"
assert_eq "1" "$(jq -r '.totals.won' "$TMP/helps.json")" \
    "won criteria are counted"
assert_eq "0" "$(jq -r '.totals.lost' "$TMP/helps.json")" \
    "lost criteria are counted"
assert_eq "1" "$(jq -r '.totals.tied' "$TMP/helps.json")" \
    "tied criteria are counted"

# --- 3. regression detection and hurts ---------------------------------------

hurts="$TMP/hurts"
new_suite "$hurts" 2 '["skills/fixture"]'
add_scenario "$hurts" demo hurts \
    '{"context":"x","type":"weighted_checklist","checklist":[{"name":"A","description":"d","max_score":60},{"name":"B","description":"d","max_score":40}]}' \
    '["skills/fixture"]'
add_run "$hurts" demo hurts with 1 '[{"name":"A","score":30,"max":60},{"name":"B","score":40,"max":40}]'
add_run "$hurts" demo hurts with 2 '[{"name":"A","score":40,"max":60},{"name":"B","score":40,"max":40}]'
add_run "$hurts" demo hurts without 1 '[{"name":"A","score":50,"max":60},{"name":"B","score":40,"max":40}]'
add_run "$hurts" demo hurts without 2 '[{"name":"A","score":55,"max":60},{"name":"B","score":40,"max":40}]'
finish_suite "$hurts"
run_report "$hurts" "$TMP/hurts.json" --format json
run_report "$hurts" "$TMP/hurts.cli" --format cli

assert_eq "hurts" "$(jq -r '.verdict.state' "$TMP/hurts.json")" \
    "a negative delta is a hurts verdict"
assert_eq "1" "$(jq -r '.totals.regressions' "$TMP/hurts.json")" \
    "a criterion where without beats with is a regression"
assert_eq "A" "$(jq -r '.scenarios[0].regressions[0].name' "$TMP/hurts.json")" \
    "the regression names the criterion that regressed"
assert_contains "$TMP/hurts.cli" "Regressions" "CLI has a regressions section"
sed -n '/^Regressions$/,/^Scenarios/p' "$TMP/hurts.cli" >"$TMP/hurts.regressions"
assert_contains "$TMP/hurts.regressions" "demo/hurts" \
    "CLI regressions section names the scenario"
assert_contains "$TMP/hurts.regressions" "35.0 -> 52.5 (-17.5)" \
    "CLI regression row formats the criterion's with/without/delta"
assert_eq "1" "$(grep -c -- ' -> ' "$TMP/hurts.regressions")" \
    "CLI prints exactly one row for a one-regression fixture"

# --- 4. verdict states: indicative, inconclusive, no measurable effect -------

indicative="$TMP/indicative"
new_suite "$indicative" 1 '["skills/fixture"]'
add_scenario "$indicative" demo indicative \
    '{"context":"x","type":"weighted_checklist","checklist":[{"name":"A","description":"d","max_score":60},{"name":"B","description":"d","max_score":40}]}'
add_run "$indicative" demo indicative with 1 '[{"name":"A","score":60,"max":60},{"name":"B","score":40,"max":40}]'
add_run "$indicative" demo indicative without 1 '[{"name":"A","score":50,"max":60},{"name":"B","score":40,"max":40}]'
finish_suite "$indicative"
run_report "$indicative" "$TMP/indicative.json" --format json
assert_eq "indicative" "$(jq -r '.verdict.state' "$TMP/indicative.json")" \
    "one run per arm is labelled indicative"

inconclusive="$TMP/inconclusive"
new_suite "$inconclusive" 2 '["skills/fixture"]'
add_scenario "$inconclusive" demo inconclusive \
    '{"context":"x","type":"weighted_checklist","checklist":[{"name":"A","description":"d","max_score":60},{"name":"B","description":"d","max_score":40}]}'
add_run "$inconclusive" demo inconclusive with 1 '[{"name":"A","score":50,"max":60},{"name":"B","score":40,"max":40}]'
add_run "$inconclusive" demo inconclusive with 2 '[{"name":"A","score":60,"max":60},{"name":"B","score":40,"max":40}]'
add_run "$inconclusive" demo inconclusive without 1 '[{"name":"A","score":50,"max":60},{"name":"B","score":40,"max":40}]'
add_run "$inconclusive" demo inconclusive without 2 '[{"name":"A","score":55,"max":60},{"name":"B","score":40,"max":40}]'
finish_suite "$inconclusive"
run_report "$inconclusive" "$TMP/inconclusive.json" --format json
assert_eq "inconclusive" "$(jq -r '.verdict.state' "$TMP/inconclusive.json")" \
    "a delta inside overlapping per-arm ranges is inconclusive"

noeffect="$TMP/no-effect"
new_suite "$noeffect" 2 '["skills/fixture"]'
add_scenario "$noeffect" demo no-effect \
    '{"context":"x","type":"weighted_checklist","checklist":[{"name":"A","description":"d","max_score":60},{"name":"B","description":"d","max_score":40}]}'
add_run "$noeffect" demo no-effect with 1 '[{"name":"A","score":50,"max":60},{"name":"B","score":40,"max":40}]'
add_run "$noeffect" demo no-effect with 2 '[{"name":"A","score":60,"max":60},{"name":"B","score":40,"max":40}]'
add_run "$noeffect" demo no-effect without 1 '[{"name":"A","score":50,"max":60},{"name":"B","score":40,"max":40}]'
add_run "$noeffect" demo no-effect without 2 '[{"name":"A","score":60,"max":60},{"name":"B","score":40,"max":40}]'
finish_suite "$noeffect"
run_report "$noeffect" "$TMP/no-effect.json" --format json
assert_eq "no measurable effect" "$(jq -r '.verdict.state' "$TMP/no-effect.json")" \
    "a zero delta is reported as no measurable effect"

# --- 4b. declared runs with missing directories stay incomplete --------------

missing="$TMP/missing-run"
new_suite "$missing" 2 '["skills/fixture"]'
add_scenario "$missing" demo missing \
    '{"context":"x","type":"weighted_checklist","checklist":[{"name":"A","description":"d","max_score":100}]}' \
    '["skills/fixture"]'
add_run "$missing" demo missing with 1 '[{"name":"A","score":100,"max":100}]'
add_run "$missing" demo missing without 1 '[{"name":"A","score":50,"max":100}]'
finish_suite "$missing"
printf '%s\n' '{"plugin":"demo","scenario":"missing","status":"runner-failed"}' >>"$missing/results.jsonl"
jq '.counts.runner_failures = 1' "$missing/suite.json" >"$missing/suite.json.tmp"
mv "$missing/suite.json.tmp" "$missing/suite.json"
run_report "$missing" "$TMP/missing.json" --format json

assert_eq "incomplete" "$(jq -r '.verdict.state' "$TMP/missing.json")" \
    "a declared run with no directory is incomplete, not helps"
assert_eq "2" "$(jq -r '[.scenarios[0].runs[] | select(.status == "missing")] | length' "$TMP/missing.json")" \
    "both missing arms are materialised as missing runs"
assert_eq "missing" "$(jq -r '[.scenarios[0].runs[] | select(.run == 2) | .status] | unique | join(",")' "$TMP/missing.json")" \
    "the absent run-2 is reported explicitly for both arms"
assert_eq "1" "$(jq -r '.totals.results_failures' "$TMP/missing.json")" \
    "results.jsonl runner failures reach the verdict"
assert_eq "2" "$(jq -r '.totals.failed_runs' "$TMP/missing.json")" \
    "missing run directories count as failed runs"

suite_failure="$TMP/suite-failure"
new_suite "$suite_failure" 1 '["skills/fixture"]'
add_scenario "$suite_failure" demo suite-failure \
    '{"context":"x","type":"weighted_checklist","checklist":[{"name":"A","description":"d","max_score":100}]}' \
    '["skills/fixture"]'
add_run "$suite_failure" demo suite-failure with 1 '[{"name":"A","score":100,"max":100}]'
add_run "$suite_failure" demo suite-failure without 1 '[{"name":"A","score":50,"max":100}]'
finish_suite "$suite_failure"
jq '.counts.runner_failures = 1' "$suite_failure/suite.json" >"$suite_failure/suite.json.tmp"
mv "$suite_failure/suite.json.tmp" "$suite_failure/suite.json"
run_report "$suite_failure" "$TMP/suite-failure.json" --format json
assert_eq "incomplete" "$(jq -r '.verdict.state' "$TMP/suite-failure.json")" \
    "suite.json runner_failures force an incomplete verdict"

one_arm="$TMP/one-arm"
new_suite "$one_arm" 1 '["skills/fixture"]'
add_scenario "$one_arm" demo one-arm \
    '{"context":"x","type":"weighted_checklist","checklist":[{"name":"A","description":"d","max_score":100}]}'
add_run "$one_arm" demo one-arm with 1 '[{"name":"A","score":100,"max":100}]'
finish_suite "$one_arm"
jq '.config.arm = "with"' "$one_arm/manifest.json" >"$one_arm/manifest.json.tmp"
mv "$one_arm/manifest.json.tmp" "$one_arm/manifest.json"
run_report "$one_arm" "$TMP/one-arm.json" --format json
assert_contains "$TMP/one-arm.json" "paired with/without" \
    "a single-arm suite is diagnosed as having no shared arms"

unwritable="$TMP/not-a-directory"
printf 'file\n' >"$unwritable"
unwritable_rc=0
"$REPORT" "$helps" --format json --out "$unwritable/child" \
    >"$TMP/unwritable.log" 2>&1 || unwritable_rc=$?
if [[ "$unwritable_rc" -ne 0 ]] &&
    grep -Fq "cannot create report directory" "$TMP/unwritable.log"; then
    pass "an unwritable --out fails instead of printing success"
else
    fail "an unwritable --out was accepted" "$TMP/unwritable.log"
fi

# --- 4c. cost and usage ------------------------------------------------------

cost="$TMP/cost"
new_suite "$cost" 2 '["skills/fixture"]'
add_scenario "$cost" demo cost \
    '{"context":"x","type":"weighted_checklist","checklist":[{"name":"A","description":"d","max_score":100}]}' \
    '["skills/fixture"]'
for run in 1 2; do
    add_run "$cost" demo cost with "$run" '[{"name":"A","score":100,"max":100}]'
    add_run "$cost" demo cost without "$run" '[{"name":"A","score":99,"max":100}]'
    add_usage "$cost" demo cost with "$run" usage \
        '{"known":true,"source":"opencode-json","total_tokens":1000,"input_tokens":900,"output_tokens":100,"reasoning_tokens":0,"cache_read_tokens":0,"cost_usd":0.015,"currency":"USD"}'
    add_usage "$cost" demo cost without "$run" usage \
        '{"known":true,"source":"opencode-json","total_tokens":900,"input_tokens":800,"output_tokens":100,"reasoning_tokens":0,"cache_read_tokens":0,"cost_usd":0.009,"currency":"USD"}'
    add_usage "$cost" demo cost with "$run" judge-usage \
        '{"known":true,"source":"claude-json","total_tokens":100,"input_tokens":80,"output_tokens":20,"reasoning_tokens":0,"cache_read_tokens":0,"cost_usd":0.003,"currency":"USD"}'
    add_usage "$cost" demo cost without "$run" judge-usage \
        '{"known":true,"source":"claude-json","total_tokens":100,"input_tokens":80,"output_tokens":20,"reasoning_tokens":0,"cache_read_tokens":0,"cost_usd":0.001,"currency":"USD"}'
done
finish_suite "$cost"
run_report "$cost" "$TMP/cost.json" --format json
run_report "$cost" "$TMP/cost.cli" --format cli
run_report "$cost" "$TMP/cost.md" --format md

if jq -e '
    ((.cost.with.combined.cost.total - 0.036) | if . < 0 then -. else . end) < 0.0000001
    and ((.cost.without.combined.cost.total - 0.020) | if . < 0 then -. else . end) < 0.0000001
    and .cost.with.combined.cost.runs_known == 2
    and .cost.with.runs_total == 2
    and ((.usage.with.agent.cost.total - 0.03) | if . < 0 then -. else . end) < 0.0000001
    and ((.cost.cost_per_delta_point - 0.008) | if . < 0 then -. else . end) < 0.0000001
' "$TMP/cost.json" >/dev/null; then
    pass "JSON carries per-arm cost totals, coverage and cost per delta point"
else
    fail "JSON cost aggregation is wrong"
fi
if jq -e '
    .cost.with.combined.tokens.input.total == 1960
    and .cost.with.combined.tokens.output.total == 240
    and .cost.with.combined.tokens.total == 2200
    and .timing.agent_seconds == 40
    and .timing.judge_seconds == 16
    and .timing.runs == 4
' "$TMP/cost.json" >/dev/null; then
    pass "JSON splits input/output tokens and records agent/judge timing"
else
    fail "JSON token split or timing aggregation is wrong"
fi
assert_contains "$TMP/cost.cli" "input tokens" "CLI shows input tokens separately"
assert_contains "$TMP/cost.cli" "output tokens" "CLI shows output tokens separately"
assert_contains "$TMP/cost.cli" "Timing" "CLI renders a timing section"
assert_contains "$TMP/cost.cli" "agent 40s" "CLI reports agent seconds"
assert_contains "$TMP/cost.md" "input tokens" "Markdown shows input tokens separately"
assert_contains "$TMP/cost.md" "output tokens" "Markdown shows output tokens separately"
assert_contains "$TMP/cost.md" "## Timing" "Markdown renders a timing section"
assert_contains "$TMP/cost.cli" "COST" "CLI renders a cost table"
assert_contains "$TMP/cost.cli" "\$0.036" "CLI shows the with cost total"
assert_contains "$TMP/cost.cli" "+\$0.016" \
    "CLI DELTA is the difference of the displayed totals"
assert_contains "$TMP/cost.cli" "coverage: agent 4/4, judge 4/4, total 4/4" \
    "CLI coverage shows agent, judge and combined coverage"
assert_contains "$TMP/cost.cli" "Efficiency: +1.0 pts for +\$0.008 per run" \
    "CLI computes efficiency only for a supported positive delta"
assert_contains "$TMP/cost.md" "## Cost" "Markdown renders a cost section"
assert_contains "$TMP/cost.md" "\$0.036" "Markdown shows cost totals"
assert_contains "$TMP/cost.md" "+\$0.016" \
    "Markdown DELTA is the difference of the displayed totals"
assert_not_contains "$TMP/cost.md" "/null" "Markdown coverage never prints null"
if run_report "$cost" "$TMP/cost.all" --format all >/dev/null 2>&1 &&
    [[ -f "$cost/report/report.html" ]]; then
    pass "HTML report exists"
    assert_contains "$cost/report/report.html" "Cost with" "HTML has cost stat cards"
    assert_contains "$cost/report/report.html" "\$0.036" "HTML cost table shows totals"
    assert_contains "$cost/report/report.html" "+\$0.016" \
        "HTML DELTA is the difference of the displayed totals"
    assert_contains "$cost/report/report.html" "Cost delta</div><div class=\"v pos\">+\$0.016" \
        "HTML cost delta card equals with minus without"
    assert_contains "$cost/report/report.html" "input tokens" "HTML shows input tokens separately"
    assert_contains "$cost/report/report.html" "output tokens" "HTML shows output tokens separately"
    assert_contains "$cost/report/report.html" "Timing" "HTML renders a timing section"
    assert_not_contains "$cost/report/report.html" "/null" "HTML coverage never prints null"
    assert_contains "$cost/report/report.html" "2/2" "HTML coverage uses known/total"
else
    fail "HTML cost rendering failed"
fi

base="$TMP/baseline-suite"
cur="$TMP/current-suite"
for dir in "$base" "$cur"; do
    new_suite "$dir" 1 '["skills/fixture"]'
    add_scenario "$dir" demo compare \
        '{"context":"x","type":"weighted_checklist","checklist":[{"name":"A","description":"d","max_score":100}]}' \
        '["skills/fixture"]'
    add_run "$dir" demo compare with 1 '[{"name":"A","score":100,"max":100}]'
done
add_run "$base" demo compare without 1 '[{"name":"A","score":90,"max":100}]'
add_run "$cur" demo compare without 1 '[{"name":"A","score":95,"max":100}]'
finish_suite "$base"
finish_suite "$cur"
add_usage "$base" demo compare with 1 usage \
    '{"known":true,"source":"opencode-json","total_tokens":1100,"input_tokens":1000,"output_tokens":100,"cost_usd":0.010,"currency":"USD"}'
add_usage "$base" demo compare without 1 usage \
    '{"known":true,"source":"opencode-json","total_tokens":1000,"input_tokens":900,"output_tokens":100,"cost_usd":0.008,"currency":"USD"}'
add_usage "$base" demo compare with 1 judge-usage \
    '{"known":true,"source":"claude-json","total_tokens":220,"input_tokens":200,"output_tokens":20,"cost_usd":0.002,"currency":"USD"}'
add_usage "$base" demo compare without 1 judge-usage \
    '{"known":true,"source":"claude-json","total_tokens":220,"input_tokens":200,"output_tokens":20,"cost_usd":0.002,"currency":"USD"}'
add_usage "$cur" demo compare with 1 usage \
    '{"known":true,"source":"opencode-json","total_tokens":1300,"input_tokens":1200,"output_tokens":100,"cost_usd":0.014,"currency":"USD"}'
add_usage "$cur" demo compare without 1 usage \
    '{"known":true,"source":"opencode-json","total_tokens":1100,"input_tokens":1000,"output_tokens":100,"cost_usd":0.010,"currency":"USD"}'
add_usage "$cur" demo compare with 1 judge-usage \
    '{"known":true,"source":"claude-json","total_tokens":220,"input_tokens":200,"output_tokens":20,"cost_usd":0.002,"currency":"USD"}'
add_usage "$cur" demo compare without 1 judge-usage \
    '{"known":true,"source":"claude-json","total_tokens":220,"input_tokens":200,"output_tokens":20,"cost_usd":0.002,"currency":"USD"}'

run_report "$cur" "$TMP/compare.json" --format json --baseline "$base"
run_report "$cur" "$TMP/compare.cli" --format cli --baseline "$base"
run_report "$cur" "$TMP/compare.md" --format md --baseline "$base"
run_report "$cur" "$TMP/compare.all" --format all --baseline "$base" >/dev/null 2>&1
if jq -e '
    .comparison.comparable_scenarios == 1
    and ((.comparison.delta.with_cost_per_run - 0.004) | if . < 0 then -. else . end) < 0.0000001
    and ((.comparison.delta.without_cost_per_run - 0.002) | if . < 0 then -. else . end) < 0.0000001
    and .comparison.delta.effect == -5
    and .comparison.delta.with_input_tokens_per_run == 200
' "$TMP/compare.json" >/dev/null; then
    pass "JSON compares per-run cost, effect and input tokens with a baseline"
else
    fail "baseline comparison JSON is wrong"
fi
assert_contains "$TMP/compare.cli" "Change since baseline" "CLI renders a baseline section"
assert_contains "$TMP/compare.cli" "Cost with / run" "CLI shows per-run cost change"
assert_contains "$TMP/compare.cli" "+\$0.004" "CLI shows the per-run cost delta"
assert_contains "$TMP/compare.cli" "Input with / run" "CLI shows per-run input token change"
assert_contains "$TMP/compare.md" "Change since baseline" "Markdown renders a baseline section"
assert_contains "$cur/report/report.html" "Change since baseline" "HTML renders a baseline section"
assert_contains "$cur/report/report.html" "+\$0.004" "HTML shows the per-run cost delta"

tiny="$TMP/tiny-cost"
new_suite "$tiny" 1 '["skills/fixture"]'
add_scenario "$tiny" demo tiny-cost \
    '{"context":"x","type":"weighted_checklist","checklist":[{"name":"A","description":"d","max_score":100}]}' \
    '["skills/fixture"]'
add_run "$tiny" demo tiny-cost with 1 '[{"name":"A","score":100,"max":100}]'
add_run "$tiny" demo tiny-cost without 1 '[{"name":"A","score":78,"max":100}]'
add_usage "$tiny" demo tiny-cost with 1 usage \
    '{"known":true,"source":"opencode-json","total_tokens":1000,"cost_usd":0.006,"currency":"USD"}'
add_usage "$tiny" demo tiny-cost without 1 usage \
    '{"known":true,"source":"opencode-json","total_tokens":900,"cost_usd":0.001,"currency":"USD"}'
add_usage "$tiny" demo tiny-cost with 1 judge-usage \
    '{"known":true,"source":"claude-json","total_tokens":100,"cost_usd":0.002,"currency":"USD"}'
add_usage "$tiny" demo tiny-cost without 1 judge-usage \
    '{"known":true,"source":"claude-json","total_tokens":100,"cost_usd":0.001,"currency":"USD"}'
finish_suite "$tiny"
run_report "$tiny" "$TMP/tiny-cost.json" --format json
run_report "$tiny" "$TMP/tiny-cost.cli" --format cli
run_report "$tiny" "$TMP/tiny-cost.md" --format md
run_report "$tiny" "$TMP/tiny-cost.all" --format all >/dev/null 2>&1

if jq -e '.cost.cost_per_delta_point > 0 and .cost.cost_per_delta_point < 0.001' \
    "$TMP/tiny-cost.json" >/dev/null; then
    pass "small positive cost per point is retained in JSON"
else
    fail "small positive cost per point is missing"
fi
assert_contains "$TMP/tiny-cost.cli" "\$0.00027 per point" \
    "CLI shows enough precision for a sub-cent cost per point"
assert_not_contains "$TMP/tiny-cost.cli" "\$0.000 per point" \
    "CLI never renders a positive cost per point as \$0.000"
assert_contains "$TMP/tiny-cost.md" "\$0.00027 per point" \
    "Markdown shows enough precision for a sub-cent cost per point"
assert_contains "$tiny/report/report.html" "\$0.00027 per point" \
    "HTML shows enough precision for a sub-cent cost per point"

subcent="$TMP/subcent-cost"
new_suite "$subcent" 1 '["skills/fixture"]'
add_scenario "$subcent" demo subcent \
    '{"context":"x","type":"weighted_checklist","checklist":[{"name":"A","description":"d","max_score":100}]}' \
    '["skills/fixture"]'
add_run "$subcent" demo subcent with 1 '[{"name":"A","score":100,"max":100}]'
add_run "$subcent" demo subcent without 1 '[{"name":"A","score":99,"max":100}]'
add_usage "$subcent" demo subcent with 1 usage \
    '{"known":true,"source":"opencode-json","total_tokens":100,"cost_usd":0.0008,"currency":"USD"}'
add_usage "$subcent" demo subcent without 1 usage \
    '{"known":true,"source":"opencode-json","total_tokens":90,"cost_usd":0.0004,"currency":"USD"}'
add_usage "$subcent" demo subcent with 1 judge-usage \
    '{"known":true,"source":"claude-json","total_tokens":10,"cost_usd":0.0002,"currency":"USD"}'
add_usage "$subcent" demo subcent without 1 judge-usage \
    '{"known":true,"source":"claude-json","total_tokens":10,"cost_usd":0.0001,"currency":"USD"}'
finish_suite "$subcent"
run_report "$subcent" "$TMP/subcent.cli" --format cli
run_report "$subcent" "$TMP/subcent.md" --format md
run_report "$subcent" "$TMP/subcent.all" --format all >/dev/null 2>&1
assert_contains "$TMP/subcent.cli" "+\$0.00050" \
    "CLI shows a positive sub-half-cent delta with 5 decimals"
assert_contains "$TMP/subcent.cli" "\$0.00050 per point" \
    "CLI efficiency shows a sub-half-cent per-run cost"
# shellcheck disable=SC2016  # the regex contains literal dollars
assert_not_matches "$TMP/subcent.cli" '\+\$0\.000([^0-9]|$)' \
    "CLI never renders a positive sub-cent delta as +\$0.000"
assert_contains "$TMP/subcent.md" "+\$0.00050" \
    "Markdown shows a positive sub-half-cent delta with 5 decimals"
assert_contains "$subcent/report/report.html" "+\$0.00050" \
    "HTML shows a positive sub-half-cent delta with 5 decimals"

negative="$TMP/negative-subcent-cost"
new_suite "$negative" 1 '["skills/fixture"]'
add_scenario "$negative" demo negative-subcent \
    '{"context":"x","type":"weighted_checklist","checklist":[{"name":"A","description":"d","max_score":100}]}' \
    '["skills/fixture"]'
add_run "$negative" demo negative-subcent with 1 '[{"name":"A","score":99,"max":100}]'
add_run "$negative" demo negative-subcent without 1 '[{"name":"A","score":100,"max":100}]'
add_usage "$negative" demo negative-subcent with 1 usage \
    '{"known":true,"source":"opencode-json","total_tokens":90,"cost_usd":0.0004,"currency":"USD"}'
add_usage "$negative" demo negative-subcent without 1 usage \
    '{"known":true,"source":"opencode-json","total_tokens":100,"cost_usd":0.0008,"currency":"USD"}'
add_usage "$negative" demo negative-subcent with 1 judge-usage \
    '{"known":true,"source":"claude-json","total_tokens":10,"cost_usd":0.0001,"currency":"USD"}'
add_usage "$negative" demo negative-subcent without 1 judge-usage \
    '{"known":true,"source":"claude-json","total_tokens":10,"cost_usd":0.0002,"currency":"USD"}'
finish_suite "$negative"
run_report "$negative" "$TMP/negative-subcent.cli" --format cli
run_report "$negative" "$TMP/negative-subcent.md" --format md
run_report "$negative" "$TMP/negative-subcent.all" --format all >/dev/null 2>&1
assert_contains "$TMP/negative-subcent.cli" "-\$0.00050" \
    "CLI shows a negative sub-half-cent delta with 5 decimals"
# shellcheck disable=SC2016  # the regex contains literal dollars
assert_not_matches "$TMP/negative-subcent.cli" '-\$0\.000([^0-9]|$)' \
    "CLI never renders a negative sub-cent delta as -\$0.000"
assert_contains "$TMP/negative-subcent.md" "-\$0.00050" \
    "Markdown shows a negative sub-half-cent delta with 5 decimals"
assert_contains "$negative/report/report.html" "-\$0.00050" \
    "HTML shows a negative sub-half-cent delta with 5 decimals"

partial="$TMP/partial-cost"
new_suite "$partial" 1 '["skills/fixture"]'
add_scenario "$partial" demo partial-cost \
    '{"context":"x","type":"weighted_checklist","checklist":[{"name":"A","description":"d","max_score":100}]}' \
    '["skills/fixture"]'
add_run "$partial" demo partial-cost with 1 '[{"name":"A","score":100,"max":100}]'
add_run "$partial" demo partial-cost without 1 '[{"name":"A","score":50,"max":100}]'
for arm in with without; do
    add_usage "$partial" demo partial-cost "$arm" 1 usage \
        '{"known":true,"source":"opencode-json","total_tokens":1000,"cost_usd":0.015,"currency":"USD"}'
done
add_usage "$partial" demo partial-cost with 1 judge-usage \
    '{"known":true,"source":"claude-json","total_tokens":100,"cost_usd":0.003,"currency":"USD"}'
finish_suite "$partial"
run_report "$partial" "$TMP/partial-cost.json" --format json
run_report "$partial" "$TMP/partial-cost.cli" --format cli
if jq -e '
    .cost.cost_per_delta_point == null
    and .cost.with.combined.cost.total == 0.018
    and .cost.without.combined.cost.total == null
    and .cost.without.combined.cost.runs_known == 0
' "$TMP/partial-cost.json" >/dev/null; then
    pass "partial usage leaves the unsupported combined total null"
else
    fail "partial usage was combined as if the judge cost were zero"
fi
assert_contains "$TMP/partial-cost.cli" "partial" "CLI marks partial cost coverage"
assert_contains "$TMP/partial-cost.cli" "coverage: agent 2/2, judge 1/2, total 1/2" \
    "CLI coverage does not understate agent cost when the judge is absent"
assert_contains "$TMP/partial-cost.cli" "Efficiency: —" \
    "CLI refuses an efficiency claim under partial coverage"

# --- 5. JSON, Markdown and escaped self-contained HTML -----------------------

escape="$TMP/escape"
new_suite "$escape" 1 '["skills/fixture"]'
add_scenario "$escape" demo 'evil<&>' \
    '{"context":"x","type":"weighted_checklist","checklist":[{"name":"A <script>alert(1)&</script>","description":"d","max_score":100}]}'
add_run "$escape" demo 'evil<&>' with 1 '[{"name":"A <script>alert(1)&</script>","score":100,"max":100}]'
add_run "$escape" demo 'evil<&>' without 1 '[{"name":"A <script>alert(1)&</script>","score":50,"max":100}]'
finish_suite "$escape"

if run_report "$escape" "$TMP/escape.all" --format all; then
    pass "--format all renders every document"
else
    fail "--format all failed" "$TMP/escape.all.err"
fi
for file in report.json report.md report.html; do
    if [[ -f "$escape/report/$file" ]]; then
        pass "all writes $file"
    else
        fail "all did not write $file"
    fi
done
if jq -e . "$escape/report/report.json" >/dev/null 2>&1; then
    pass "the stable JSON report is valid JSON"
else
    fail "the stable JSON report is invalid"
fi
assert_eq "evil<&>" "$(jq -r '.scenarios[0].scenario' "$escape/report/report.json")" \
    "JSON preserves the raw scenario name"
assert_contains "$escape/report/report.html" 'evil&lt;&amp;&gt;' \
    "HTML escapes scenario names"
assert_contains "$escape/report/report.html" '&lt;script&gt;alert(1)&amp;&lt;/script&gt;' \
    "HTML escapes criterion names"
assert_not_contains "$escape/report/report.html" '<script>alert(1)' \
    "HTML never emits unescaped script text"
assert_not_contains "$escape/report/report.html" 'src="http' \
    "HTML is self-contained (no remote scripts)"
assert_not_contains "$escape/report/report.html" 'href="http' \
    "HTML is self-contained (no remote links)"

run_report "$escape" "$TMP/escape.stdout" --format json
if jq -e . "$TMP/escape.stdout" >/dev/null 2>&1 &&
    [[ "$(jq -r '.verdict.state' "$TMP/escape.stdout")" == "indicative" ]]; then
    pass "--format json prints the document to stdout"
else
    fail "--format json did not print valid JSON to stdout"
fi

# --- 6. make eval dry-run and selection -------------------------------------

make_dry="$TMP/make-dry.log"
if make -C "$PROJECT_ROOT" eval PLUGIN=git-workflow UNIT=skills/pr RUNS=1 DRY=1 \
    >"$make_dry" 2>&1; then
    pass "make eval DRY=1 succeeds without an agent"
else
    fail "make eval DRY=1 failed" "$make_dry"
fi
assert_contains "$make_dry" "Eval suite plan" "dry run prints the suite plan"
assert_contains "$make_dry" "git-workflow/pr-draft-read-replica" "dry run selects the unit's scenario"
assert_contains "$make_dry" "installs: skills/pr" "dry run shows the selected unit"
assert_contains "$make_dry" "Report destination:" "dry run prints the report destination"
assert_not_contains "$make_dry" "Agent: " "dry run does not start an agent"

make_skill="$TMP/make-skill.log"
make -C "$PROJECT_ROOT" eval PLUGIN=git-workflow SKILL=pr RUNS=1 DRY=1 \
    >"$make_skill" 2>&1
assert_contains "$make_skill" "installs: skills/pr" "SKILL maps to skills/<name>"

make_command="$TMP/make-command.log"
make -C "$PROJECT_ROOT" eval PLUGIN=code-quality UNIT=commands/review DRY=1 \
    >"$make_command" 2>&1
assert_contains "$make_command" "code-quality/review-all-flag" \
    "UNIT selects scenarios declaring the command"
assert_not_contains "$make_command" "code-quality/check-" \
    "UNIT excludes scenarios that declare a different command"

make_noagent="$TMP/make-noagent.log"
make_noagent_rc=0
make -C "$PROJECT_ROOT" eval PLUGIN=git-workflow RUNS=1 \
    >"$make_noagent" 2>&1 || make_noagent_rc=$?
if [[ "$make_noagent_rc" -ne 0 ]] &&
    grep -Fq "AGENT is required" "$make_noagent"; then
    pass "a real run without AGENT fails before an agent starts"
else
    fail "an empty AGENT was accepted" "$make_noagent"
fi
assert_contains "$make_noagent" "AGENT='claude -p'" \
    "the AGENT error shows a concrete example"

make_both="$TMP/make-both.log"
make_both_rc=0
make -C "$PROJECT_ROOT" eval SKILL=pr UNIT=skills/pr DRY=1 \
    >"$make_both" 2>&1 || make_both_rc=$?
if [[ "$make_both_rc" -ne 0 ]] &&
    grep -Fq "pass SKILL or UNIT, not both" "$make_both"; then
    pass "SKILL and UNIT together are rejected"
else
    fail "SKILL and UNIT together were accepted" "$make_both"
fi

echo
echo "================================"
echo "Results: $PASSED passed, $FAILED failed"
[[ "$FAILED" -eq 0 ]] || exit 1
exit 0
