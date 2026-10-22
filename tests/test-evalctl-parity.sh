#!/usr/bin/env bash

# Shell-vs-evalctl parity gate
#
# Deterministic comparison between the legacy shell harness and the Go
# Milestone A harness. No model, network, or credentials are involved: every
# campaign uses `true` as the agent command in an isolated --out directory.
#
# It compares:
#   * scenario selection across plugin, unit, scenario, and shard filters
#   * plan output, including invocation counts
#   * the deterministic authoring gate and --changes output
#   * one comparison scenario and one functional scenario run end-to-end,
#     including status, arm/run results, mechanical scores, pending counts,
#     suite counts, criteria, and the grading index
#
# Usage:
#   tests/test-evalctl-parity.sh
#   EVALCTL_BIN=/path/to/evalctl tests/test-evalctl-parity.sh
#
# Deliberately no -e: report every mismatch in one run.

set -uo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT" || exit 1

SHELL_SUITE="scripts/eval-suite.sh"
SHELL_GATE="tests/test-evals.sh"
EVALCTL_BIN="${EVALCTL_BIN:-}"

PASSED=0
FAILED=0

pass() {
    echo "  ✓ $1"
    PASSED=$((PASSED + 1))
}

fail() {
    echo "  ✗ $1"
    FAILED=$((FAILED + 1))
    if [[ -n "${2:-}" && -s "$2" ]]; then
        sed 's/^/    /' "$2" | tail -20
    fi
}

TMP="$(mktemp -d "${TMPDIR:-/tmp}/evalctl-parity-XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

if [[ -z "$EVALCTL_BIN" ]]; then
    EVALCTL_BIN="$TMP/evalctl"
    if ! go build -trimpath -o "$EVALCTL_BIN" ./cmd/evalctl >"$TMP/build.log" 2>&1; then
        echo "could not build evalctl:" >&2
        cat "$TMP/build.log" >&2
        exit 1
    fi
fi
if [[ ! -x "$EVALCTL_BIN" ]]; then
    echo "evalctl binary is not executable: $EVALCTL_BIN" >&2
    exit 1
fi

echo "🔀 Shell/evalctl parity checks"
echo "=============================="
echo

# --- selection ----------------------------------------------------------------

compare_lists() {
    local label="$1"
    shift
    local shell_out go_out
    if ! shell_out="$("$SHELL_SUITE" --list "$@" 2>"$TMP/shell-list.err")"; then
        fail "$label (shell list failed)" "$TMP/shell-list.err"
        return
    fi
    if ! go_out="$("$EVALCTL_BIN" list "$@" 2>"$TMP/go-list.err")"; then
        fail "$label (evalctl list failed)" "$TMP/go-list.err"
        return
    fi
    if [[ "$shell_out" == "$go_out" ]]; then
        pass "$label"
    else
        diff <(printf '%s\n' "$shell_out") <(printf '%s\n' "$go_out") \
            >"$TMP/list.diff" 2>&1 || true
        fail "$label" "$TMP/list.diff"
    fi
}

compare_lists "list: every scenario"
compare_lists "list: --plugin git-workflow" --plugin git-workflow
compare_lists "list: --unit skills/pr" --unit skills/pr
compare_lists "list: --unit commands/review" --unit commands/review
compare_lists "list: --scenario regex" --scenario 'pr-draft'
compare_lists "list: --shard 1/4" --shard 1/4
compare_lists "list: --shard 3/4" --shard 3/4

# --- plan ---------------------------------------------------------------------

compare_plans() {
    local label="$1"
    shift
    local shell_out go_out
    if ! shell_out="$("$SHELL_SUITE" --plan "$@" 2>"$TMP/shell-plan.err")"; then
        fail "$label (shell plan failed)" "$TMP/shell-plan.err"
        return
    fi
    if ! go_out="$("$EVALCTL_BIN" suite --plan "$@" 2>"$TMP/go-plan.err")"; then
        fail "$label (evalctl plan failed)" "$TMP/go-plan.err"
        return
    fi
    if [[ "$shell_out" == "$go_out" ]]; then
        pass "$label"
    else
        diff <(printf '%s\n' "$shell_out") <(printf '%s\n' "$go_out") \
            >"$TMP/plan.diff" 2>&1 || true
        fail "$label" "$TMP/plan.diff"
    fi
}

compare_plans "plan: every scenario, --jobs 4 --runs 2" --jobs 4 --runs 2
compare_plans "plan: selected plugin and unit" --plugin git-workflow --unit skills/pr \
    --runs 3 --jobs 2 --shard 1/1

# --- deterministic authoring gate --------------------------------------------

if "$SHELL_GATE" >"$TMP/shell-gate.log" 2>&1; then
    pass "authoring gate: tests/test-evals.sh passes"
else
    fail "authoring gate: tests/test-evals.sh failed" "$TMP/shell-gate.log"
fi
if "$EVALCTL_BIN" check >"$TMP/go-gate.log" 2>&1; then
    pass "authoring gate: evalctl check passes"
else
    fail "authoring gate: evalctl check failed" "$TMP/go-gate.log"
fi

if diff <("$SHELL_GATE" --changes HEAD 2>"$TMP/shell-changes.err") \
    <("$EVALCTL_BIN" check --changes HEAD 2>"$TMP/go-changes.err") \
    >"$TMP/changes.diff" 2>&1; then
    pass "authoring gate: --changes HEAD output matches"
else
    fail "authoring gate: --changes HEAD output differs" "$TMP/changes.diff"
fi

# --- stub-agent campaigns -----------------------------------------------------

# project_results keeps only fields both harnesses define the same way. v3
# attempt paths, absolute evidence paths, harness versions, and timing are
# intentionally excluded: they are expected to differ.
project_results() {
    jq -S -s '
        sort_by(.plugin, .scenario)
        | map({
            plugin,
            scenario,
            mode,
            functional,
            status,
            runs: (
              .runs
              | sort_by(.arm, .run)
              | map({
                  arm,
                  run,
                  status,
                  agent_exit,
                  timed_out,
                  mechanical: {score: .mechanical.score, max: .mechanical.max},
                  pending,
                  judged,
                  usage_known: .usage.known
                })
            )
          })
    ' "$1"
}

project_counts() {
    jq -S '{
        planned: .counts.planned,
        executed: .counts.executed,
        skipped: .counts.skipped,
        runner_failures: .counts.runner_failures,
        agent_invocations: .counts.agent_invocations,
        pending_runs: .counts.pending_runs,
        scenarios_with_mechanical_checks: .counts.scenarios_with_mechanical_checks
    }' "$1"
}

compare_campaign() {
    local label="$1"
    shift
    local shell_out="$TMP/shell-$label"
    local go_out="$TMP/go-$label"

    if ! "$SHELL_SUITE" "$@" --agent-cmd true --no-auth --runs 1 --jobs 1 \
        --out "$shell_out" >"$TMP/shell-$label.log" 2>&1; then
        fail "$label campaign (shell run failed)" "$TMP/shell-$label.log"
        return
    fi
    if ! "$EVALCTL_BIN" suite "$@" --agent-cmd true --runs 1 --jobs 1 \
        --out "$go_out" >"$TMP/go-$label.log" 2>&1; then
        fail "$label campaign (evalctl run failed)" "$TMP/go-$label.log"
        return
    fi

    if diff <(project_results "$shell_out/results.jsonl") \
        <(project_results "$go_out/results.jsonl") >"$TMP/$label-results.diff" 2>&1; then
        pass "$label campaign: scenario and run results match"
    else
        fail "$label campaign: scenario and run results differ" "$TMP/$label-results.diff"
    fi

    if diff <(project_counts "$shell_out/suite.json") \
        <(project_counts "$go_out/suite.json") >"$TMP/$label-counts.diff" 2>&1; then
        pass "$label campaign: suite counts match"
    else
        fail "$label campaign: suite counts differ" "$TMP/$label-counts.diff"
    fi

    if diff -r "$shell_out/criteria" "$go_out/criteria" \
        >"$TMP/$label-criteria.diff" 2>&1; then
        pass "$label campaign: criteria snapshot matches"
    else
        fail "$label campaign: criteria snapshot differs" "$TMP/$label-criteria.diff"
    fi

    # Evidence paths legitimately differ (v1 run directory vs v3 attempt
    # evidence), so compare the row fields that select and count work.
    if diff \
        <(awk -F'\t' '{print $1"\t"$2"\t"$3"\t"$4"\t"$5}' "$shell_out/grading/index.tsv") \
        <(awk -F'\t' '{print $1"\t"$2"\t"$3"\t"$4"\t"$5}' "$go_out/grading/index.tsv") \
        >"$TMP/$label-grading.diff" 2>&1; then
        pass "$label campaign: grading index matches"
    else
        fail "$label campaign: grading index differs" "$TMP/$label-grading.diff"
    fi
}

# A comparison scenario exercises both arms and the mechanical checklist. A
# functional scenario exercises the with-only hook path.
compare_campaign comparison --plugin git-workflow --scenario 'git-workflow/pr-draft-read-replica$'
compare_campaign functional --plugin security-hooks --scenario 'security-hooks/scanner-clean-payload-allowed$'

echo
echo "Parity: $PASSED passed, $FAILED failed"
[[ "$FAILED" -eq 0 ]]
