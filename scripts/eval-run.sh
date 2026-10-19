#!/usr/bin/env bash

# Local eval runner
#
# Stages a scenario from plugins/<plugin>/evals/<scenario>/ in a temporary
# workspace, runs an agent against its task.md - once with the plugin's skills
# installed and once without - records what changed, and scores the parts of
# the rubric a script can score (a scenario's optional checks.sh).
#
# The workspace is not a security boundary. The agent runs with the invoking
# user's privileges, so it can read and write absolute host paths and reach
# the network. Use a container or VM when a run must be isolated.
#
# A scenario with "mode": "functional" exercises the current plugin source
# directly and runs only the with arm. Hook scenarios use this because they
# invoke the hook themselves; they report an absolute score, not a delta.
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
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
EVAL_USAGE="$SCRIPT_DIR/eval-usage.sh"
PLUGIN=""
SCENARIO=""
AGENT_CMD=""
USAGE_CMD=""
ARM="both"
RUNS=1
TIMEOUT=900
OUT_DIR="$PROJECT_ROOT/results/eval-run"
SKILL_DIR=".claude/skills"
KEEP=0
SELFTEST=0
AUTH=1
REAL_HOME="${HOME:-}"
UNITS=()
CURRENT_SANDBOX=""
CURRENT_HOME=""
USAGE_UNKNOWN=0
AUTH_PATHS=(
    .claude.json
    .claude/.credentials.json
    .claude/settings.json
    .codex/auth.json
    .codex/config.toml
    .config/gh/hosts.yml
    .local/share/opencode/auth.json
)

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
  --agent-cmd CMD     command that reads task.md on stdin, run in the
                      temporary workspace (e.g. 'claude -p', 'codex exec -')
  --arm ARM           both (default), with, or without
  --runs N            runs per arm (default 1)
  --usage-cmd CMD     optional post-run hook that normalizes usage/tokens
                      (see scripts/eval-usage.sh)
  --unit UNIT         unit the with-arm installs (repeatable):
                      skills/<name>, commands/<name>, or hooks/<name>.
                      Without it the with-arm installs every skill and
                      command the plugin ships.
  --timeout SECS      per-run deadline (default 900)
  --skill-dir DIR     where the 'with' arm installs the plugin's skills in the
                      workspace, relative to it (default .claude/skills).
                      The plugin's commands go to the sibling commands/ dir.
  --out DIR           results directory (default results/eval-run)
  --keep              keep workspaces; evidence bundles are always written
  --no-auth           do not copy the agent CLI's login state into the
                      workspace (also skips the macOS Keychain symlink)
  --auth-path PATH    extra file under your home to copy in for the agent
                      (repeatable; e.g. .local/share/opencode/auth.json)
  --selftest          run the plumbing against a no-op agent; no model needed
EOF
}

die() {
    echo "eval-run: $*" >&2
    exit 1
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --plugin | --scenario | --agent-cmd | --usage-cmd | --arm | --runs | --timeout | --skill-dir | --out | --auth-path | --unit)
            if [[ $# -lt 2 || -z "$2" ]]; then
                echo "eval-run: $1 needs a value" >&2
                usage >&2
                exit 2
            fi
            case "$1" in
                --plugin) PLUGIN="$2" ;;
                --scenario) SCENARIO="$2" ;;
                --agent-cmd) AGENT_CMD="$2" ;;
                --usage-cmd) USAGE_CMD="$2" ;;
                --arm) ARM="$2" ;;
                --runs) RUNS="$2" ;;
                --timeout) TIMEOUT="$2" ;;
                --skill-dir) SKILL_DIR="$2" ;;
                --out) OUT_DIR="$2" ;;
                --auth-path) AUTH_PATHS+=("$2") ;;
                --unit) UNITS+=("$2") ;;
                *) ;;
            esac
            shift 2
            ;;
        --keep)
            KEEP=1
            shift
            ;;
        --no-auth)
            AUTH=0
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
[[ "$SKILL_DIR" != /* ]] || die "--skill-dir must be relative to the workspace"
for auth_path in "${AUTH_PATHS[@]}"; do
    [[ "$auth_path" != /* && "$auth_path" != *..* ]] ||
        die "--auth-path must be a relative path under \$HOME: $auth_path"
done

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
        # Run the sleeper in the background so the TERM trap can kill it;
        # otherwise a timed-out watcher leaves an orphan sleep behind on
        # systems without GNU timeout.
        sleeper=""
        trap 'kill -TERM "$sleeper" 2>/dev/null' TERM INT
        sleep "$secs" &
        sleeper=$!
        wait "$sleeper"
        kill -TERM "$pid" 2>/dev/null
    ) >/dev/null 2>&1 &
    watcher=$!
    wait "$pid"
    local rc=$?
    kill -TERM "$watcher" 2>/dev/null
    return "$rc"
}

# Give the agent's CLI its login state without letting it write to the real
# home. Credential files are copied into the workspace and excluded from the
# evidence bundle. On macOS the Keychains directory is exposed as a symlink
# because the CLI looks it up under $HOME; a symlink grants the agent full
# read/write access to the user's keychains. That is a deliberate tradeoff for
# authenticated runs - pass --no-auth to skip credential copying.
prepare_agent_home() {
    local home="$1"
    [[ "$AUTH" == "1" && -n "$REAL_HOME" ]] || return 0

    local rel src dest
    for rel in "${AUTH_PATHS[@]}"; do
        src="$REAL_HOME/$rel"
        [[ -f "$src" ]] || continue
        dest="$home/$rel"
        if ! mkdir -p "$(dirname "$dest")"; then
            die "cannot create the agent config directory for $src"
        fi
        # A failed copy would silently produce an unauthenticated run that
        # looks exactly like a valid one, so it is fatal.
        if ! cp -p "$src" "$dest"; then
            die "cannot copy agent credentials from $src; rerun with --no-auth to skip credential copying, or pass --auth-path for the file your agent CLI actually uses"
        fi
    done

    if [[ -d "$REAL_HOME/Library/Keychains" ]]; then
        mkdir -p "$home/Library"
        ln -s "$REAL_HOME/Library/Keychains" "$home/Library/Keychains"
    fi
}

# A kept sandbox must not retain the credentials copied in for the run.
clear_agent_home() {
    local home="$1"
    local rel
    for rel in "${AUTH_PATHS[@]}"; do
        rm -f "$home/$rel"
    done
    [[ -L "$home/Library/Keychains" ]] && rm -f "$home/Library/Keychains"
}

# An interrupt during the agent must not leave copied credentials (or, unless
# --keep was requested, the whole sandbox) in the temp directory. The helper is
# idempotent: run_once calls it on the normal path, and the EXIT trap calls it
# again with nothing left to do.
cleanup_sandbox() {
    local sandbox="${CURRENT_SANDBOX:-}"
    [[ -n "$sandbox" ]] || return 0
    if [[ -n "${CURRENT_HOME:-}" ]]; then
        clear_agent_home "$CURRENT_HOME" 2>/dev/null || true
    fi
    if [[ "$KEEP" == "0" ]]; then
        rm -rf "$sandbox"
    fi
    CURRENT_SANDBOX=""
    CURRENT_HOME=""
}

on_interrupt() {
    cleanup_sandbox
    exit "$1"
}

trap cleanup_sandbox EXIT
trap 'on_interrupt 130' INT
trap 'on_interrupt 143' TERM

# Normalize the just-finished agent log. Unknown usage is recorded, never
# guessed: the evidence and the summary both say known:false.
record_usage() {
    local dir="$1" log="$2" sandbox="$3" arm="$4" run="$5" exit_code="$6"
    local usage_file="$dir/usage.json"
    local args=(--log "$log" --output "$usage_file" --run-dir "$dir"
        --arm "$arm" --run "$run" --agent-exit "$exit_code")
    [[ -n "$sandbox" ]] && args+=(--sandbox "$sandbox")
    [[ -n "$USAGE_CMD" ]] && args+=(--cmd "$USAGE_CMD")

    if ! "$EVAL_USAGE" "${args[@]}" >/dev/null 2>"$dir/usage.log"; then
        printf '{"known":false,"source":"none"}\n' >"$usage_file"
    fi
    if ! jq -e 'type == "object" and (.known | type) == "boolean"'         "$usage_file" >/dev/null 2>&1; then
        printf '{"known":false,"source":"none"}\n' >"$usage_file"
    fi
}

usage_line() {
    jq -r '
        if .known then
            "usage: " +
            (if (.cost_usd | type) == "number" then "$" + (.cost_usd | tostring)
             else "cost unknown" end) +
            ", " +
            (if (.total_tokens | type) == "number" then (.total_tokens | tostring) + " tokens"
             else "tokens unknown" end)
        else empty end
    ' "$1" 2>/dev/null
}

usage_hint() {
    local hint=""
    if [[ "$AGENT_CMD" == *opencode* && "$AGENT_CMD" == *run* &&
        "$AGENT_CMD" != *"--format json"* ]]; then
        hint="opencode run without --format json"
    elif [[ "$AGENT_CMD" == *claude* &&
        ( "$AGENT_CMD" =~ (^|[[:space:]])-p([[:space:]]|$) ||
          "$AGENT_CMD" =~ (^|[[:space:]])--print([[:space:]]|$) ) &&
        "$AGENT_CMD" != *"--output-format json"* ]]; then
        hint="claude -p without --output-format json"
    fi
    [[ -n "$hint" ]] || return 0
    echo "   hint: agent usage was not reported ($hint); add the structured-output flag or --usage-cmd"
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
    OUT_DIR="$(mktemp -d "${TMPDIR:-/tmp}/eval-run-selftest-XXXXXX")" ||
        die "cannot create a selftest directory"
    KEEP=0
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

# A functional scenario has no with/without comparison: it exercises the
# current plugin source directly and reports an absolute score. A comparison
# scenario runs both arms.
MODE="$(jq -r '.mode // "comparison"' "$SCENARIO_JSON")"
case "$MODE" in
    comparison | functional) ;;
    *) die "scenario mode must be comparison or functional: $MODE" ;;
esac
if [[ "$MODE" == "functional" ]]; then
    case "$ARM" in
        both) ARM=with ;;
        without) die "functional scenarios have no without arm; use --arm with or --arm both" ;;
        *) ;;
    esac
fi

# A unit is what the with-arm installs. It must name something the plugin
# actually ships, so a typo cannot silently turn an experiment back into an
# install-everything run.
unit_exists() {
    local unit="$1" kind name
    kind="${unit%%/*}"
    name="${unit#*/}"
    [[ "$unit" == */* && -n "$name" ]] || return 1
    case "$kind" in
        skills) [[ -d "$PLUGIN_DIR/skills/$name" ]] ;;
        commands) [[ -f "$PLUGIN_DIR/commands/$name.md" ]] ;;
        hooks) [[ -f "$PLUGIN_DIR/hooks/$name.sh" || -f "$PLUGIN_DIR/hooks/$name" ]] ;;
        *) return 1 ;;
    esac
}

for unit in ${UNITS[@]+"${UNITS[@]}"}; do
    [[ "$unit" =~ ^(skills|commands|hooks)/[A-Za-z0-9._-]+$ ]] ||
        die "invalid --unit '$unit' (expected skills/<name>, commands/<name> or hooks/<name>)"
    unit_exists "$unit" ||
        die "unknown unit '$unit' for plugin '$PLUGIN'"
done

SEEN_UNITS=()
for unit in ${UNITS[@]+"${UNITS[@]}"}; do
    already=0
    for seen in ${SEEN_UNITS[@]+"${SEEN_UNITS[@]}"}; do
        [[ "$seen" == "$unit" ]] && already=1
    done
    ((already)) || SEEN_UNITS+=("$unit")
done
UNITS=(${SEEN_UNITS[@]+"${SEEN_UNITS[@]}"})

# Hook scenarios invoke the hook themselves, so the with-arm stages the
# current plugin source for the fixture to copy. A declared hook unit is what
# identifies the scenario, whether or not --unit was passed.
HOOK_SCENARIO=0
while IFS= read -r declared; do
    [[ -n "$declared" ]] || continue
    [[ "$declared" == hooks/* ]] || continue
    unit_exists "$declared" ||
        die "scenario declares unknown hook unit '$declared' for plugin '$PLUGIN'"
    HOOK_SCENARIO=1
done < <(jq -r '(.units // [.unit] // [])[] | select(. != null and . != "")' "$SCENARIO_JSON")

echo "🧪 $PLUGIN/$SCENARIO"
echo "   mode: $MODE   arms: $ARM   runs per arm: $RUNS   agent: $AGENT_CMD"
if ((${#UNITS[@]} > 0)); then
    echo "   units: ${UNITS[*]}"
fi

RESULTS=()

# Stage the hook source the fixture will copy into its workspace. The with arm
# gets the current plugin source, so an edit to a hook is exercised rather
# than a stale fixture copy. The without arm of a comparison-mode hook
# scenario gets a disabled control with the same filenames.
stage_hook_source() {
    local sandbox="$1" arm="$2"
    local dest="$sandbox/.eval-plugin"
    local hook name
    rm -rf "$dest"
    mkdir -p "$dest/hooks"
    if [[ "$arm" == "with" ]]; then
        if [[ -d "$PLUGIN_DIR/hooks" ]]; then
            cp -R "$PLUGIN_DIR/hooks/." "$dest/hooks/"
        fi
    else
        for hook in "$PLUGIN_DIR"/hooks/*; do
            [[ -e "$hook" ]] || continue
            name="$(basename "$hook")"
            cat >"$dest/hooks/$name" <<'EOF'
#!/usr/bin/env bash
# Disabled control hook: accepts every call without scanning or deciding.
cat >/dev/null
exit 0
EOF
            chmod +x "$dest/hooks/$name"
        done
    fi
    if [[ -f "$PLUGIN_DIR/security-patterns.conf" ]]; then
        cp "$PLUGIN_DIR/security-patterns.conf" "$dest/security-patterns.conf"
    fi
}

run_once() {
    local arm="$1" run="$2"
    local sandbox rc out
    sandbox="$(mktemp -d "${TMPDIR:-/tmp}/eval-run-$SCENARIO-XXXXXX")" ||
        die "cannot create a workspace for $arm/run-$run"
    CURRENT_SANDBOX="$sandbox"
    out="$OUT_DIR/$PLUGIN/$SCENARIO/$arm/run-$run"
    # A run directory is recreated, not reused: anything left from an earlier
    # execution (judgement, usage, judge timing) describes evidence that no
    # longer exists.
    if ! rm -rf "$out" || ! mkdir -p "$out" "$sandbox/home"; then
        die "cannot create result directory: $out"
    fi
    CURRENT_HOME="$sandbox/home"

    # Stage the fixture exactly as scenario.json declares it.
    local include
    while IFS= read -r include; do
        [[ -z "$include" ]] && continue
        cp -R "$SCENARIO_DIR/$include" "$sandbox/" || die "cannot stage $include"
    done <<<"$(jq -r '.include[]?' "$SCENARIO_JSON")"

    # Hook fixtures copy their source from EVAL_HOOK_SOURCE. The with arm gets
    # the current plugin source; a comparison-mode without arm gets a disabled
    # control, so the delta measures the hook rather than two fixture copies.
    local hook_source=""
    if [[ "$HOOK_SCENARIO" == "1" ]]; then
        hook_source="$sandbox/.eval-plugin"
        stage_hook_source "$sandbox" "$arm"
    fi

    # HOME is redirected for every step, so a fixture or an agent that writes
    # to ~ writes into the workspace - never into the contributor's home. The
    # staged script is invoked by absolute path: several fixtures cd to their
    # own directory, and a relative $0 would otherwise be resolved twice.
    # shellcheck disable=SC2030,SC2031  # HOME and hook source are subshell-local
    if ! (
        cd "$sandbox" || exit 1
        export HOME="$sandbox/home"
        [[ -n "$hook_source" ]] && export EVAL_HOOK_SOURCE="$hook_source"
        bash "$sandbox/inputs/setup.sh"
    ) >"$out/setup.log" 2>&1; then
        die "setup failed for $arm/run-$run - see $out/setup.log"
    fi

    snapshot_to "$sandbox" "$out/before.txt"

    # The 'with' arm installs the selected plugin units into the sandbox, the
    # way an agent that has the plugin loaded would see them. Without --unit
    # it keeps the historical install-everything behaviour.
    if [[ "$arm" == "with" ]]; then
        local local_unit kind name command_dir
        local_unit=""
        if ((${#UNITS[@]} > 0)); then
            for local_unit in "${UNITS[@]}"; do
                kind="${local_unit%%/*}" name="${local_unit#*/}"
                case "$kind" in
                    skills)
                        mkdir -p "$sandbox/$SKILL_DIR/$name"
                        cp -R "$PLUGIN_DIR/skills/$name/." "$sandbox/$SKILL_DIR/$name/"
                        ;;
                    commands)
                        command_dir="$(dirname "$SKILL_DIR")/commands/$PLUGIN"
                        mkdir -p "$sandbox/$command_dir"
                        cp "$PLUGIN_DIR/commands/$name.md" "$sandbox/$command_dir/"
                        ;;
                    hooks)
                        # The scenario invokes the hook itself; the current
                        # source is staged separately and copied by the
                        # fixture. The unit records what is under test.
                        ;;
                    *) ;;
                esac
            done
        else
            mkdir -p "$sandbox/$SKILL_DIR"
            local skill
            for skill in "$PLUGIN_DIR"/skills/*/; do
                name="$(basename "$skill")"
                mkdir -p "$sandbox/$SKILL_DIR/$name"
                # "$skill." copies the contents, which behaves the same on BSD
                # and GNU cp (a trailing slash alone does not).
                cp -R "$skill." "$sandbox/$SKILL_DIR/$name/"
            done

            # Commands are a second unit type. A plugin exposes them
            # namespaced, so commands/<name>.md becomes
            # commands/<plugin>/<name>.md and answers /<plugin>:<name>.
            if [[ -d "$PLUGIN_DIR/commands" ]]; then
                local command
                command_dir="$(dirname "$SKILL_DIR")/commands/$PLUGIN"
                mkdir -p "$sandbox/$command_dir"
                for command in "$PLUGIN_DIR"/commands/*.md; do
                    [[ -e "$command" ]] || continue
                    cp "$command" "$sandbox/$command_dir/"
                done
            fi
        fi
    fi

    # Credentials are only present for the agent: fixtures and checks run
    # without them.
    prepare_agent_home "$sandbox/home"

    local started ended duration
    started="$(date +%s)"
    (
        cd "$sandbox" || exit 1
        HOME="$sandbox/home" run_with_deadline "$TIMEOUT" bash -c "$AGENT_CMD"
    ) <"$TASK" >"$out/agent.log" 2>&1
    rc=$?
    ended="$(date +%s)"
    duration=$((ended - started))
    clear_agent_home "$sandbox/home"
    CURRENT_HOME=""

    record_usage "$out" "$out/agent.log" "$sandbox" "$arm" "$run" "$rc"
    local usage_file="$out/usage.json"
    if [[ "$(jq -r '.known' "$usage_file" 2>/dev/null)" != "true" ]]; then
        USAGE_UNKNOWN=1
    fi

    local timed_out=false
    [[ "$rc" == "124" || "$rc" == "143" ]] && timed_out=true

    snapshot_to "$sandbox" "$out/after.txt"
    # Grading needs the evidence bundle, so it is always captured; --keep only
    # controls whether the sandbox directory itself survives.
    (
        cd "$sandbox" || exit 1
        tar --exclude ./home -cf "$out/snapshot.tar" .
    ) || die "cannot write the evidence bundle $out/snapshot.tar"

    # Mechanical checks: the scenario's own checks.sh, validated against the
    # rubric it claims to score. A checks.sh that disagrees with criteria.json
    # is ignored rather than trusted.
    local covered="[]" mechanical="null" raw="$out/checks.raw.json"
    if [[ -x "$SCENARIO_DIR/checks.sh" ]]; then
        if (
            cd "$sandbox" || exit 1
            # shellcheck disable=SC2030,SC2031  # HOME and hook source are deliberately subshell-local
            export HOME="$sandbox/home" SANDBOX="$sandbox" SCENARIO_DIR CRITERIA
            # shellcheck disable=SC2031  # deliberately subshell-local
            [[ -n "$hook_source" ]] && export EVAL_HOOK_SOURCE="$hook_source"
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

    local usage_json
    usage_json="$(jq -c '.' "$usage_file" 2>/dev/null)"
    [[ -n "$usage_json" ]] || usage_json='{"known":false,"source":"none"}'

    jq -n --arg plugin "$PLUGIN" --arg scenario "$SCENARIO" --arg arm "$arm" --arg mode "$MODE" --argjson run "$run" \
        --argjson exit "$rc" --argjson score "$score" --argjson max "$max" \
        --argjson timed_out "$timed_out" --argjson duration "$duration" \
        --argjson mechanical "${mechanical:-null}" --argjson usage "$usage_json" --arg evidence "$out" '
        {
            plugin: $plugin, scenario: $scenario, arm: $arm, mode: $mode, run: $run,
            agent_exit: $exit,
            timed_out: $timed_out,
            duration_seconds: $duration,
            mechanical: {score: $score, max: $max, items: $mechanical},
            usage: $usage,
            evidence: $evidence
        }' >"$out/result.json"

    cleanup_sandbox
    if [[ "$timed_out" == "true" ]]; then
        echo "   $arm run $run: timed out after ${duration}s, mechanical $score/$max"
    else
        echo "   $arm run $run: agent exit $rc, mechanical $score/$max"
    fi
    local usage_summary
    usage_summary="$(usage_line "$usage_file")"
    if [[ -n "$usage_summary" ]]; then
        echo "   $arm run $run: $usage_summary"
    else
        echo "   $arm run $run: usage: not reported by the agent command"
    fi
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
    echo "Agent: \`$AGENT_CMD\`  ·  mode: $MODE  ·  runs per arm: $RUNS"
    echo
    if [[ "$MODE" == "functional" ]]; then
        echo "Functional scenario: the with arm exercises the current plugin source;"
        echo "there is no without baseline and no contribution delta."
        echo
    fi
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

if [[ "$USAGE_UNKNOWN" == "1" ]]; then
    usage_hint
fi

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
    if [[ "$failed" -eq 0 ]]; then
        rm -rf "$OUT_DIR"
        exit 0
    fi
    exit 1
fi

echo
echo "Evidence: $summary_dir"
