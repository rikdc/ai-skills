#!/usr/bin/env bash

# Eval suite runner: plan, run, grade and aggregate eval scenarios.
#
# scripts/eval-run.sh runs one scenario. This script runs the campaign around
# it: discovery across every plugin, a deterministic plan, bounded parallelism,
# resume, sharding, optional judging, and one aggregate report. Staging stays
# in eval-run.sh - this script never reimplements the sandbox contract.
#
# Everything runs on the contributor's machine. The agent command is the only
# thing that talks to a model, so a suite costs whatever that command costs;
# no hosted eval credits are involved.
#
# Usage:
#   scripts/eval-suite.sh --agent-cmd 'claude -p' --jobs 4
#   scripts/eval-suite.sh --agent-cmd 'claude -p' --changed origin/main
#   scripts/eval-suite.sh --agent-cmd 'claude -p' --plugin git-workflow \
#       --judge-cmd 'claude -p --permission-mode acceptEdits'
#   scripts/eval-suite.sh --list
#   scripts/eval-suite.sh --plan --jobs 4
#   scripts/eval-suite.sh --selftest
#
# Results land in results/eval-suite/<timestamp>-<sha>/ (git-ignored).

set -uo pipefail
shopt -s nullglob

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
# The largest jq programs live in scripts/jq/ and are loaded with jq -f, so
# this script stays orchestration rather than embedded data manipulation.
JQ_DIR="$SCRIPT_DIR/jq"
RUN_ONE="$SCRIPT_DIR/eval-run.sh"
GRADE_ONE="$SCRIPT_DIR/eval-grade.sh"
EVAL_USAGE="$SCRIPT_DIR/eval-usage.sh"

AGENT_CMD=""
JUDGE_CMD=""
USAGE_CMD=""
JUDGE_USAGE_CMD=""
JUDGE_TIMEOUT=600
PLUGINS=()
UNITS=()
SCENARIO_FILTER=""
CHANGED_REF=""
ARM="both"
RUNS=1
JOBS=1
TIMEOUT=900
SKILL_DIR=".claude/skills"
OUT_DIR=""
OUT_EXPLICIT=0
RESUME=0
SHARD=""
KEEP=0
STRICT=0
AUTH=1
AUTH_PATHS=()
MODE="run"
SELFTEST=0

usage() {
    cat <<'EOF'
Run a suite of eval scenarios locally.

Usage:
  scripts/eval-suite.sh --agent-cmd CMD [options]
  scripts/eval-suite.sh --list [selection options]
  scripts/eval-suite.sh --plan [selection options]
  scripts/eval-suite.sh --selftest

Selection:
  --plugin NAME       only this plugin (repeatable)
  --unit UNIT         only scenarios declaring this unit (repeatable):
                      skills/<name>, commands/<name> or hooks/<name>.
                      The unit is also what each with-arm installs.
  --scenario REGEX    only scenarios whose plugin/scenario matches
  --changed REF       only scenarios a ref touched (plus the working tree)
  --shard I/N         deterministic slice I of N (1-based)

Execution:
  --agent-cmd CMD     command that reads task.md on stdin, run in the sandbox
                      (e.g. 'claude -p', 'codex exec -')
  --arm ARM           both (default), with, or without
  --runs N            runs per arm (default 1)
  --jobs N            concurrent scenarios (default 1)
  --timeout SECS      per-run agent deadline (default 900)
  --skill-dir DIR     sandbox dir for the with-arm's skills (default
                      .claude/skills); commands go to the sibling commands/ dir

Output:
  --out DIR           suite directory (default results/eval-suite/<stamp>-<sha>)
  --resume            skip scenarios already complete in --out (requires --out)
  --judge-cmd CMD     grade each executed run after it finishes
  --usage-cmd CMD     optional hook to normalize agent usage/tokens
  --judge-usage-cmd CMD
                      optional hook to normalize judge usage/tokens
  --judge-timeout S   per-run judge deadline (default 600)
  --keep              keep sandboxes (evidence bundles are always written)
  --no-auth           do not copy the agent CLI's login state into sandboxes
  --auth-path PATH    extra login/config file under your home to copy in
                      (repeatable; forwarded to every run)
  --strict            fail on runner errors, pending criteria, or delta <= 0

  --list              print selected plugin/scenario pairs and exit
  --plan              print the selection and invocation count, run nothing
  --selftest          exercise discovery, scheduling and aggregation with a
                      no-op agent; no model needed
EOF
}

die() {
    echo "eval-suite: $*" >&2
    exit 2
}

warn() {
    echo "eval-suite: $*" >&2
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --agent-cmd | --judge-cmd | --usage-cmd | --judge-usage-cmd | --plugin | --unit | \
            --scenario | --changed | --arm | --runs | --jobs | --timeout | --skill-dir | \
            --out | --shard | --judge-timeout | --auth-path)
            if [[ $# -lt 2 || -z "$2" ]]; then
                echo "eval-suite: $1 needs a value" >&2
                usage >&2
                exit 2
            fi
            case "$1" in
                --agent-cmd) AGENT_CMD="$2" ;;
                --judge-cmd) JUDGE_CMD="$2" ;;
                --usage-cmd) USAGE_CMD="$2" ;;
                --judge-usage-cmd) JUDGE_USAGE_CMD="$2" ;;
                --plugin) PLUGINS+=("$2") ;;
                --unit) UNITS+=("$2") ;;
                --scenario) SCENARIO_FILTER="$2" ;;
                --changed) CHANGED_REF="$2" ;;
                --arm) ARM="$2" ;;
                --runs) RUNS="$2" ;;
                --jobs) JOBS="$2" ;;
                --timeout) TIMEOUT="$2" ;;
                --skill-dir) SKILL_DIR="$2" ;;
                --out) OUT_DIR="$2"; OUT_EXPLICIT=1 ;;
                --shard) SHARD="$2" ;;
                --judge-timeout) JUDGE_TIMEOUT="$2" ;;
                --auth-path) AUTH_PATHS+=("$2") ;;
                *) ;;
            esac
            shift 2
            ;;
        --resume)
            RESUME=1
            shift
            ;;
        --keep)
            KEEP=1
            shift
            ;;
        --no-auth)
            AUTH=0
            shift
            ;;
        --strict)
            STRICT=1
            shift
            ;;
        --list)
            MODE="list"
            shift
            ;;
        --plan)
            MODE="plan"
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
            echo "eval-suite: unknown argument: $1" >&2
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
[[ "$JOBS" =~ ^[0-9]+$ && "$JOBS" -ge 1 ]] || die "--jobs must be a positive integer"
[[ "$TIMEOUT" =~ ^[0-9]+$ && "$TIMEOUT" -ge 1 ]] || die "--timeout must be a positive integer"
[[ "$JUDGE_TIMEOUT" =~ ^[0-9]+$ && "$JUDGE_TIMEOUT" -ge 1 ]] || die "--judge-timeout must be a positive integer"
[[ "$SKILL_DIR" != /* ]] || die "--skill-dir must be relative to the sandbox"
for auth_path in ${AUTH_PATHS[@]+"${AUTH_PATHS[@]}"}; do
    [[ "$auth_path" != /* && "$auth_path" != *..* ]] ||
        die "--auth-path must be a relative path under \$HOME: $auth_path"
done
for unit in ${UNITS[@]+"${UNITS[@]}"}; do
    [[ "$unit" =~ ^(skills|commands|hooks)/[A-Za-z0-9._-]+$ ]] ||
        die "invalid --unit '$unit' (expected skills/<name>, commands/<name> or hooks/<name>)"
done
[[ "$RESUME" == "0" || "$OUT_EXPLICIT" == "1" ]] || die "--resume needs --out pointing at a previous suite"
if [[ "$MODE" == "run" && "$SELFTEST" == "0" && -z "$AGENT_CMD" ]]; then
    die "--agent-cmd is required: pass the agent that should attempt each task.md, e.g. --agent-cmd 'claude -p'"
fi
[[ -x "$RUN_ONE" ]] || die "missing runner: $RUN_ONE"
[[ -x "$GRADE_ONE" ]] || die "missing grader: $GRADE_ONE"

# --- helpers ------------------------------------------------------------------

sha256_stream() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum
    else
        shasum -a 256
    fi | awk '{print $1}'
}

sha256_file() {
    local file="$1"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$file" | awk '{print $1}'
    else
        shasum -a 256 "$file" | awk '{print $1}'
    fi
}

hash_tree() {
    local root="$1"
    [[ -d "$root" ]] || return 0
    (
        cd "$root" || return 1
        find . -type f -print | LC_ALL=C sort | while IFS= read -r file; do
            local mode="file"
            [[ -x "$file" ]] && mode="exec"
            printf '%s %s ' "$mode" "$file"
            sha256_file "$file"
        done
    )
}

# A scenario is only reusable while everything that shapes its runs is
# unchanged: the fixture and rubric, and the plugin units the with-arm
# installs. Editing a skill therefore reruns that plugin's scenarios.
fingerprint_scenario() {
    local plugin="$1" scenario="$2"
    local plugin_dir="$PROJECT_ROOT/plugins/$plugin"
    local scenario_dir="$plugin_dir/evals/$scenario"
    {
        hash_tree "$scenario_dir"
        hash_tree "$plugin_dir/skills"
        hash_tree "$plugin_dir/commands"
        hash_tree "$plugin_dir/hooks"
        printf 'manifest '
        if [[ -f "$plugin_dir/.claude-plugin/plugin.json" ]]; then
            sha256_file "$plugin_dir/.claude-plugin/plugin.json"
        else
            printf 'none\n'
        fi
        if [[ -f "$plugin_dir/security-patterns.conf" ]]; then
            printf 'patterns '
            sha256_file "$plugin_dir/security-patterns.conf"
        fi
    } | sha256_stream
}

git_sha() {
    git -C "$PROJECT_ROOT" rev-parse --short HEAD 2>/dev/null || echo "nogit"
}

git_dirty() {
    if [[ -n "$(git -C "$PROJECT_ROOT" status --porcelain 2>/dev/null)" ]]; then
        echo "true"
    else
        echo "false"
    fi
}

arms() {
    if [[ "$ARM" == "both" ]]; then
        echo "without with"
    else
        echo "$ARM"
    fi
}

arm_count() {
    if [[ "$ARM" == "both" ]]; then
        echo 2
    else
        echo 1
    fi
}

unit_exists_in_plugin() {
    local plugin="$1" unit="$2" kind name
    kind="${unit%%/*}"
    name="${unit#*/}"
    case "$kind" in
        skills) [[ -d "$PROJECT_ROOT/plugins/$plugin/skills/$name" ]] ;;
        commands) [[ -f "$PROJECT_ROOT/plugins/$plugin/commands/$name.md" ]] ;;
        hooks) [[ -f "$PROJECT_ROOT/plugins/$plugin/hooks/$name.sh" ||
            -f "$PROJECT_ROOT/plugins/$plugin/hooks/$name" ]] ;;
        *) return 1 ;;
    esac
}

# Declared units are a newline-separated list: "unit" or "units" in
# scenario.json. Callers turn this into an array or match against it.
scenario_units() {
    local file="$1"
    [[ -f "$file" ]] || return 0
    jq -r '(.units // [.unit] // [])[] | select(. != null and . != "")' "$file" 2>/dev/null
}

# A unit must exist somewhere in the selected plugins (or anywhere when no
# --plugin narrowed the campaign). Otherwise a typo would select nothing and
# look like an empty campaign.
for unit in ${UNITS[@]+"${UNITS[@]}"}; do
    found=0
    if ((${#PLUGINS[@]} > 0)); then
        for plugin in "${PLUGINS[@]}"; do
            unit_exists_in_plugin "$plugin" "$unit" && found=1
        done
    else
        for plugin_dir in "$PROJECT_ROOT"/plugins/*/; do
            unit_exists_in_plugin "$(basename "$plugin_dir")" "$unit" && found=1
        done
    fi
    ((found)) || die "unknown --unit '$unit' (not shipped by the selected plugin(s))"
done

judgement_valid() {
    local judgement="$1" request="$2"
    [[ -f "$judgement" && -f "$request" ]] || return 1
    jq -e --slurpfile req "$request" '
        type == "array"
        and length > 0
        and ([.[].name] | sort) == ([$req[0].judgement_pending[].name] | sort)
        and all(.[];
            . as $item
            | ($item.name | type == "string")
              and ($item.score | type == "number")
              and ($item.max | type == "number")
              and $item.score >= 0
              and $item.score <= $item.max
              and ($req[0].judgement_pending[] | select(.name == $item.name) | .max_score) == $item.max
        )
    ' "$judgement" >/dev/null 2>&1
}

# --- selftest selection -------------------------------------------------------
# Three representative scenarios: one with mechanical checks, one
# command-only plugin, one skill-only plugin. Derived from the repository so
# the selftest survives plugins being added or renamed.

SELFTEST_PAIRS=()

remember_pair() {
    local pair="$1" existing
    [[ -n "$pair" ]] || return 0
    for existing in ${SELFTEST_PAIRS[@]+"${SELFTEST_PAIRS[@]}"}; do
        [[ "$existing" == "$pair" ]] && return 0
    done
    SELFTEST_PAIRS+=("$pair")
}

pair_for_dir() {
    local dir="$1"
    local rel="${dir#"$PROJECT_ROOT"/plugins/}"
    local plugin="${rel%%/*}"
    local rest="${rel#*/evals/}"
    printf '%s/%s' "$plugin" "${rest%%/*}"
}

if [[ "$SELFTEST" == "1" ]]; then
    checks_dir="$(find "$PROJECT_ROOT/plugins" -path '*/evals/*/checks.sh' -print -quit 2>/dev/null)"
    [[ -n "$checks_dir" ]] && remember_pair "$(pair_for_dir "$(dirname "$checks_dir")")"

    for plugin_dir in "$PROJECT_ROOT"/plugins/*/; do
        [[ -d "$plugin_dir/commands" && -d "$plugin_dir/evals" ]] || continue
        for scenario_dir in "$plugin_dir"evals/*/; do
            remember_pair "$(pair_for_dir "${scenario_dir%/}")"
            break
        done
        break
    done

    for plugin_dir in "$PROJECT_ROOT"/plugins/*/; do
        [[ -d "$plugin_dir/skills" && -d "$plugin_dir/evals" ]] || continue
        for scenario_dir in "$plugin_dir"evals/*/; do
            remember_pair "$(pair_for_dir "${scenario_dir%/}")"
            break
        done
        break
    done

    AGENT_CMD="true"
    JOBS=2
    OUT_DIR="$(mktemp -d "${TMPDIR:-/tmp}/eval-suite-selftest-XXXXXX")" ||
        die "cannot create a selftest directory"
    OUT_EXPLICIT=1
    RESUME=0
fi

# --- discovery ----------------------------------------------------------------

PLAN_PLUGIN=()
PLAN_SCENARIO=()
PLAN_FP=()
PLAN_UNIT=()
PLAN_DECLARED=()

changed_scenarios=()
changed_plugins=()
if [[ -n "$CHANGED_REF" ]]; then
    git -C "$PROJECT_ROOT" rev-parse --git-dir >/dev/null 2>&1 ||
        die "--changed needs a git checkout"
    # Capture each command separately: the brace group's exit status is the
    # last command, so a failed git diff was previously masked by a successful
    # git ls-files and the bad ref never reached its error.
    if ! changed_diff="$(git -C "$PROJECT_ROOT" diff --name-only "$CHANGED_REF" -- plugins 2>/dev/null)"; then
        die "cannot diff against '$CHANGED_REF' (fetch it first)"
    fi
    if ! changed_untracked="$(git -C "$PROJECT_ROOT" ls-files --others --exclude-standard -- plugins 2>/dev/null)"; then
        die "cannot list untracked files under plugins"
    fi
    changed_paths="$(printf '%s\n%s\n' "$changed_diff" "$changed_untracked" | sort -u)"
    while IFS= read -r path; do
        [[ "$path" == plugins/*/* ]] || continue
        rest="${path#plugins/}"
        plugin="${rest%%/*}"
        tail="${rest#*/}"
        case "$tail" in
            evals/*)
                scenario="${tail#evals/}"
                scenario="${scenario%%/*}"
                [[ -n "$scenario" ]] && changed_scenarios+=("$plugin/$scenario")
                ;;
            skills/* | commands/* | hooks/* | .claude-plugin/* | .tessl-plugin/* | \
                security-patterns.conf)
                changed_plugins+=("$plugin")
                ;;
            *) ;;
        esac
    done <<<"$changed_paths"
fi

is_changed() {
    local plugin="$1" scenario="$2" item
    for item in ${changed_scenarios[@]+"${changed_scenarios[@]}"}; do
        [[ "$item" == "$plugin/$scenario" ]] && return 0
    done
    for item in ${changed_plugins[@]+"${changed_plugins[@]}"}; do
        [[ "$item" == "$plugin" ]] && return 0
    done
    return 1
}

plugin_selected() {
    local plugin="$1" wanted
    ((${#PLUGINS[@]} == 0)) && return 0
    for wanted in "${PLUGINS[@]}"; do
        [[ "$wanted" == "$plugin" ]] && return 0
    done
    return 1
}

selftest_selected() {
    local pair="$1" wanted
    for wanted in ${SELFTEST_PAIRS[@]+"${SELFTEST_PAIRS[@]}"}; do
        [[ "$wanted" == "$pair" ]] && return 0
    done
    return 1
}

shard_index=0
shard_count=1
if [[ -n "$SHARD" ]]; then
    [[ "$SHARD" =~ ^([0-9]+)/([0-9]+)$ ]] || die "--shard must look like 1/3"
    shard_index="${BASH_REMATCH[1]}"
    shard_count="${BASH_REMATCH[2]}"
    ((shard_count >= 1)) || die "--shard denominator must be at least 1"
    ((shard_index >= 1 && shard_index <= shard_count)) ||
        die "--shard index must be between 1 and $shard_count"
fi

position=0
for scenario_dir in "$PROJECT_ROOT"/plugins/*/evals/*/; do
    plugin="$(basename "$(dirname "$(dirname "$scenario_dir")")")"
    scenario="$(basename "$scenario_dir")"
    pair="$plugin/$scenario"

    if [[ "$SELFTEST" == "1" ]]; then
        selftest_selected "$pair" || continue
    fi
    plugin_selected "$plugin" || continue
    if [[ -n "$SCENARIO_FILTER" && ! "$pair" =~ $SCENARIO_FILTER ]]; then
        continue
    fi
    if [[ -n "$CHANGED_REF" ]] && ! is_changed "$plugin" "$scenario"; then
        continue
    fi

    complete=1
    for required in task.md criteria.json scenario.json inputs/setup.sh; do
        [[ -e "$scenario_dir/$required" ]] || complete=0
    done
    if [[ "$complete" == "0" ]]; then
        warn "skipping incomplete scenario: $plugin/$scenario"
        continue
    fi

    # The unit is what the with-arm installs and what --unit selects on. A
    # declared unit that does not exist is a broken scenario, not an empty
    # selection, so fail clearly.
    declared_units=""
    while IFS= read -r declared; do
        [[ -n "$declared" ]] || continue
        unit_exists_in_plugin "$plugin" "$declared" ||
            die "scenario $pair declares unknown unit '$declared' for plugin '$plugin'"
        case " $declared_units " in
            *" $declared "*) ;;
            *) declared_units="$declared_units $declared" ;;
        esac
    done <<<"$(scenario_units "$scenario_dir/scenario.json")"
    declared_units="${declared_units# }"

    selected_units=""
    if ((${#UNITS[@]} > 0)); then
        [[ -n "$declared_units" ]] ||
            die "scenario $pair has no unit metadata; add \"unit\": \"skills/<name>\" or \"units\": [...] to scenario.json"
        for wanted in "${UNITS[@]}"; do
            for declared in $declared_units; do
                [[ "$wanted" == "$declared" ]] &&
                    selected_units="$selected_units $wanted"
            done
        done
        selected_units="${selected_units# }"
        [[ -n "$selected_units" ]] || continue
    fi

    position=$((position + 1))
    if ((shard_count > 1)); then
        if (((position - 1) % shard_count != shard_index - 1)); then
            continue
        fi
    fi

    PLAN_PLUGIN+=("$plugin")
    PLAN_SCENARIO+=("$scenario")
    PLAN_UNIT+=("$selected_units")
    PLAN_DECLARED+=("$declared_units")
done

if [[ "$SELFTEST" == "1" && "${#PLAN_PLUGIN[@]}" -lt 3 ]]; then
    die "selftest expected 3 representative scenarios, found ${#PLAN_PLUGIN[@]}"
fi
if [[ "${#PLAN_PLUGIN[@]}" -eq 0 ]]; then
    if ((${#UNITS[@]} > 0)); then
        die "no scenarios declare unit(s): ${UNITS[*]}"
    fi
    die "no scenarios selected"
fi

SCENARIO_COUNT="${#PLAN_PLUGIN[@]}"
INVOCATIONS=$((SCENARIO_COUNT * $(arm_count) * RUNS))

if [[ "$MODE" == "list" ]]; then
    for i in "${!PLAN_PLUGIN[@]}"; do
        printf '%s/%s\n' "${PLAN_PLUGIN[$i]}" "${PLAN_SCENARIO[$i]}"
    done
    exit 0
fi

if [[ "$MODE" == "plan" ]]; then
    echo "Eval suite plan"
    echo
    for i in "${!PLAN_PLUGIN[@]}"; do
        printf '  %s/%s\n' "${PLAN_PLUGIN[$i]}" "${PLAN_SCENARIO[$i]}"
        if [[ -n "${PLAN_UNIT[$i]}" ]]; then
            printf '      installs: %s\n' "${PLAN_UNIT[$i]}"
        else
            printf '      installs: all units (%s)\n' "${PLAN_DECLARED[$i]:-undeclared}"
        fi
    done
    echo
    echo "Scenarios: $SCENARIO_COUNT"
    echo "Arms: $ARM ($(arm_count))  ·  runs per arm: $RUNS"
    echo "Agent invocations: $INVOCATIONS"
    if [[ -n "$JUDGE_CMD" ]]; then
        echo "Judge invocations: up to $INVOCATIONS"
    fi
    echo "Jobs: $JOBS  ·  per-run timeout: ${TIMEOUT}s"
    exit 0
fi

# Fingerprints are only needed to run or resume; list and plan stay cheap.
for i in "${!PLAN_PLUGIN[@]}"; do
    PLAN_FP+=("$(fingerprint_scenario "${PLAN_PLUGIN[$i]}" "${PLAN_SCENARIO[$i]}")")
done

# --- suite directory ----------------------------------------------------------

if [[ "$OUT_EXPLICIT" == "0" ]]; then
    OUT_DIR="$PROJECT_ROOT/results/eval-suite/$(date -u +%Y%m%dT%H%M%SZ)-$(git_sha)"
fi
if [[ "$RESUME" == "1" ]]; then
    [[ -f "$OUT_DIR/manifest.json" ]] ||
        die "--resume found no manifest in $OUT_DIR (is that a suite directory?)"
else
    if [[ -d "$OUT_DIR" && -n "$(ls -A "$OUT_DIR" 2>/dev/null)" ]]; then
        die "$OUT_DIR is not empty; use --resume or a new --out"
    fi
fi

if ! mkdir -p "$OUT_DIR/runs" "$OUT_DIR/logs" "$OUT_DIR/grading" "$OUT_DIR/criteria"; then
    die "cannot create suite directories under $OUT_DIR"
fi

# Keep the rubric with the suite so a report remains readable even if the
# scenario changes later. The runner still uses the repository copy.
for i in "${!PLAN_PLUGIN[@]}"; do
    plugin="${PLAN_PLUGIN[$i]}"
    scenario="${PLAN_SCENARIO[$i]}"
    if ! mkdir -p "$OUT_DIR/criteria/$plugin"; then
        die "cannot create criteria directory under $OUT_DIR"
    fi
    if ! cp "$PROJECT_ROOT/plugins/$plugin/evals/$scenario/criteria.json" \
        "$OUT_DIR/criteria/$plugin/$scenario.json"; then
        die "cannot copy criteria for $plugin/$scenario into $OUT_DIR"
    fi
done

HARNESS_FP="$(sha256_file "$RUN_ONE")"

if ((${#AUTH_PATHS[@]} > 0)); then
    auth_paths_json="$(printf '%s\n' "${AUTH_PATHS[@]}" | jq -R . | jq -s .)"
else
    auth_paths_json="[]"
fi
if ((${#UNITS[@]} > 0)); then
    units_json="$(printf '%s\n' "${UNITS[@]}" | jq -R . | jq -s 'unique')"
else
    units_json="[]"
fi

config_json="$(jq -nc \
    --arg agent "$AGENT_CMD" --arg arm "$ARM" --argjson runs "$RUNS" \
    --argjson timeout "$TIMEOUT" --arg skill "$SKILL_DIR" --argjson auth "$AUTH" \
    --argjson auth_paths "$auth_paths_json" --argjson units "$units_json" \
    --arg harness "$HARNESS_FP" \
    --arg judge "$JUDGE_CMD" --arg usage "$USAGE_CMD" --arg judge_usage "$JUDGE_USAGE_CMD" \
    --argjson judge_timeout "$JUDGE_TIMEOUT" \
    --arg git_sha "$(git_sha)" --argjson git_dirty "$(git_dirty)" '
    {
        agent_cmd: $agent,
        arm: $arm,
        runs: $runs,
        timeout: $timeout,
        skill_dir: $skill,
        auth: $auth,
        auth_paths: $auth_paths,
        units: $units,
        harness: $harness,
        judge_cmd: $judge,
        usage_cmd: $usage,
        judge_usage_cmd: $judge_usage,
        judge_timeout: $judge_timeout,
        git_sha: $git_sha,
        git_dirty: $git_dirty
    }')"

scenarios_json="$(
    for i in "${!PLAN_PLUGIN[@]}"; do
        jq -nc --arg plugin "${PLAN_PLUGIN[$i]}" --arg scenario "${PLAN_SCENARIO[$i]}" \
            --arg fingerprint "${PLAN_FP[$i]}" --arg units "${PLAN_DECLARED[$i]}" \
            '{plugin: $plugin, scenario: $scenario, fingerprint: $fingerprint,
              units: ($units | split(" ") | map(select(. != "")))}'
    done | jq -s '.'
)"

MANIFEST_NEEDS_REWRITE=0
if [[ "$RESUME" == "1" ]]; then
    MANIFEST_NEEDS_REWRITE=1
    jq -e --arg agent "$AGENT_CMD" --arg arm "$ARM" --argjson runs "$RUNS" \
        --argjson timeout "$TIMEOUT" --arg skill "$SKILL_DIR" --argjson auth "$AUTH" \
        --argjson auth_paths "$auth_paths_json" --argjson units "$units_json" \
        --arg harness "$HARNESS_FP" --arg usage "$USAGE_CMD" \
        --arg judge_usage "$JUDGE_USAGE_CMD" '
        .config.agent_cmd == $agent
        and .config.arm == $arm
        and .config.runs == $runs
        and .config.timeout == $timeout
        and .config.skill_dir == $skill
        and .config.auth == $auth
        and ((.config.auth_paths // []) | sort) == ($auth_paths | sort)
        and ((.config.units // []) | sort) == ($units | sort)
        and .config.harness == $harness
        and (.config.usage_cmd // "") == $usage
        and (.config.judge_usage_cmd // "") == $judge_usage
    ' "$OUT_DIR/manifest.json" >/dev/null 2>&1 ||
        die "$OUT_DIR was created with a different agent, arm, runs, timeout, skill dir, auth mode, auth paths, units, usage commands or runner; use a new --out"
else
    jq -n --argjson config "$config_json" --argjson scenarios "$scenarios_json" \
        --arg created_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" '
        {version: 1, created_at: $created_at, config: $config, scenarios: $scenarios}' \
        >"$OUT_DIR/manifest.json"

    : >"$OUT_DIR/plan.tsv"
    for i in "${!PLAN_PLUGIN[@]}"; do
        printf '%s\t%s\t%s\n' "${PLAN_PLUGIN[$i]}" "${PLAN_SCENARIO[$i]}" "${PLAN_FP[$i]}" \
            >>"$OUT_DIR/plan.tsv"
    done
fi

resume_complete() {
    local i="$1"
    local plugin="${PLAN_PLUGIN[$i]}" scenario="${PLAN_SCENARIO[$i]}"
    local previous arm run
    [[ "$RESUME" == "1" ]] || return 1

    previous="$(jq -r --arg p "$plugin" --arg s "$scenario" '
        .scenarios[] | select(.plugin == $p and .scenario == $s) | .fingerprint // empty
    ' "$OUT_DIR/manifest.json" 2>/dev/null)"
    [[ "$previous" == "${PLAN_FP[$i]}" ]] || return 1

    for arm in $(arms); do
        for run in $(seq 1 "$RUNS"); do
            [[ -f "$OUT_DIR/runs/$plugin/$scenario/$arm/run-$run/result.json" ]] || return 1
        done
    done
    return 0
}

# --- execution ----------------------------------------------------------------

SUITE_START="$(date +%s)"
EXECUTED=0
SKIPPED=0

pids=()
job_labels=()
job_logs=()
job_started=()
job_status=()

start_job() {
    local i="$1"
    local plugin="${PLAN_PLUGIN[$i]}" scenario="${PLAN_SCENARIO[$i]}"
    local label="$plugin/$scenario"
    local log="$OUT_DIR/logs/$plugin/$scenario.log"
    local status="$OUT_DIR/logs/$plugin/$scenario.status"
    local passthrough_args=()
    [[ "$KEEP" == "1" ]] && passthrough_args+=(--keep)
    [[ "$AUTH" == "0" ]] && passthrough_args+=(--no-auth)
    [[ -n "$USAGE_CMD" ]] && passthrough_args+=(--usage-cmd "$USAGE_CMD")
    local auth_path
    for auth_path in ${AUTH_PATHS[@]+"${AUTH_PATHS[@]}"}; do
        passthrough_args+=(--auth-path "$auth_path")
    done
    local plan_unit
    local -a plan_units=()
    read -r -a plan_units <<<"${PLAN_UNIT[$i]:-}"
    for plan_unit in ${plan_units[@]+"${plan_units[@]}"}; do
        passthrough_args+=(--unit "$plan_unit")
    done

    if ! mkdir -p "$OUT_DIR/logs/$plugin"; then
        die "cannot create log directory under $OUT_DIR"
    fi
    rm -f "$status"
    printf '▶ %s\n' "$label"

    (
        rc=0
        "$RUN_ONE" --plugin "$plugin" --scenario "$scenario" \
            --agent-cmd "$AGENT_CMD" --arm "$ARM" --runs "$RUNS" \
            --timeout "$TIMEOUT" --skill-dir "$SKILL_DIR" \
            --out "$OUT_DIR/runs" ${passthrough_args[@]+"${passthrough_args[@]}"} || rc=$?

        # Judging is per run, after its evidence exists. A grading failure is
        # left pending rather than over-scored; it never fails the run itself.
        if ((rc == 0)) && [[ -n "$JUDGE_CMD" ]]; then
            for dir in "$OUT_DIR/runs/$plugin/$scenario"/*/run-*; do
                [[ -f "$dir/grading-request.json" ]] || continue
                grade_args=(--run-dir "$dir" --judge-cmd "$JUDGE_CMD" \
                    --timeout "$JUDGE_TIMEOUT")
                [[ -n "$JUDGE_USAGE_CMD" ]] && grade_args+=(--usage-cmd "$JUDGE_USAGE_CMD")
                "$GRADE_ONE" "${grade_args[@]}" || true
            done
        fi

        # The parent polls this file instead of kill -0: a finished child can
        # linger as a zombie, which kill -0 reports as alive.
        printf '%s\n' "$rc" >"$status"
    ) >"$log" 2>&1 &

    pids+=("$!")
    job_labels+=("$label")
    job_logs+=("$log")
    job_started+=("$(date +%s)")
    job_status+=("$status")
}

reap_jobs() {
    local alive_pids=() alive_labels=() alive_logs=()
    local alive_started=() alive_status=()
    local i rc finished
    for i in "${!pids[@]}"; do
        finished=0
        rc=0
        if [[ -f "${job_status[$i]}" ]]; then
            finished=1
            rc="$(cat "${job_status[$i]}" 2>/dev/null)"
            [[ "$rc" =~ ^[0-9]+$ ]] || rc=1
            wait "${pids[$i]}" 2>/dev/null
        elif ! kill -0 "${pids[$i]}" 2>/dev/null; then
            finished=1
            rc=1
            wait "${pids[$i]}" 2>/dev/null
        fi

        if ((finished)); then
            if ((rc != 0)); then
                printf '✗ %s (runner exit %s; log: %s)\n' \
                    "${job_labels[$i]}" "$rc" "${job_logs[$i]}"
            else
                printf '✓ %s (%ss)\n' \
                    "${job_labels[$i]}" "$(($(date +%s) - ${job_started[$i]}))"
            fi
        else
            alive_pids+=("${pids[$i]}")
            alive_labels+=("${job_labels[$i]}")
            alive_logs+=("${job_logs[$i]}")
            alive_started+=("${job_started[$i]}")
            alive_status+=("${job_status[$i]}")
        fi
    done
    pids=(${alive_pids[@]+"${alive_pids[@]}"})
    job_labels=(${alive_labels[@]+"${alive_labels[@]}"})
    job_logs=(${alive_logs[@]+"${alive_logs[@]}"})
    job_started=(${alive_started[@]+"${alive_started[@]}"})
    job_status=(${alive_status[@]+"${alive_status[@]}"})
}

# Deepest-first PID list for one tracked job: the agent and its helpers come
# before the shell that launched them. Only explicit PIDs are signalled - the
# suite never signals its own process group.
# shellcheck disable=SC2329  # invoked by abort_suite and by the signal trap
job_tree() {
    local parent="$1" child
    while IFS= read -r child; do
        [[ -n "$child" ]] || continue
        job_tree "$child"
    done < <(pgrep -P "$parent" 2>/dev/null)
    printf '%s\n' "$parent"
}

# Non-interactive shells start asynchronous jobs with SIGINT ignored, so Ctrl-C
# would otherwise kill the suite while paid agent jobs keep running until their
# timeouts and sandbox credentials survive. TERM each tracked tree, give
# eval-run.sh a moment to run its own sandbox cleanup, then KILL what ignored
# TERM.
# shellcheck disable=SC2329  # invoked only by the INT/TERM trap strings
abort_suite() {
    local status="$1" signal_name="$2" pid child
    local -a trees=()
    trap - INT TERM
    trap '' INT TERM
    printf 'eval-suite: %s; stopping %d job(s)\n' "$signal_name" "${#pids[@]}" >&2
    for pid in ${pids[@]+"${pids[@]}"}; do
        while IFS= read -r child; do
            [[ -n "$child" ]] || continue
            trees+=("$child")
        done < <(job_tree "$pid")
    done
    for pid in ${trees[@]+"${trees[@]}"}; do
        kill -TERM "$pid" 2>/dev/null || true
    done
    sleep 2
    for pid in ${trees[@]+"${trees[@]}"}; do
        if kill -0 "$pid" 2>/dev/null; then
            kill -KILL "$pid" 2>/dev/null || true
        fi
    done
    for pid in ${pids[@]+"${pids[@]}"}; do
        wait "$pid" 2>/dev/null || true
    done
    exit "$status"
}

trap 'abort_suite 130 INT' INT
trap 'abort_suite 143 TERM' TERM

echo "🧪 Eval suite: $SCENARIO_COUNT scenarios, $ARM arms, $RUNS run(s), $JOBS job(s)"
echo "   Agent: $AGENT_CMD"
echo "   Out:   $OUT_DIR"

next=0
while ((next < SCENARIO_COUNT || ${#pids[@]} > 0)); do
    while ((${#pids[@]} < JOBS && next < SCENARIO_COUNT)); do
        if resume_complete "$next"; then
            SKIPPED=$((SKIPPED + 1))
            next=$((next + 1))
            continue
        fi
        start_job "$next"
        EXECUTED=$((EXECUTED + 1))
        next=$((next + 1))
    done
    if ((${#pids[@]} > 0)); then
        reap_jobs
        sleep 0.2
    fi
done

# A narrowed --resume leaves the old manifest describing scenarios that were
# not part of this pass. Rewriting it after resume_complete has used the old
# fingerprints keeps the manifest and suite.json in sync.
if [[ "$MANIFEST_NEEDS_REWRITE" == "1" ]]; then
    if ! jq --argjson scenarios "$scenarios_json" '.scenarios = $scenarios' \
        "$OUT_DIR/manifest.json" >"$OUT_DIR/manifest.json.tmp"; then
        die "cannot rewrite the resume manifest in $OUT_DIR"
    fi
    if ! mv "$OUT_DIR/manifest.json.tmp" "$OUT_DIR/manifest.json"; then
        die "cannot replace the resume manifest in $OUT_DIR"
    fi
    : >"$OUT_DIR/plan.tsv"
    for i in "${!PLAN_PLUGIN[@]}"; do
        printf '%s\t%s\t%s\n' "${PLAN_PLUGIN[$i]}" "${PLAN_SCENARIO[$i]}" "${PLAN_FP[$i]}" \
            >>"$OUT_DIR/plan.tsv"
    done
fi

# Runs that were skipped by --resume never entered a job, and a judge can fail
# mid-campaign. Give every unfinished run one more pass; eval-grade skips a
# valid judgement, so this is cheap for runs already graded inline.
if [[ -n "$JUDGE_CMD" ]]; then
    for i in "${!PLAN_PLUGIN[@]}"; do
        plugin="${PLAN_PLUGIN[$i]}"
        scenario="${PLAN_SCENARIO[$i]}"
        for dir in "$OUT_DIR/runs/$plugin/$scenario"/*/run-*; do
            [[ -f "$dir/grading-request.json" ]] || continue
            judgement_valid "$dir/judgement.json" "$dir/grading-request.json" && continue
            grade_args=(--run-dir "$dir" --judge-cmd "$JUDGE_CMD" \
                --timeout "$JUDGE_TIMEOUT")
            [[ -n "$JUDGE_USAGE_CMD" ]] && grade_args+=(--usage-cmd "$JUDGE_USAGE_CMD")
            "$GRADE_ONE" "${grade_args[@]}" || true
        done
    done
fi

# Backfill missing usage files from logs that already exist. This runs for
# every selected run, not only resumed ones: it is cheap, never reruns a
# model, and keeps older suite directories reportable.
backfill_usage() {
    local dir="$1" kind="$2" arm="$3" run="$4"
    local log out cmd exit_code
    if [[ "$kind" == "agent" ]]; then
        log="$dir/agent.log"
        out="$dir/usage.json"
        cmd="$USAGE_CMD"
        exit_code="$(jq -r '.agent_exit // 0' "$dir/result.json" 2>/dev/null || echo 0)"
    else
        log="$dir/judge.log"
        out="$dir/judge-usage.json"
        cmd="$JUDGE_USAGE_CMD"
        exit_code=0
    fi
    [[ -f "$out" ]] && return 0
    [[ -f "$log" ]] || return 0

    local args=(--log "$log" --output "$out" --run-dir "$dir"
        --arm "$arm" --run "$run" --agent-exit "$exit_code")
    [[ -n "$cmd" ]] && args+=(--cmd "$cmd")
    if ! "$EVAL_USAGE" "${args[@]}" >/dev/null 2>"$dir/$kind-usage.log"; then
        printf '{"known":false,"source":"none"}\n' >"$out"
    fi
    if ! jq -e 'type == "object" and (.known | type) == "boolean"' "$out" >/dev/null 2>&1; then
        printf '{"known":false,"source":"none"}\n' >"$out"
    fi
}

for i in "${!PLAN_PLUGIN[@]}"; do
    plugin="${PLAN_PLUGIN[$i]}"
    scenario="${PLAN_SCENARIO[$i]}"
    for arm in $(arms); do
        for run in $(seq 1 "$RUNS"); do
            run_dir="$OUT_DIR/runs/$plugin/$scenario/$arm/run-$run"
            [[ -d "$run_dir" ]] || continue
            backfill_usage "$run_dir" agent "$arm" "$run"
            backfill_usage "$run_dir" judge "$arm" "$run"
        done
    done
done

# --- aggregation --------------------------------------------------------------

summarise_run() {
    local dir="$1" arm="$2" run="$3" denominator="$4"
    local result="$dir/result.json"
    local usage_json judge_usage_json
    usage_json="$(jq -c '
        if type == "object" and (.known | type) == "boolean" then .
        else {known: false, source: "none"} end
    ' "$dir/usage.json" 2>/dev/null)"
    [[ -n "$usage_json" ]] || usage_json='{"known":false,"source":"none"}'
    judge_usage_json="$(jq -c '
        if type == "object" and (.known | type) == "boolean" then .
        else {known: false, source: "none"} end
    ' "$dir/judge-usage.json" 2>/dev/null)"
    [[ -n "$judge_usage_json" ]] || judge_usage_json='{"known":false,"source":"none"}'

    if [[ ! -f "$result" ]]; then
        jq -nc --arg arm "$arm" --argjson run "$run" --arg evidence "$dir" \
            --argjson usage "$usage_json" --argjson judge_usage "$judge_usage_json" '
            {
                arm: $arm, run: $run, status: "missing",
                agent_exit: null, timed_out: false,
                mechanical: {score: 0, max: 0}, judged: null,
                pending: 0, score: 0, max: 0, percent: null,
                usage: $usage, judge_usage: $judge_usage,
                evidence: $evidence
            }'
        return
    fi

    local mech_score mech_max agent_exit timed_out pending
    mech_score="$(jq -r '.mechanical.score // 0' "$result")"
    mech_max="$(jq -r '.mechanical.max // 0' "$result")"
    agent_exit="$(jq -r '.agent_exit // 0' "$result")"
    timed_out="$(jq -r '.timed_out // false' "$result")"
    pending=0
    if [[ -f "$dir/grading-request.json" ]]; then
        pending="$(jq -r '.judgement_pending | length' "$dir/grading-request.json")"
    fi

    local judged_json="null" judged_score=0 judged_max=0
    if judgement_valid "$dir/judgement.json" "$dir/grading-request.json"; then
        judged_json="$(jq -c '.' "$dir/judgement.json")"
        judged_score="$(jq -r '[.[].score] | add // 0' "$dir/judgement.json")"
        judged_max="$(jq -r '[.[].max] | add // 0' "$dir/judgement.json")"
        pending=0
    fi

    local max="$denominator"

    # Sum in jq, not Bash arithmetic: a judge may award fractional credit and
    # integer arithmetic would fail silently, dropping the row.
    jq -nc --arg arm "$arm" --argjson run "$run" --arg evidence "$dir" \
        --argjson agent_exit "$agent_exit" --argjson timed_out "$timed_out" \
        --argjson mech_score "$mech_score" --argjson mech_max "$mech_max" \
        --argjson judged "$judged_json" --argjson judged_score "$judged_score" \
        --argjson judged_max "$judged_max" --argjson pending "$pending" \
        --argjson usage "$usage_json" --argjson judge_usage "$judge_usage_json" \
        --argjson max "$max" '
        {
            arm: $arm, run: $run, status: "ok",
            agent_exit: $agent_exit, timed_out: $timed_out,
            mechanical: {score: $mech_score, max: $mech_max},
            judged: (if $judged == null then null
                     else {score: $judged_score, max: $judged_max, items: $judged} end),
            pending: $pending, score: ($mech_score + $judged_score), max: $max,
            percent: (if $max > 0 then (($mech_score + $judged_score) * 100 / $max) else null end),
            usage: $usage, judge_usage: $judge_usage,
            evidence: $evidence
        }'
}

summarise_scenario() {
    local plugin="$1" scenario="$2"
    local runs=() arm run criteria denominator
    criteria="$OUT_DIR/criteria/$plugin/$scenario.json"
    [[ -f "$criteria" ]] ||
        criteria="$PROJECT_ROOT/plugins/$plugin/evals/$scenario/criteria.json"
    denominator="$(jq -r '[.checklist[].max_score] | add // 0' "$criteria" 2>/dev/null || true)"
    [[ "$denominator" =~ ^[0-9]+([.][0-9]+)?$ ]] || denominator=0
    if ! awk -v d="$denominator" 'BEGIN { exit !(d > 0) }'; then
        warn "no positive rubric denominator for $plugin/$scenario; percentages will be null"
        denominator=0
    fi

    for arm in $(arms); do
        for run in $(seq 1 "$RUNS"); do
            runs+=("$(summarise_run "$OUT_DIR/runs/$plugin/$scenario/$arm/run-$run" "$arm" "$run" "$denominator")")
        done
    done

    printf '%s\n' "${runs[@]}" | jq -sc --arg plugin "$plugin" --arg scenario "$scenario" \
        -f "$JQ_DIR/suite-scenario.jq"
}

: >"$OUT_DIR/results.jsonl"
for i in "${!PLAN_PLUGIN[@]}"; do
    summarise_scenario "${PLAN_PLUGIN[$i]}" "${PLAN_SCENARIO[$i]}" >>"$OUT_DIR/results.jsonl"
done

AGGREGATE_FAILURES="$(jq -s '[.[] | select(.status != "ok")] | length' "$OUT_DIR/results.jsonl")"
JUDGED_RUNS="$(jq -s '[.[].runs[] | select(.judged != null)] | length' "$OUT_DIR/results.jsonl")"
PENDING_RUNS="$(jq -s '[.[].runs[] | select(.pending > 0)] | length' "$OUT_DIR/results.jsonl")"
COVERED_SCENARIOS="$(
    jq -s '[.[] | select((.runs | map(.mechanical.max) | add // 0) > 0)] | length' "$OUT_DIR/results.jsonl"
)"
COST_TOTAL_RUNS="$(jq -s '[.[].runs[]] | length' "$OUT_DIR/results.jsonl")"
COST_KNOWN_RUNS="$(
    jq -s '[.[].runs[] | select(
        ((.usage.cost_usd // null) | type) == "number"
        or ((.judge_usage.cost_usd // null) | type) == "number"
    )] | length' "$OUT_DIR/results.jsonl"
)"
DURATION="$(( $(date +%s) - SUITE_START ))"

printf 'plugin\tscenario\tarm\trun\tpending\tevidence\n' >"$OUT_DIR/grading/index.tsv"
jq -r '
    . as $scenario
    | .runs[]
    | select(.pending > 0)
    | [$scenario.plugin, $scenario.scenario, .arm, (.run | tostring),
       (.pending | tostring), .evidence]
    | @tsv
' "$OUT_DIR/results.jsonl" >>"$OUT_DIR/grading/index.tsv"

counts_json="$(jq -nc \
    --argjson planned "$SCENARIO_COUNT" --argjson executed "$EXECUTED" \
    --argjson skipped "$SKIPPED" --argjson runner_failures "$AGGREGATE_FAILURES" \
    --argjson agent_invocations "$((EXECUTED * $(arm_count) * RUNS))" \
    --argjson judged_runs "$JUDGED_RUNS" --argjson pending_runs "$PENDING_RUNS" \
    --argjson covered_scenarios "$COVERED_SCENARIOS" --argjson duration "$DURATION" \
    --argjson cost_known "$COST_KNOWN_RUNS" --argjson cost_total "$COST_TOTAL_RUNS" \
    -f "$JQ_DIR/suite-counts.jq")"

jq -s --argjson config "$config_json" --argjson counts "$counts_json" '
    {config: $config, counts: $counts, scenarios: .}
' "$OUT_DIR/results.jsonl" >"$OUT_DIR/suite.json"

{
    echo "# Eval suite"
    echo
    echo "Agent: \`$AGENT_CMD\`  ·  arms: $ARM  ·  runs per arm: $RUNS  ·  jobs: $JOBS"
    if ((${#UNITS[@]} > 0)); then
        echo "Units: $(printf '%s, ' "${UNITS[@]}" | sed 's/, $//')"
    fi
    echo
    echo "Git: \`$(git_sha)\`$([[ "$(git_dirty)" == "true" ]] && echo ' (dirty tree)')  ·  duration: ${DURATION}s"
    echo
    echo "| Planned | Executed | Skipped | Runner failures | Agent invocations |"
    echo "| --- | --- | --- | --- | --- |"
    echo "| $SCENARIO_COUNT | $EXECUTED | $SKIPPED | $AGGREGATE_FAILURES | $((EXECUTED * $(arm_count) * RUNS)) |"
    echo
    if ((PENDING_RUNS > 0)); then
        echo "**$PENDING_RUNS run(s) still have pending criteria.** A mechanical score is"
        echo "not a verdict; grade the remainder with:"
        echo
        echo '```bash'
        echo "scripts/eval-grade.sh --suite \"$OUT_DIR\" --judge-cmd 'claude -p --permission-mode acceptEdits'"
        echo '```'
        echo
        echo "The index is at \`$OUT_DIR/grading/index.tsv\`."
        echo
    fi

    echo "## By plugin"
    echo
    echo "| Plugin | Scenarios | With | Without | Δ |"
    echo "| --- | --- | --- | --- | --- |"
    jq -sr '
        def mean_present: map(select(. != null)) | if length == 0 then null else (add / length) end;
        def pct: if . == null then "—" else (((. * 10 | round) / 10) | tostring) + "%" end;
        def pts: if . == null then "—"
                 else (((. * 10 | round) / 10) | if . > 0 then "+" + tostring else tostring end) + " pts"
                 end;
        group_by(.plugin)[]
        | . as $rows
        | ([$rows[] | .arms.with.percent] | mean_present) as $with
        | ([$rows[] | .arms.without.percent] | mean_present) as $without
        | [$rows[0].plugin, (length | tostring), ($with | pct), ($without | pct),
           (if $with == null or $without == null then null else ($with - $without) end | pts)]
        | "| " + join(" | ") + " |"
    ' "$OUT_DIR/results.jsonl"

    echo
    echo "## By scenario"
    echo
    echo "| Plugin | Scenario | With | Without | Δ | Pending |"
    echo "| --- | --- | --- | --- | --- | --- |"
    jq -r '
        def pct: if . == null then "—" else (((. * 10 | round) / 10) | tostring) + "%" end;
        def pts: if . == null then "—"
                 else (((. * 10 | round) / 10) | if . > 0 then "+" + tostring else tostring end) + " pts"
                 end;
        [ .plugin, .scenario, (.arms.with.percent | pct), (.arms.without.percent | pct),
          (.delta | pts), ((.pending | tostring) + " pending") ]
        | "| " + join(" | ") + " |"
    ' "$OUT_DIR/results.jsonl"

    echo
    echo "## Cost"
    echo
    if ((COST_KNOWN_RUNS == 0)); then
        echo "Cost: not reported by the agent or judge command (0/$COST_TOTAL_RUNS runs); add --format json, --output-format json, or --usage-cmd."
    else
        echo "| Plugin | Scenario | Agent with | Agent without | Judge with | Judge without | Total with | Total without | Coverage |"
        echo "| --- | --- | --- | --- | --- | --- | --- | --- | --- |"
        jq -r -f "$JQ_DIR/suite-summary-cost.jq" "$OUT_DIR/results.jsonl"
        echo
        echo "Cost/tokens are sums over runs that reported them; unknown is never counted as zero."
    fi

    echo
    echo "Mechanical coverage: $COVERED_SCENARIOS of $SCENARIO_COUNT scenario(s) ship a checks.sh."
    echo "Rows with no percentage have nothing scoreable yet; see the grading index."
} >"$OUT_DIR/summary.md"

cat "$OUT_DIR/summary.md"

# --- exit status and selftest -------------------------------------------------

strict_failed=0
if [[ "$STRICT" == "1" ]]; then
    if ((AGGREGATE_FAILURES > 0)); then
        warn "strict: $AGGREGATE_FAILURES scenario(s) had runner failures"
        strict_failed=1
    fi
    if ((PENDING_RUNS > 0)); then
        warn "strict: $PENDING_RUNS run(s) still have pending criteria"
        strict_failed=1
    fi
    discriminating="$(jq -s -r '
        [.[] | select(.delta != null and .delta <= 0)] | length
    ' "$OUT_DIR/results.jsonl")"
    if ((discriminating > 0)); then
        warn "strict: $discriminating scenario(s) scored no better with the unit than without"
        strict_failed=1
    fi
fi

if [[ "$SELFTEST" == "1" ]]; then
    echo
    echo "🧪 Selftest"
    passed=0
    failed=0
    check() {
        if [[ "$1" == "1" ]]; then
            echo "  ✓ $2"
            passed=$((passed + 1))
        else
            echo "  ✗ $2"
            failed=$((failed + 1))
        fi
    }

    check "$([[ -f "$OUT_DIR/summary.md" && -f "$OUT_DIR/results.jsonl" && -f "$OUT_DIR/suite.json" ]] && echo 1 || echo 0)" \
        "summary, results and suite report written"
    check "$([[ "$(jq -r '.counts.executed' "$OUT_DIR/suite.json")" == "3" ]] && echo 1 || echo 0)" \
        "all three representative scenarios executed"
    check "$([[ "$(wc -l <"$OUT_DIR/results.jsonl" | tr -d ' ')" == "3" ]] && echo 1 || echo 0)" \
        "one aggregate row per scenario"
    check "$([[ "$(jq -r '.counts.runner_failures' "$OUT_DIR/suite.json")" == "0" ]] && echo 1 || echo 0)" \
        "no runner failures"

    command_dir=""
    for scenario_dir in "$PROJECT_ROOT"/plugins/*/evals/*/; do
        plugin="$(basename "$(dirname "$(dirname "$scenario_dir")")")"
        pair="$plugin/$(basename "$scenario_dir")"
        if selftest_selected "$pair" && [[ -d "$PROJECT_ROOT/plugins/$plugin/commands" ]]; then
            command_dir="$scenario_dir"
            break
        fi
    done
    if [[ -n "$command_dir" ]]; then
        plugin="$(basename "$(dirname "$(dirname "$command_dir")")")"
        scenario="$(basename "$command_dir")"
        command_name="$(basename "$(find "$PROJECT_ROOT/plugins/$plugin/commands" -name '*.md' -print -quit)")"
        tar -tf "$OUT_DIR/runs/$plugin/$scenario/with/run-1/snapshot.tar" 2>/dev/null |
            grep -qE "^(\./)?\.claude/commands/$plugin/$command_name$" &&
            with_command=1 || with_command=0
        tar -tf "$OUT_DIR/runs/$plugin/$scenario/without/run-1/snapshot.tar" 2>/dev/null |
            grep -qE '^(\./)?\.claude/commands/' &&
            without_command=1 || without_command=0
        check "$([[ "$with_command" == "1" && "$without_command" == "0" ]] && echo 1 || echo 0)" \
            "the with-arm installs commands, the without-arm does not"
    else
        check 1 "no command-only scenario selected"
    fi

    echo
    echo "📊 Selftest: $((passed + failed)), Passed: $passed, Failed: $failed"
    rm -rf "$OUT_DIR"
    [[ "$failed" -eq 0 ]] || exit 1
    exit 0
fi

if ((AGGREGATE_FAILURES > 0)); then
    exit 1
fi
[[ "$strict_failed" == "0" ]] || exit 1
exit 0
