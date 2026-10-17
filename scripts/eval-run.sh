#!/usr/bin/env bash

# Local eval runner
#
# Stages a scenario from plugins/<plugin>/evals/<scenario>/ in a throwaway
# sandbox, runs an agent against its task.md - once with the plugin's skills
# installed and once without - records what changed, and scores the parts of
# the rubric a script can score (a scenario's optional checks.sh).
#
# The rest of a rubric needs judgement, so runs stop there and leave a grading
# request behind: the criteria a script did not cover, plus the evidence. The
# review-evals skill, or you, does that part.
#
# Nothing here needs a network, a model, or credentials - the agent command you
# pass is what does the modelling, and it runs on this machine. That is why
# this tool is not wired into CI.
#
# Usage:
#   scripts/eval-run.sh --plugin git-workflow --scenario pr-draft-read-replica \
#       --agent-cmd 'claude -p' [--arm both] [--runs 1] [--keep]
#
#   scripts/eval-run.sh --selftest    # exercise the plumbing with a stub agent
#
# Results land in results/eval-run/<plugin>/<scenario>/ (git-ignored).

set -uo pipefail
shopt -s nullglob

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PLUGIN=""
SCENARIO=""
AGENT_CMD=""
ARM="both"
RUNS=1
TIMEOUT=900
OUT_DIR="$PROJECT_ROOT/results/eval-run"
SKILL_DIR=".claude/skills"
KEEP=0
SELFTEST=0

usage() {
    cat <<'EOF'
Local eval runner: stage a scenario, run an agent with and without the
plugin's skills, record what changed, and score what a script can score.

Usage:
  scripts/eval-run.sh --plugin <plugin> --scenario <scenario> [options]
  scripts/eval-run.sh --selftest

Options:
  --plugin NAME       plugin under test, e.g. git-workflow
  --scenario NAME     scenario directory under plugins/<plugin>/evals/
  --agent-cmd CMD     command that reads task.md on stdin, run in the sandbox
                      (e.g. 'claude -p', 'codex exec -')
  --arm ARM           both (default), with, or without
  --runs N            runs per arm (default 1)
  --timeout SECS      per-run deadline (default 900)
  --skill-dir DIR     where the 'with' arm installs the plugin's skills in the
                      sandbox (default .claude/skills)
  --out DIR           results directory (default results/eval-run)
  --keep              keep sandboxes for inspection
  --selftest          run the plumbing against a no-op agent; no model needed
EOF
}

die() {
    echo "eval-run: $*" >&2
    exit 1
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --plugin | --scenario | --agent-cmd | --arm | --runs | --timeout | --skill-dir | --out)
            if [[ $# -lt 2 || -z "$2" ]]; then
                echo "eval-run: $1 needs a value" >&2
                usage >&2
                exit 2
            fi
            case "$1" in
                --plugin) PLUGIN="$2" ;;
                --scenario) SCENARIO="$2" ;;
                --agent-cmd) AGENT_CMD="$2" ;;
                --arm) ARM="$2" ;;
                --runs) RUNS="$2" ;;
                --timeout) TIMEOUT="$2" ;;
                --skill-dir) SKILL_DIR="$2" ;;
                --out) OUT_DIR="$2" ;;
                *) ;;
            esac
            shift 2
            ;;
        --keep)
            KEEP=1
            shift
            ;;
        --selftest)
            SELFTEST=1
            shift
            ;;
        -h | --help)
            usage
            exit 0
            ;;
        *)
            echo "eval-run: unknown argument: $1" >&2
            usage >&2
            exit 2
            ;;
    esac
done

case "$ARM" in
    both | with | without) ;;
    *) die "--arm must be both, with or without" ;;
esac
[[ "$RUNS" =~ ^[0-9]+$ && "$RUNS" -ge 1 ]] || die "--runs must be a positive integer"
[[ "$TIMEOUT" =~ ^[0-9]+$ && "$TIMEOUT" -ge 1 ]] || die "--timeout must be a positive integer"

# --- helpers ------------------------------------------------------------------

# Best-effort deadline: GNU timeout when present, otherwise a watchdog that
# kills the agent shell (not its whole process group - keep runs short).
run_with_deadline() {
    local secs="$1"
    shift
    if command -v timeout >/dev/null 2>&1; then
        timeout "$secs" "$@"
        return
    fi
    "$@" &
    local pid=$! watcher
    (
        sleep "$secs"
        kill -TERM "$pid" 2>/dev/null
    ) >/dev/null 2>&1 &
    watcher=$!
    wait "$pid"
    local rc=$?
    kill -TERM "$watcher" 2>/dev/null
    return "$rc"
}

# Everything a grader needs to see the state a run produced: git history and
# status when the sandbox is a repository, plus a file listing.
snapshot_to() {
    local sandbox="$1" target="$2"
    {
        if git -C "$sandbox" rev-parse --git-dir >/dev/null 2>&1; then
            echo "== git log"
            git -C "$sandbox" log --oneline --all 2>/dev/null
            echo
            echo "== git status"
            git -C "$sandbox" status --porcelain 2>/dev/null
            echo
            echo "== staged diff stat"
            git -C "$sandbox" diff --cached --stat 2>/dev/null
        else
            echo "== not a git repository"
        fi
        echo
        echo "== files"
        (
            cd "$sandbox" || exit 1
            find . -path ./home -prune -o -path ./.git -prune -o -type f -print | sort
        )
    } >"$target"
}

# --- scenario resolution ------------------------------------------------------

if [[ "$SELFTEST" == "1" ]]; then
    # The first scenario that ships mechanical checks, so the selftest
    # exercises the whole path including scoring.
    selftest_scenario="$(find "$PROJECT_ROOT/plugins" -path '*/evals/*/checks.sh' -print 2>/dev/null | sort | head -1)"
    [[ -n "$selftest_scenario" ]] || die "--selftest needs a scenario with a checks.sh"
    SCENARIO_DIR="$(dirname "$selftest_scenario")"
    PLUGIN="$(echo "$SCENARIO_DIR" | sed -E 's#.*/plugins/([^/]+)/evals/.*#\1#')"
    SCENARIO="$(basename "$SCENARIO_DIR")"
    AGENT_CMD="true"
    OUT_DIR="$(mktemp -d "${TMPDIR:-/tmp}/eval-run-selftest-XXXXXX")"
    KEEP=1
else
    [[ -n "$PLUGIN" ]] || die "--plugin is required (or use --selftest)"
    [[ -n "$SCENARIO" ]] || die "--scenario is required (or use --selftest)"
    [[ -n "$AGENT_CMD" ]] ||
        die "--agent-cmd is required: pass the agent that should attempt task.md, e.g. --agent-cmd 'claude -p'"
    SCENARIO_DIR="$PROJECT_ROOT/plugins/$PLUGIN/evals/$SCENARIO"
fi

[[ -d "$SCENARIO_DIR" ]] || die "no such scenario: $SCENARIO_DIR"
PLUGIN_DIR="$PROJECT_ROOT/plugins/$PLUGIN"
CRITERIA="$SCENARIO_DIR/criteria.json"
SCENARIO_JSON="$SCENARIO_DIR/scenario.json"
TASK="$SCENARIO_DIR/task.md"
SETUP="$SCENARIO_DIR/inputs/setup.sh"

for required in "$TASK" "$CRITERIA" "$SCENARIO_JSON" "$SETUP"; do
    [[ -e "$required" ]] || die "scenario is incomplete: $required is missing"
done

echo "🧪 $PLUGIN/$SCENARIO"
echo "   arms: $ARM   runs per arm: $RUNS   agent: $AGENT_CMD"

RESULTS=()

run_once() {
    local arm="$1" run="$2"
    local sandbox rc out
    sandbox="$(mktemp -d "${TMPDIR:-/tmp}/eval-run-$SCENARIO-XXXXXX")"
    out="$OUT_DIR/$PLUGIN/$SCENARIO/$arm/run-$run"
    mkdir -p "$out" "$sandbox/home"

    # Stage the fixture exactly as scenario.json declares it.
    local include
    while IFS= read -r include; do
        [[ -z "$include" ]] && continue
        cp -R "$SCENARIO_DIR/$include" "$sandbox/" || die "cannot stage $include"
    done <<<"$(jq -r '.include[]?' "$SCENARIO_JSON")"

    # HOME is redirected for every step, so a fixture or an agent that writes
    # to ~ writes into the sandbox - never into the contributor's home.
    if ! (cd "$sandbox" && HOME="$sandbox/home" bash inputs/setup.sh) >"$out/setup.log" 2>&1; then
        [[ "$KEEP" == "0" ]] && rm -rf "$sandbox"
        die "setup failed for $arm/run-$run - see $out/setup.log"
    fi

    snapshot_to "$sandbox" "$out/before.txt"

    # The 'with' arm installs the plugin's skills into the sandbox, the way an
    # agent that has the plugin loaded would see them.
    if [[ "$arm" == "with" ]]; then
        mkdir -p "$sandbox/$SKILL_DIR"
        local skill name
        for skill in "$PLUGIN_DIR"/skills/*/; do
            name="$(basename "$skill")"
            mkdir -p "$sandbox/$SKILL_DIR/$name"
            # "$skill." copies the contents, which behaves the same on BSD and
            # GNU cp (a trailing slash alone does not).
            cp -R "$skill." "$sandbox/$SKILL_DIR/$name/"
        done
    fi

    (
        cd "$sandbox" || exit 1
        HOME="$sandbox/home" run_with_deadline "$TIMEOUT" bash -c "$AGENT_CMD"
    ) <"$TASK" >"$out/agent.log" 2>&1
    rc=$?

    snapshot_to "$sandbox" "$out/after.txt"
    if [[ "$KEEP" == "1" ]]; then
        (
            cd "$sandbox" || exit 1
            tar --exclude ./home -cf "$out/snapshot.tar" . 2>/dev/null
        )
    fi

    # Mechanical checks: the scenario's own checks.sh, validated against the
    # rubric it claims to score. A checks.sh that disagrees with criteria.json
    # is ignored rather than trusted.
    local covered="[]" mechanical="null" raw="$out/checks.raw.json"
    if [[ -x "$SCENARIO_DIR/checks.sh" ]]; then
        if (
            cd "$sandbox" || exit 1
            # shellcheck disable=SC2030  # the sandbox HOME is deliberately subshell-local
            export HOME="$sandbox/home" SANDBOX="$sandbox" SCENARIO_DIR CRITERIA
            bash "$SCENARIO_DIR/checks.sh"
        ) >"$raw" 2>"$out/checks.log"; then
            if jq -e . "$raw" >/dev/null 2>&1 &&
                jq -e --slurpfile crit "$CRITERIA" '
                    all(.[];
                        . as $item
                        | (($item.name | type) == "string")
                          and (($item.score | type) == "number")
                          and (($item.max | type) == "number")
                          and ($item.max == ($crit[0].checklist[] | select(.name == $item.name) | .max_score))
                    )' "$raw" >/dev/null 2>&1; then
                mechanical="$(jq -c '.' "$raw")"
                covered="$(jq -c '[.[].name]' "$raw")"
                mv "$raw" "$out/mechanical.json"
            else
                echo "  ⚠️  $arm/run-$run: checks.sh output does not match criteria.json - ignored"
            fi
        else
            echo "  ⚠️  $arm/run-$run: checks.sh failed - see $out/checks.log"
        fi
    fi

    # What a script could not score, for the judging step.
    jq -n --slurpfile crit "$CRITERIA" --argjson covered "$covered" \
        --arg plugin "$PLUGIN" --arg scenario "$SCENARIO" --arg arm "$arm" --argjson run "$run" \
        --arg evidence "$out" '
        {
            plugin: $plugin, scenario: $scenario, arm: $arm, run: $run,
            criteria: $crit[0],
            covered_by_checks: $covered,
            judgement_pending: [$crit[0].checklist[] | select(.name as $n | $covered | index($n) | not)],
            evidence: $evidence
        }' >"$out/grading-request.json"

    local score max items_json="$mechanical"
    [[ "$items_json" == "null" ]] && items_json="[]"
    score="$(jq -r '[.[].score] | add // 0' <<<"$items_json")"
    max="$(jq -r '[.[].max] | add // 0' <<<"$items_json")"

    jq -n --arg plugin "$PLUGIN" --arg scenario "$SCENARIO" --arg arm "$arm" --argjson run "$run" \
        --argjson exit "$rc" --argjson score "$score" --argjson max "$max" \
        --argjson mechanical "${mechanical:-null}" --arg evidence "$out" '
        {
            plugin: $plugin, scenario: $scenario, arm: $arm, run: $run,
            agent_exit: $exit,
            mechanical: {score: $score, max: $max, items: $mechanical},
            evidence: $evidence
        }' >"$out/result.json"

    [[ "$KEEP" == "0" ]] && rm -rf "$sandbox"
    echo "   $arm run $run: agent exit $rc, mechanical $score/$max"
    RESULTS+=("$out/result.json")
}

for arm in $([[ "$ARM" == "both" ]] && echo "without with" || echo "$ARM"); do
    for run in $(seq 1 "$RUNS"); do
        run_once "$arm" "$run"
    done
done

# --- summary ------------------------------------------------------------------

summary_dir="$OUT_DIR/$PLUGIN/$SCENARIO"
{
    echo "# $PLUGIN/$SCENARIO"
    echo
    echo "Agent: \`$AGENT_CMD\`  ·  runs per arm: $RUNS"
    echo
    echo "| arm | run | agent exit | mechanical |"
    echo "| --- | --- | --- | --- |"
    for result in "${RESULTS[@]}"; do
        jq -r '"| \(.arm) | \(.run) | \(.agent_exit) | \(.mechanical.score)/\(.mechanical.max) |"' "$result"
    done
    echo
    if [[ "$ARM" == "both" ]]; then
        without_total="$(jq -s -r '[.[] | select(.arm == "without") | .mechanical.score] | add // 0' "${RESULTS[@]}")"
        with_total="$(jq -s -r '[.[] | select(.arm == "with") | .mechanical.score] | add // 0' "${RESULTS[@]}")"
        echo "Mechanical total — with: $with_total, without: $without_total, delta: $((with_total - without_total))"
        echo
        echo "A scenario only proves something if the without-arm falls short somewhere:"
        echo "the delta is the skill's contribution. Grade the pending criteria in each"
        echo "grading-request.json to complete the picture."
    fi
} >"$summary_dir/summary.md"

cat "$summary_dir/summary.md"

if [[ "$SELFTEST" == "1" ]]; then
    echo
    echo "🧪 Selftest"
    passed=0
    failed=0
    check() {
        if [[ -e "$1" ]]; then
            echo "  ✓ $2"
            passed=$((passed + 1))
        else
            echo "  ✗ $2"
            failed=$((failed + 1))
        fi
    }
    with_dir="$OUT_DIR/$PLUGIN/$SCENARIO/with/run-1"
    without_dir="$OUT_DIR/$PLUGIN/$SCENARIO/without/run-1"
    check "$summary_dir/summary.md" "summary written"
    check "$with_dir/setup.log" "setup log written"
    check "$with_dir/mechanical.json" "checks.sh scored the with-arm"
    check "$with_dir/grading-request.json" "grading request written"
    check "$with_dir/snapshot.tar" "evidence bundle written"
    with_skill="$(tar -tf "$with_dir/snapshot.tar" 2>/dev/null | grep -c "$SKILL_DIR/.*/SKILL.md")"
    without_skill="$(tar -tf "$without_dir/snapshot.tar" 2>/dev/null | grep -c "$SKILL_DIR/.*/SKILL.md")"
    if [[ "${with_skill:-0}" -gt 0 && "${without_skill:-0}" -eq 0 ]]; then
        echo "  ✓ the with-arm installs skills, the without-arm does not"
        passed=$((passed + 1))
    else
        echo "  ✗ skill installation differs between arms (with=$with_skill without=$without_skill)"
        failed=$((failed + 1))
    fi
    # shellcheck disable=SC2031  # the point of the check is that the parent HOME stayed untouched
    if [[ -e "$HOME/bin/gh" || -e "$HOME/bin/curl" ]]; then
        echo "  ✗ the run wrote into the real HOME"
        failed=$((failed + 1))
    else
        echo "  ✓ the real HOME was untouched"
        passed=$((passed + 1))
    fi
    echo
    echo "📊 Selftest: $((passed + failed)), Passed: $passed, Failed: $failed"
    [[ "$failed" -eq 0 ]] || exit 1
    exit 0
fi

echo
echo "Evidence: $summary_dir"
