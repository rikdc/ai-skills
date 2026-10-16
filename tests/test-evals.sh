#!/usr/bin/env bash

# Eval and Skill Quality Gate
#
# Deterministic checks over the eval scenarios and skills in this repository:
# no model, no network, no credentials. Anything that needs judgement - does a
# scenario discriminate, is a rubric fair to the unit it tests, is a recording
# shim faithful to the real CLI, is a description well written - is deliberately
# out of scope. Those checks need an LLM and belong on a contributor's machine,
# not in public CI.
#
# What it checks, per unit:
#   scenario  layout, naming, rubric arithmetic, fixture hygiene, dead fixtures
#   manifest  the tessl manifest matches the Claude manifest and the workspace
#   skill     frontmatter, spec limits, allowed-tools breadth, dead pointers
#   repo      evals stay at the plugin root, never at the repo root
#
# Usage:
#   tests/test-evals.sh                     check the repository
#   tests/test-evals.sh --root DIR          check a copy of the repository
#   tests/test-evals.sh --changes GIT-REF   list the skills and scenarios a ref touched
#
# Deliberately no -e: report every failure in one run, not just the first.
set -uo pipefail
shopt -s nullglob

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ROOT="$PROJECT_ROOT"
CHANGES_REF=""

usage() {
    cat <<'EOF'
Deterministic eval and skill quality gate.

Usage:
  tests/test-evals.sh                     check the repository
  tests/test-evals.sh --root DIR          check a copy of the repository
  tests/test-evals.sh --changes GIT-REF   list the skills and scenarios a ref touched
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --root | --changes)
            if [[ $# -lt 2 || -z "$2" ]]; then
                echo "test-evals: $1 needs a value" >&2
                usage >&2
                exit 2
            fi
            if [[ "$1" == "--root" ]]; then
                ROOT="$2"
            else
                CHANGES_REF="$2"
            fi
            shift 2
            ;;
        -h | --help)
            usage
            exit 0
            ;;
        *)
            echo "test-evals: unknown argument: $1" >&2
            usage >&2
            exit 2
            ;;
    esac
done

cd "$ROOT" || {
    echo "test-evals: cannot enter $ROOT" >&2
    exit 2
}
ROOT="$(pwd)"

# --- --changes: which skills and scenarios a ref touched ---------------------
# Informational only: a reviewer, or an agent running on a contributor's
# machine, uses this to decide what needs judging or re-running.

if [[ -n "$CHANGES_REF" ]]; then
    if ! git rev-parse --git-dir >/dev/null 2>&1; then
        echo "test-evals: --changes needs a git checkout" >&2
        exit 2
    fi
    if ! changed="$(git diff --name-only "$CHANGES_REF"...HEAD -- plugins 2>/dev/null)"; then
        echo "test-evals: cannot diff against '$CHANGES_REF' (fetch it first)" >&2
        exit 2
    fi

    list_matches() {
        local label="$1" pattern="$2" matches=""
        matches="$(printf '%s\n' "$changed" | grep -oE "$pattern" | sort -u)"
        if [[ -z "$matches" ]]; then
            echo "$label: (none)"
        else
            echo "$label:"
            while IFS= read -r match; do
                echo "  $match"
            done <<<"$matches"
        fi
    }

    list_matches "Changed skills" '^plugins/[^/]+/skills/[^/]+'
    list_matches "Changed scenarios" '^plugins/[^/]+/evals/[^/]+'
    exit 0
fi

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

# Reports one unit as a single test: it passes when it produced no findings.
finish() {
    local label="$1" findings="$2"
    if [[ -z "$findings" ]]; then
        pass "$label"
        return
    fi
    while IFS= read -r line; do
        [[ -n "$line" ]] && fail "$line"
    done <<<"$findings"
}

echo "🧪 Eval and Skill Quality Gate"
echo "=============================="

WORKSPACE="$(jq -r '.name // ""' tessl.json 2>/dev/null | cut -d/ -f1)"
if [[ -z "$WORKSPACE" ]]; then
    fail "tessl.json: no workspace name (expected {\"name\": \"<workspace>/<project>\"})"
else
    pass "tessl.json workspace: $WORKSPACE"
fi

# --- repo-wide placement ------------------------------------------------------

echo
echo "🚀 Eval placement..."

placement_findings=""
if [[ -e evals ]]; then
    placement_findings+="evals/ exists at the repository root - scenarios belong inside their plugin"$'\n'
fi
for stray in plugins/*/skills/*/evals/; do
    placement_findings+="$stray: scenarios belong at the plugin root (plugins/<plugin>/evals/), not under a skill"$'\n'
done
finish "scenarios live at the plugin root" "$placement_findings"

# --- eval scenarios -----------------------------------------------------------

echo
echo "🚀 Eval scenarios and fixtures..."

for scenario_dir in plugins/*/evals/*/; do
    scenario="${scenario_dir%/}"
    name="$(basename "$scenario")"
    findings=""

    # Naming: tessl's downloader owns canonical scenario-N names and overwrites
    # them, so a hand-written scenario must never use one.
    if [[ "$name" =~ ^scenario-[0-9]+$ ]]; then
        findings+="$scenario: canonical tessl name - use a descriptive name, a re-download would overwrite hand edits"$'\n'
    fi

    for required in task.md criteria.json scenario.json inputs; do
        if [[ ! -e "$scenario/$required" ]]; then
            findings+="$scenario: missing $required"$'\n'
        fi
    done
    if [[ -f "$scenario/task.md" && ! -s "$scenario/task.md" ]]; then
        findings+="$scenario/task.md: empty"$'\n'
    fi

    # scenario.json: the include list is what the sandbox receives.
    if [[ -f "$scenario/scenario.json" ]]; then
        if ! jq -e . "$scenario/scenario.json" >/dev/null 2>&1; then
            findings+="$scenario/scenario.json: invalid JSON"$'\n'
        elif [[ "$(jq -r '.include | type' "$scenario/scenario.json" 2>/dev/null)" != "array" ]]; then
            findings+="$scenario/scenario.json: 'include' must be an array"$'\n'
        else
            while IFS= read -r include; do
                [[ -z "$include" ]] && continue
                [[ -e "$scenario/$include" ]] ||
                    findings+="$scenario/scenario.json: include path does not exist: $include"$'\n'
            done <<<"$(jq -r '.include[]?' "$scenario/scenario.json")"
        fi
    fi

    # criteria.json: weighted checklist, 8-13 items, arithmetic summing to 100.
    if [[ -f "$scenario/criteria.json" ]]; then
        if ! jq -e . "$scenario/criteria.json" >/dev/null 2>&1; then
            findings+="$scenario/criteria.json: invalid JSON"$'\n'
        else
            [[ -n "$(jq -r '.context // ""' "$scenario/criteria.json")" ]] ||
                findings+="$scenario/criteria.json: empty context"$'\n'
            [[ "$(jq -r '.type // ""' "$scenario/criteria.json")" == "weighted_checklist" ]] ||
                findings+="$scenario/criteria.json: type must be weighted_checklist"$'\n'

            items="$(jq '.checklist | length' "$scenario/criteria.json")"
            if ((items < 8 || items > 13)); then
                findings+="$scenario/criteria.json: $items checklist items (expected 8-13)"$'\n'
            fi

            while IFS= read -r bad; do
                [[ -z "$bad" ]] && continue
                findings+="$scenario/criteria.json: item needs a name, a description and a positive max_score: $bad"$'\n'
            done <<<"$(jq -r '.checklist[] | select(
                ((.name // "") == "") or ((.description // "") == "") or
                ((.max_score // 0) | type) != "number" or ((.max_score // 0) <= 0)
            ) | .name // "(unnamed)"' "$scenario/criteria.json")"

            duplicates="$(jq -r '[.checklist[].name] | group_by(.) | map(select(length > 1)) | flatten | join(", ")' "$scenario/criteria.json")"
            [[ -n "$duplicates" ]] && findings+="$scenario/criteria.json: duplicate item names: $duplicates"$'\n'

            sum="$(jq '[.checklist[].max_score] | add // 0' "$scenario/criteria.json")"
            [[ "$sum" == "100" ]] || findings+="$scenario/criteria.json: max_score sums to $sum (expected 100)"$'\n'
        fi
    fi

    # Fixtures: one entry point named setup.sh, self-contained and safe to run
    # locally, leaving the graded repository state clean.
    setup="$scenario/inputs/setup.sh"
    if [[ ! -f "$setup" ]]; then
        findings+="$scenario/inputs: no setup.sh"$'\n'
    else
        [[ -x "$setup" ]] || findings+="$setup: not executable"$'\n'
        [[ "$(head -1 "$setup")" == "#!/usr/bin/env bash" ]] ||
            findings+="$setup: shebang must be #!/usr/bin/env bash"$'\n'
        grep -q 'set -euo pipefail' "$setup" ||
            findings+="$setup: missing set -euo pipefail"$'\n'

        # shellcheck disable=SC2016  # the pattern is a regex, not an expansion
        if grep -qE '\$HOME|~/' "$setup"; then
            findings+="$setup: writes outside the workspace (\$HOME) - authors run setups locally"$'\n'
        fi

        # Every git init must be quiet. A script that initialises the working
        # directory must keep its own files out of the graded status; one that
        # cd's into a subdirectory first (a nested fixture repo) has nothing to
        # exclude. A cd back to the script's own location, or to . or .., is
        # still the working directory.
        init_report="$(awk '
            /^[[:space:]]*#/ { next }
            match($0, /(^|[;&|(])[[:space:]]*cd[[:space:]]+[^;&|)]+/) {
                target = substr($0, RSTART, RLENGTH)
                sub(/^.*cd[[:space:]]+/, "", target)
                sub(/[[:space:]]+$/, "", target)
                nested = (target !~ /dirname|\$0|BASH_SOURCE/ && target !~ /^"?\.\.?\/?"?$/)
            }
            /git init/ {
                if ($0 !~ /git init[^;&|#]*[[:space:]](-q|--quiet)([[:space:];&|)]|$)/) print "loud " NR
                if (!nested) top = 1
            }
            END { if (top) print "top" }
        ' "$setup")"
        while IFS= read -r loud; do
            [[ -z "$loud" ]] && continue
            findings+="$setup:${loud#loud }: git init must be quiet (git init -q)"$'\n'
        done <<<"$(grep '^loud ' <<<"$init_report")"
        if grep -qx 'top' <<<"$init_report"; then
            grep -q '\.git/info/exclude' "$setup" ||
                findings+="$setup: initialises the working directory without a .git/info/exclude for fixture files"$'\n'
        fi

        # Dead fixtures: every checked-in fixture file has to be used, otherwise
        # it is dead weight that drifts away from what the script writes.
        for fixture in "$scenario"/inputs/* "$scenario"/inputs/.[!.]* "$scenario"/inputs/..?*; do
            base="$(basename "$fixture")"
            [[ "$base" == "setup.sh" ]] && continue
            grep -qFw -- "$base" "$setup" ||
                findings+="$fixture: nothing in setup.sh references it (dead fixture)"$'\n'
        done
    fi

    for extra in "$scenario"/inputs/*.sh; do
        [[ "$(basename "$extra")" == "setup.sh" ]] ||
            findings+="$extra: only inputs/setup.sh may be a shell entry point"$'\n'
    done

    finish "$scenario" "$findings"
done

# --- plugin manifests ---------------------------------------------------------

echo
echo "🚀 Plugin manifests..."

for plugin_dir in plugins/*/; do
    plugin="$(basename "$plugin_dir")"
    [[ -d "$plugin_dir/evals" ]] || continue

    findings=""
    claude="$plugin_dir/.claude-plugin/plugin.json"
    tessl="$plugin_dir/.tessl-plugin/plugin.json"

    if [[ -z "$(ls -A "$plugin_dir/evals" 2>/dev/null)" ]]; then
        findings+="$plugin_dir/evals: empty directory"$'\n'
    fi

    if [[ ! -f "$claude" ]]; then
        findings+="$claude: missing (the Claude manifest is the source of truth)"$'\n'
    elif ! jq -e . "$claude" >/dev/null 2>&1; then
        findings+="$claude: invalid JSON"$'\n'
    fi

    if [[ ! -f "$tessl" ]]; then
        findings+="$tessl: missing - scenarios need a tessl manifest to run"$'\n'
    elif ! jq -e . "$tessl" >/dev/null 2>&1; then
        findings+="$tessl: invalid JSON"$'\n'
    else
        # Without a workspace there is no expected name; tessl.json has
        # already failed above, so don't repeat that failure per plugin.
        if [[ -n "$WORKSPACE" ]]; then
            expected_name="${WORKSPACE}/${plugin}"
            actual_name="$(jq -r '.name // ""' "$tessl")"
            [[ "$actual_name" == "$expected_name" ]] ||
                findings+="$tessl: name is '$actual_name' (expected '$expected_name')"$'\n'
        fi
        [[ "$(jq -r 'if has("private") then .private else true end' "$tessl")" == "true" ]] ||
            findings+="$tessl: private must be true for repository plugins"$'\n'

        if [[ -f "$claude" ]] && jq -e . "$claude" >/dev/null 2>&1; then
            claude_version="$(jq -r '.version // ""' "$claude")"
            tessl_version="$(jq -r '.version // ""' "$tessl")"
            [[ "$claude_version" == "$tessl_version" ]] ||
                findings+="$tessl: version '$tessl_version' does not match the Claude manifest '$claude_version'"$'\n'
            claude_description="$(jq -r '.description // ""' "$claude")"
            tessl_description="$(jq -r '.description // ""' "$tessl")"
            [[ "$claude_description" == "$tessl_description" ]] ||
                findings+="$tessl: description does not match the Claude manifest"$'\n'
        fi
    fi

    finish "$plugin_dir" "$findings"
done

# --- skills -------------------------------------------------------------------

echo
echo "🚀 Skills..."

for skill_md in plugins/*/skills/*/SKILL.md; do
    skill_name="$(basename "$(dirname "$skill_md")")"
    frontmatter=""
    findings=""

    # Frontmatter: exactly one name and one description key. A duplicated key is
    # valid YAML for some parsers and makes the skill unreadable for others.
    if [[ "$(head -1 "$skill_md")" != "---" ]]; then
        findings+="$skill_md: no YAML frontmatter"$'\n'
    else
        frontmatter="$(awk 'NR == 1 { next } /^---[[:space:]]*$/ { closed = 1; exit } { print } END { if (!closed) print "\001unclosed" }' "$skill_md")"
        if [[ "$frontmatter" == *$'\001unclosed' ]]; then
            findings+="$skill_md: frontmatter is not closed (no second ---)"$'\n'
            frontmatter=""
        else
            name_keys="$(printf '%s\n' "$frontmatter" | grep -c '^name:')"
            description_keys="$(printf '%s\n' "$frontmatter" | grep -c '^description:')"
            [[ "$name_keys" == "1" ]] ||
                findings+="$skill_md: $name_keys 'name:' keys in frontmatter (expected 1)"$'\n'
            [[ "$description_keys" == "1" ]] ||
                findings+="$skill_md: $description_keys 'description:' keys in frontmatter (expected 1)"$'\n'

            declared_name="$(printf '%s\n' "$frontmatter" | sed -n 's/^name:[[:space:]]*//p' | head -1)"
            [[ "$declared_name" == "$skill_name" ]] ||
                findings+="$skill_md: frontmatter name '$declared_name' does not match the directory '$skill_name'"$'\n'

            # The description may be inline or a YAML block scalar (">" / "|"), which
            # spans the indented lines that follow it.
            description="$(printf '%s\n' "$frontmatter" | sed -n 's/^description:[[:space:]]*//p' | head -1)"
            case "$description" in
                '>' | '>-' | '>+' | '|' | '|-' | '|+')
                    description="$(printf '%s\n' "$frontmatter" | awk '
                        /^description:[[:space:]]*[|>]/ { collecting = 1; next }
                        collecting && /^[[:space:]]+/ { sub(/^[[:space:]]+/, ""); printf "%s ", $0; next }
                        collecting { exit }
                    ')"
                    ;;
                *) ;;
            esac
            if (( ${#description} < 60 )); then
                findings+="$skill_md: description is ${#description} characters - too short to carry activation triggers"$'\n'
            fi
        fi
    fi

    lines="$(wc -l <"$skill_md" | tr -d ' ')"
    ((lines <= 500)) || findings+="$skill_md: $lines lines (Agent Skills limit is 500)"$'\n'

    # allowed-tools: permission patterns match on the start of a command, so an
    # unbounded pattern grants far more than it looks like it does. Whether a
    # skill declares allowed-tools at all, and how narrowly, is a review
    # judgement - this gate rejects only the unbounded patterns.
    # Read from the frontmatter only, inline or as a YAML list, so prose that
    # mentions a pattern is not mistaken for a grant.
    allowed_tools="$(printf '%s\n' "$frontmatter" | awk '
        /^allowed-tools:/ { sub(/^allowed-tools:[[:space:]]*/, ""); print; collecting = 1; next }
        collecting && /^[[:space:]]+-?[[:space:]]*/ { sub(/^[[:space:]]+-?[[:space:]]*/, ""); print; next }
        collecting { exit }
    ' | tr ',' '\n' | sed 's/^[[:space:]]*//; s/[[:space:]]*$//; s/^["'\'']//; s/["'\'']$//')"
    while IFS= read -r tool; do
        [[ -z "$tool" ]] && continue
        if [[ "$tool" == "Bash" || "$tool" =~ ^Bash\([[:space:]]*:?[[:space:]]*\*[[:space:]]*\)$ || "$tool" == *"PATH=*"* ]]; then
            findings+="$skill_md: allowed-tools contains '$tool' - it approves every command"$'\n'
        fi
    done <<<"$allowed_tools"

    # Dead pointers: repo paths a skill tells the agent to read must exist.
    # Only plugin paths: a worked example in prose (a hypothetical diff
    # touching scripts/backup.sh, say) describes code that does not exist yet,
    # while a plugins/... path is a reference the agent is meant to read.
    # shellcheck disable=SC2016  # the pattern is a regex, not an expansion
    while IFS= read -r path; do
        [[ -z "$path" ]] && continue
        [[ "$path" == *"<"* || "$path" == *">"* || "$path" == *"*"* ]] && continue
        path="${path%%#*}"
        path="${path%%:*}"
        [[ -e "$path" ]] ||
            findings+="$skill_md: points at a path that does not exist: $path"$'\n'
    done <<<"$(grep -oE '`plugins/[^`]+`' "$skill_md" | tr -d '`')"

    finish "$skill_md" "$findings"
done

# --- summary ------------------------------------------------------------------

echo
total=$((PASSED + FAILED))
echo "📊 Tests: $total, Passed: $PASSED, Failed: $FAILED"

if [[ "$FAILED" -eq 0 ]]; then
    echo "✅ All tests passed!"
    exit 0
fi
echo "❌ $FAILED test(s) failed!"
exit 1
