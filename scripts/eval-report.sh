#!/usr/bin/env bash

# Render an eval suite report.
#
# This is read-only over a suite directory. It never runs an eval, a model, or
# a network call: it reads the suite manifest, the rubric, and the per-run
# artifacts and turns them into a CLI, JSON, Markdown or HTML report.
#
# The denominator is always the rubric's fixed max_score sum (100 for this
# repository). A partially graded run is reported as "52/100 + 48 pending",
# never renormalised to 52/52. Criteria are paired by name across arms, and a
# criterion only contributes to a delta when both arms scored it for the same
# run.
#
# Usage:
#   scripts/eval-report.sh results/eval-suite/<id>
#   scripts/eval-report.sh results/eval-suite/<id> --format all
#   scripts/eval-report.sh results/eval-suite/<id> --format json > report.json
#
# Reports are written to <suite>/report/ by default.

set -uo pipefail
shopt -s nullglob

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
# The largest jq programs live in scripts/jq/ and are loaded with jq -f, so
# this script stays orchestration rather than embedded data manipulation.
JQ_DIR="$SCRIPT_DIR/jq"

SUITE_DIR=""
FORMAT="cli"
OUT_DIR=""
NO_COLOR_FLAG=0
COLOR=0

usage() {
    cat <<'EOF'
Render an eval suite report.

Usage:
  scripts/eval-report.sh SUITE_DIR [options]

Options:
  --format FORMAT   cli (default), json, md, html, or all
  --out DIR         report directory (default SUITE_DIR/report)
  --no-color        disable ANSI colour even on a terminal
  -h, --help        show this help

NO_COLOR in the environment also disables colour. json and md print their
document to stdout; html, cli and all print the report path(s).
EOF
}

die() {
    echo "eval-report: $*" >&2
    exit 1
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --format | --out)
            if [[ $# -lt 2 || -z "$2" ]]; then
                echo "eval-report: $1 needs a value" >&2
                usage >&2
                exit 2
            fi
            case "$1" in
                --format) FORMAT="$2" ;;
                --out) OUT_DIR="$2" ;;
                *) ;;
            esac
            shift 2
            ;;
        --no-color)
            NO_COLOR_FLAG=1
            shift
            ;;
        -h | --help)
            usage
            exit 0
            ;;
        -*)
            echo "eval-report: unknown argument: $1" >&2
            usage >&2
            exit 2
            ;;
        *)
            [[ -z "$SUITE_DIR" ]] || {
                echo "eval-report: unexpected argument: $1" >&2
                usage >&2
                exit 2
            }
            SUITE_DIR="$1"
            shift
            ;;
    esac
done

case "$FORMAT" in
    cli | json | md | html | all) ;;
    *) die "--format must be cli, json, md, html or all" ;;
esac
[[ -n "$SUITE_DIR" ]] || die "pass a suite directory (e.g. results/eval-suite/<id>)"
[[ -d "$SUITE_DIR" ]] || die "no such suite directory: $SUITE_DIR"
SUITE_DIR="$(cd "$SUITE_DIR" && pwd)"
for required in manifest.json suite.json results.jsonl; do
    [[ -f "$SUITE_DIR/$required" ]] || die "$SUITE_DIR is not a suite directory: $required is missing"
done
jq -e . "$SUITE_DIR/manifest.json" >/dev/null 2>&1 || die "manifest.json is not valid JSON"
jq -e . "$SUITE_DIR/suite.json" >/dev/null 2>&1 || die "suite.json is not valid JSON"

[[ -n "$OUT_DIR" ]] || OUT_DIR="$SUITE_DIR/report"
if ! mkdir -p "$OUT_DIR"; then
    die "cannot create report directory: $OUT_DIR"
fi

TMP="$(mktemp -d "${TMPDIR:-/tmp}/eval-report-XXXXXX")" ||
    die "cannot create a temporary directory"
trap 'rm -rf "$TMP"' EXIT

if [[ -z "${NO_COLOR:-}" && "$NO_COLOR_FLAG" == "0" && -t 1 ]]; then
    COLOR=1
fi

# shellcheck disable=SC2317  # invoked by the renderers below
c() {
    local code="$1"
    shift
    if ((COLOR)); then
        printf '\033[%sm%s\033[0m' "$code" "$*"
    else
        printf '%s' "$*"
    fi
}

# --- read suite metadata ------------------------------------------------------

AGENT_CMD="$(jq -r '.config.agent_cmd // ""' "$SUITE_DIR/manifest.json")"
JUDGE_CMD="$(jq -r '.config.judge_cmd // ""' "$SUITE_DIR/manifest.json")"
ARM="$(jq -r '.config.arm // "both"' "$SUITE_DIR/manifest.json")"
RUNS="$(jq -r '.config.runs // 1' "$SUITE_DIR/manifest.json")"
GIT_SHA="$(jq -r '.config.git_sha // "nogit"' "$SUITE_DIR/manifest.json")"
GIT_DIRTY="$(jq -r '.config.git_dirty // false' "$SUITE_DIR/manifest.json")"
CREATED_AT="$(jq -r '.created_at // ""' "$SUITE_DIR/manifest.json")"
INVOCATIONS="$(jq -r '.counts.agent_invocations // null' "$SUITE_DIR/suite.json")"

MODEL="$(jq -rn --arg a "$AGENT_CMD" '
    $a | try (capture("(--model[ =]|-m )(?<m>[^ ]+)") | .m) catch ""
')"

UNIT_LABEL="$(jq -r '
    if ((.config.units // []) | length) > 0 then
        (.config.units | join(", "))
    else
        ([.scenarios[].plugin] | unique | join(", "))
    end
' "$SUITE_DIR/manifest.json")"
[[ -n "$UNIT_LABEL" ]] || UNIT_LABEL="all plugins"

SCENARIO_COUNT="$(jq '.scenarios | length' "$SUITE_DIR/manifest.json")"
((SCENARIO_COUNT > 0)) || die "manifest.json lists no scenarios"

# A valid judgement covers exactly the still-pending criteria with the rubric's
# own maxima. Anything else is ignored, so the criteria stay pending instead of
# silently lowering the denominator.
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

# --- analyse one scenario -----------------------------------------------------

: >"$TMP/scenarios.jsonl"
scenario_index=0

while IFS=$'\t' read -r plugin scenario declared_units; do
    [[ -n "$plugin" ]] || continue
    scenario_index=$((scenario_index + 1))

    criteria="$SUITE_DIR/criteria/$plugin/$scenario.json"
    [[ -f "$criteria" ]] ||
        criteria="$PROJECT_ROOT/plugins/$plugin/evals/$scenario/criteria.json"
    [[ -f "$criteria" ]] ||
        die "no criteria.json for $plugin/$scenario (looked in the suite and the repository)"
    jq -e '.checklist | type == "array" and length > 0' "$criteria" >/dev/null 2>&1 ||
        die "invalid rubric for $plugin/$scenario: $criteria"
    denominator="$(jq -r '[.checklist[].max_score] | add // 0' "$criteria")"
    [[ "$denominator" =~ ^[0-9]+([.][0-9]+)?$ && "$denominator" != "0" ]] ||
        die "rubric for $plugin/$scenario has no positive max_score sum"

    : >"$TMP/runs.jsonl"
    for arm in with without; do
        [[ "$ARM" == "both" || "$ARM" == "$arm" ]] || continue
        for run in $(seq 1 "$RUNS"); do
            run_dir="$SUITE_DIR/runs/$plugin/$scenario/$arm/run-$run"
            result="$run_dir/result.json"
            mechanical="$run_dir/mechanical.json"
            judgement="$run_dir/judgement.json"
            request="$run_dir/grading-request.json"
            evidence="${run_dir#"$SUITE_DIR"/}"

            status="missing"
            agent_exit="null"
            timed_out="false"
            mech_items="[]"
            judged_items="null"
            usage_json='{"known":false,"source":"none"}'
            judge_usage_json='{"known":false,"source":"none"}'

            if [[ -d "$run_dir" ]]; then
                status="ok"
                if [[ -f "$result" ]]; then
                    agent_exit="$(jq -r '.agent_exit // 0' "$result" 2>/dev/null)"
                    [[ "$agent_exit" =~ ^-?[0-9]+$ ]] || agent_exit="null"
                    timed_out="$(jq -r '.timed_out // false' "$result" 2>/dev/null)"
                else
                    status="missing"
                fi

                if [[ -f "$mechanical" ]]; then
                    mech_items="$(jq -c 'if type == "array" then . else [] end' "$mechanical" 2>/dev/null)"
                elif [[ -f "$result" ]]; then
                    mech_items="$(jq -c '.mechanical.items // []' "$result" 2>/dev/null)"
                fi
                [[ -n "$mech_items" ]] || mech_items="[]"

                if judgement_valid "$judgement" "$request"; then
                    judged_items="$(jq -c '.' "$judgement")"
                fi

                if [[ -f "$run_dir/usage.json" ]]; then
                    parsed="$(jq -c '
                        if type == "object" and (.known | type) == "boolean" then .
                        else {known: false, source: "none"} end
                    ' "$run_dir/usage.json" 2>/dev/null)"
                    [[ -n "$parsed" ]] && usage_json="$parsed"
                fi
                if [[ -f "$run_dir/judge-usage.json" ]]; then
                    parsed="$(jq -c '
                        if type == "object" and (.known | type) == "boolean" then .
                        else {known: false, source: "none"} end
                    ' "$run_dir/judge-usage.json" 2>/dev/null)"
                    [[ -n "$parsed" ]] && judge_usage_json="$parsed"
                fi
            fi

            jq -nc --arg arm "$arm" --argjson run "$run" --arg status "$status" \
                --argjson agent_exit "$agent_exit" --argjson timed_out "$timed_out" \
                --argjson mech_items "$mech_items" --argjson judged_items "$judged_items" \
                --argjson usage "$usage_json" --argjson judge_usage "$judge_usage_json" \
                --arg evidence "$evidence" '
                {
                    arm: $arm,
                    run: $run,
                    status: $status,
                    agent_exit: $agent_exit,
                    timed_out: $timed_out,
                    mechanical: {items: $mech_items},
                    judged: (if $judged_items == null then null
                             else {items: $judged_items} end),
                    usage: $usage,
                    judge_usage: $judge_usage,
                    evidence: $evidence
                }' >>"$TMP/runs.jsonl"
        done
    done

    jq -s --slurpfile crit "$criteria" \
        --arg plugin "$plugin" --arg scenario "$scenario" \
        --arg units "$declared_units" --argjson configured_runs "$RUNS" \
        -f "$JQ_DIR/report-scenario.jq" "$TMP/runs.jsonl" >>"$TMP/scenarios.jsonl"
done < <(jq -r '.scenarios[] | [.plugin, .scenario, ((.units // []) | join(" "))] | @tsv' \
    "$SUITE_DIR/manifest.json")

((scenario_index > 0)) || die "manifest.json lists no scenarios"

# --- aggregate and derive the verdict -----------------------------------------

jq -s --arg suite "$SUITE_DIR" --arg label "$UNIT_LABEL" \
    --arg agent "$AGENT_CMD" --arg judge "$JUDGE_CMD" --arg model "$MODEL" \
    --arg git_sha "$GIT_SHA" --argjson git_dirty "$GIT_DIRTY" \
    --arg created_at "$CREATED_AT" --arg arm "$ARM" \
    --argjson configured_runs "$RUNS" --argjson invocations "$INVOCATIONS" \
    --slurpfile suite_json "$SUITE_DIR/suite.json" \
    --slurpfile results "$SUITE_DIR/results.jsonl" \
    -f "$JQ_DIR/report-aggregate.jq" "$TMP/scenarios.jsonl" >"$TMP/report.json"

jq -e '.verdict.state' "$TMP/report.json" >/dev/null 2>&1 ||
    die "could not derive a verdict from the suite"

# --- renderers ----------------------------------------------------------------

render_md() {
    # shellcheck disable=SC2016  # backticks are literal Markdown, not command substitution
    local state
    state="$(jq -r '.verdict.state' "$TMP/report.json")"
    {
        printf '# Eval report: %s\n\n' "$UNIT_LABEL"
        printf -- "- Suite: \`%s\`\n" "$SUITE_DIR"
        printf -- "- Agent: \`%s\`\n" "$AGENT_CMD"
        [[ -n "$MODEL" ]] && printf -- "- Model: \`%s\`\n" "$MODEL"
        printf -- "- Git: \`%s\`%s  ·  Created: %s  ·  n: %s\n" \
            "$GIT_SHA" "$([[ "$GIT_DIRTY" == "true" ]] && printf ' (dirty)')" "$CREATED_AT" "$RUNS"
        [[ "$INVOCATIONS" != "null" ]] && printf -- '- Agent invocations: %s\n' "$INVOCATIONS"
        printf -- '- Fixed denominator: the rubric sum; unjudged criteria remain pending and are never renormalised.\n'
        printf '\n## Verdict: %s\n\n' "$state"
        printf '%s\n\n' "$(jq -r '.verdict.reason' "$TMP/report.json")"
        jq -r '
            def fmt: if . == null then "—"
                     else ((. * 10 | round) / 10 | tostring) as $s
                     | if ($s | contains(".")) then $s else $s + ".0" end end;
            def pfmt: if . == null then "—" else (if . > 0 then "+" else "" end) + fmt end;
            "| With | Without | Delta | Won | Lost | Tied | Pending | Coverage | Failures |",
            "| --- | --- | --- | --- | --- | --- | --- | --- | --- |",
            ("| " + ([.totals.with, .totals.without] | map(fmt) | join(" | ")) + " | "
             + (.totals.delta | pfmt) + " | "
             + ([.totals.won, .totals.lost, .totals.tied, .totals.pending_points,
                 ((.totals.coverage_percent | fmt) + "%"), (.totals.runner_failures | tostring)] | join(" | "))
             + " |")
        ' "$TMP/report.json"
        printf '\n## Regressions\n\n'
        if [[ "$(jq -r '.totals.regressions' "$TMP/report.json")" == "0" ]]; then
            printf 'None.\n'
        else
            printf '| Scenario | Criterion | With | Without | Delta |\n| --- | --- | --- | --- | --- |\n'
            jq -r '
                def fmt: if . == null then "—"
                         else ((. * 10 | round) / 10 | tostring) as $s
                         | if ($s | contains(".")) then $s else $s + ".0" end end;
                def pfmt: if . == null then "—" else (if . > 0 then "+" else "" end) + fmt end;
                def esc: tostring | gsub("\\|"; "\\|");
                .criteria[] | select(.delta != null and .delta < -0.0001)
                | [ (.plugin + "/" + .scenario), .name, (.with | fmt), (.without | fmt), (.delta | pfmt) ]
                | map(esc) | "| " + join(" | ") + " |"
            ' "$TMP/report.json"
        fi
        printf '\n## Scenarios\n\n'
        printf '| Scenario | With | Without | Delta | Pending | Status |\n| --- | --- | --- | --- | --- | --- |\n'
        jq -r '
            def fmt: if . == null then "—"
                     else ((. * 10 | round) / 10 | tostring) as $s
                     | if ($s | contains(".")) then $s else $s + ".0" end end;
            def pfmt: if . == null then "—" else (if . > 0 then "+" else "" end) + fmt end;
            def esc: tostring | gsub("\\|"; "\\|");
            .scenarios | sort_by(.delta // -1000000) | reverse[]
            | [ (.plugin + "/" + .scenario), (.arms.with.percent_mean | fmt),
                (.arms.without.percent_mean | fmt), (.delta | pfmt),
                ((.pending_points | tostring) + " pts"), .status ]
            | map(esc) | "| " + join(" | ") + " |"
        ' "$TMP/report.json"
        printf '\n## Criteria\n\n'
        printf '| Scenario | Criterion | With | Without | Delta | Status |\n| --- | --- | --- | --- | --- | --- |\n'
        jq -r '
            def fmt: if . == null then "—"
                     else ((. * 10 | round) / 10 | tostring) as $s
                     | if ($s | contains(".")) then $s else $s + ".0" end end;
            def pfmt: if . == null then "—" else (if . > 0 then "+" else "" end) + fmt end;
            def arrow: if . == null then "—" elif . > 0.0001 then "▲" elif . < -0.0001 then "▼" else "=" end;
            def esc: tostring | gsub("\\|"; "\\|");
            .criteria | sort_by([(if .delta == null then 1 else 0 end), (.delta // 0)])[]
            | [ (.plugin + "/" + .scenario), .name, (.with | fmt), (.without | fmt),
                (((.delta | arrow) + " " + (.delta | pfmt))), .status ]
            | map(esc) | "| " + join(" | ") + " |"
        ' "$TMP/report.json"
        printf '\n## Cost\n\n'
        cost_known="$(jq -r '
            [.cost.with.agent.cost.total, .cost.without.agent.cost.total,
             .cost.with.judge.cost.total, .cost.without.judge.cost.total,
             .cost.with.combined.tokens.total, .cost.without.combined.tokens.total]
            | map(select(. != null)) | length
        ' "$TMP/report.json")"
        if [[ "$cost_known" == "0" ]]; then
            printf "Cost: not reported by the agent command (0/%s runs); add \`--format json\`, \`--output-format json\`, or \`--usage-cmd\`.\n" \
                "$(jq -r '.cost.with.runs_total + .cost.without.runs_total' "$TMP/report.json")"
        else
            printf '| Cost | With | Without | Delta |\n| --- | --- | --- | --- |\n'
            jq -r '
                def money: if . == null then "—"
                           else "$" + (((. * 1000 | round) / 1000) | tostring) end;
                def pad($n):
                    . as $s
                    | ($s | index(".")) as $dot
                    | if $dot == null then $s + "." + ([range(0; $n) | "0"] | join(""))
                      else (($s | length) - $dot - 1) as $have
                      | $s + ([range(0; ($n - $have)) | "0"] | join(""))
                      end;
                def fixed3: ((. * 1000 | round) / 1000 | tostring) | pad(3);
                def fixed5: ((. * 100000 | round) / 100000 | tostring) | pad(5);
                def money_delta:
                    if . == null then "—"
                    elif . == 0 then "$0.000"
                    elif . > 0 then
                        if . >= 0.001 then "+$" + fixed3
                        elif . >= 0.0001 then "+$" + fixed5
                        else "< $0.0001" end
                    else
                        (-. ) as $a
                        | if $a >= 0.001 then "-$" + ($a | fixed3)
                          elif $a >= 0.0001 then "-$" + ($a | fixed5)
                          else "> -$0.0001" end
                    end;
                def tokens: if . == null then "—"
                            elif . >= 1000000 then (((. / 1000000) * 10 | round / 10) | tostring) + "M"
                            elif . >= 1000 then (((. / 1000) * 10 | round / 10) | tostring) + "k"
                            else (. | tostring) end;
                def token_delta: if . == null then "—"
                                 elif . > 0 then "+" + (. | tokens)
                                 elif . < 0 then "-" + ((. * -1) | tokens)
                                 else "0" end;
                def sum_delta($w; $o): if $w == null or $o == null then null else ($w - $o) end;
                . as $m
                | ("| agent | " + ($m.cost.with.agent.cost.total | money) + " | "
                   + ($m.cost.without.agent.cost.total | money) + " | "
                   + (sum_delta($m.cost.with.agent.cost.total; $m.cost.without.agent.cost.total) | money_delta) + " |"),
                  ("| judge | " + ($m.cost.with.judge.cost.total | money) + " | "
                   + ($m.cost.without.judge.cost.total | money) + " | "
                   + (sum_delta($m.cost.with.judge.cost.total; $m.cost.without.judge.cost.total) | money_delta) + " |"),
                  ("| total | " + ($m.cost.with.combined.cost.total | money) + " | "
                   + ($m.cost.without.combined.cost.total | money) + " | "
                   + (sum_delta($m.cost.with.combined.cost.total; $m.cost.without.combined.cost.total) | money_delta) + " |"),
                  ("| tokens | " + ($m.cost.with.combined.tokens.total | tokens) + " | "
                   + ($m.cost.without.combined.tokens.total | tokens) + " | "
                   + (sum_delta($m.cost.with.combined.tokens.total; $m.cost.without.combined.tokens.total) | token_delta) + " |")
            ' "$TMP/report.json"
            agent_known=$(( $(jq -r '.cost.with.agent.cost.runs_known' "$TMP/report.json") + $(jq -r '.cost.without.agent.cost.runs_known' "$TMP/report.json") ))
            judge_known=$(( $(jq -r '.cost.with.judge.cost.runs_known' "$TMP/report.json") + $(jq -r '.cost.without.judge.cost.runs_known' "$TMP/report.json") ))
            combined_known=$(( $(jq -r '.cost.with.combined.cost.runs_known' "$TMP/report.json") + $(jq -r '.cost.without.combined.cost.runs_known' "$TMP/report.json") ))
            runs_total=$(( $(jq -r '.cost.with.runs_total' "$TMP/report.json") + $(jq -r '.cost.without.runs_total' "$TMP/report.json") ))
            partial=""
            ((combined_known < runs_total)) && partial=" (partial)"
            printf '\nCoverage: agent %s/%s, judge %s/%s, total %s/%s%s. Unknown usage is never counted as zero.\n' \
                "$agent_known" "$runs_total" "$judge_known" "$runs_total" \
                "$combined_known" "$runs_total" "$partial"
            cpp="$(jq -r '.cost.cost_per_delta_point // "null"' "$TMP/report.json")"
            if [[ "$cpp" != "null" ]]; then
                printf 'Efficiency: %s pts for %s per run (%s per point).\n' \
                    "$(cli_delta "$(jq -r '.totals.delta' "$TMP/report.json")")" \
                    "$(cli_money_delta "$(jq -r '.cost.delta.combined_cost' "$TMP/report.json")")" \
                    "$(cli_cost_per_point "$cpp")"
            else
                printf 'Efficiency: — (partial cost coverage or no positive score delta).\n'
            fi
        fi
        printf "\nRaw JSON: \`report.json\`  ·  HTML: \`report.html\`\n"
    } >"$TMP/report.md"
}

render_html() {
    local cpp_text
    cpp_text="$(cli_cost_per_point "$(jq -r '.cost.cost_per_delta_point // "null"' "$TMP/report.json")")"
    {
        cat <<'HTML'
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Eval report</title>
<style>
:root {
  --bg: #ffffff; --fg: #1a1a1a; --muted: #555555; --line: #d8d8d8;
  --card: #f5f5f5; --good: #0a7a3d; --bad: #b3261e; --warn: #8a5a00;
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #111417; --fg: #e8e8e8; --muted: #a0a6ad; --line: #333a40;
    --card: #1a1f24; --good: #5cd68a; --bad: #ff8a80; --warn: #ffd166;
  }
}
* { box-sizing: border-box; }
body { margin: 0 auto; max-width: 1100px; padding: 2rem 1.25rem 4rem;
  background: var(--bg, #fff); color: var(--fg, #1a1a1a);
  font: 15px/1.55 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
h1, h2 { line-height: 1.2; }
h2 { margin-top: 2rem; border-bottom: 1px solid var(--line, #ddd); padding-bottom: .35rem; }
a { color: inherit; }
.meta { color: var(--muted, #555); }
.verdict { border: 1px solid var(--line, #ddd); border-left: 6px solid var(--warn, #8a5a00);
  background: var(--card, #f5f5f5); padding: 1rem 1.25rem; border-radius: 8px; }
.verdict.incomplete { border-left-color: var(--bad, #b3261e); }
.verdict.helps { border-left-color: var(--good, #0a7a3d); }
.verdict.hurts { border-left-color: var(--bad, #b3261e); }
.verdict .state { font-size: 1.4rem; font-weight: 700; text-transform: uppercase; letter-spacing: .04em; }
.verdict .reason { color: var(--muted, #555); margin-top: .35rem; }
.cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: .75rem; margin-top: 1rem; }
.card { background: var(--card, #f5f5f5); border: 1px solid var(--line, #ddd);
  border-radius: 8px; padding: .75rem .9rem; }
.card .k { color: var(--muted, #555); font-size: .8rem; text-transform: uppercase; letter-spacing: .05em; }
.card .v { font-size: 1.35rem; font-weight: 600; }
.pos { color: var(--good, #0a7a3d); }
.neg { color: var(--bad, #b3261e); }
table { width: 100%; border-collapse: collapse; margin-top: .75rem; font-size: .92rem; }
th, td { text-align: left; padding: .45rem .55rem; border-bottom: 1px solid var(--line, #ddd); vertical-align: top; }
th { color: var(--muted, #555); font-size: .78rem; text-transform: uppercase; letter-spacing: .05em; }
td.num { font-variant-numeric: tabular-nums; white-space: nowrap; }
.note { color: var(--muted, #555); font-size: .9rem; }
.badge { display: inline-block; padding: .1rem .45rem; border: 1px solid var(--line, #ddd);
  border-radius: 999px; font-size: .78rem; }
ul.links { columns: 2; }
@media (max-width: 640px) { ul.links { columns: 1; } table { display: block; overflow-x: auto; } }
</style>
</head>
<body>
HTML
        jq -r '
            def e: tostring | gsub("\n"; " ") | @html;
            def fmt: if . == null then "—"
                     else ((. * 10 | round) / 10 | tostring) as $s
                     | if ($s | contains(".")) then $s else $s + ".0" end end;
            def pfmt: if . == null then "—" else (if . > 0 then "+" else "" end) + fmt end;
            def arrow: if . == null then "—" elif . > 0.0001 then "▲" elif . < -0.0001 then "▼" else "=" end;
            def cls: if . == null then "" elif . > 0.0001 then "pos" elif . < -0.0001 then "neg" else "" end;
            def money: if . == null then "—"
                       else "$" + (((. * 1000 | round) / 1000) | tostring) end;
            def pad($n):
                . as $s
                | ($s | index(".")) as $dot
                | if $dot == null then $s + "." + ([range(0; $n) | "0"] | join(""))
                  else (($s | length) - $dot - 1) as $have
                  | $s + ([range(0; ($n - $have)) | "0"] | join(""))
                  end;
            def fixed3: ((. * 1000 | round) / 1000 | tostring) | pad(3);
            def fixed5: ((. * 100000 | round) / 100000 | tostring) | pad(5);
            def money_delta:
                if . == null then "—"
                elif . == 0 then "$0.000"
                elif . > 0 then
                    if . >= 0.001 then "+$" + fixed3
                    elif . >= 0.0001 then "+$" + fixed5
                    else "< $0.0001" end
                else
                    (-. ) as $a
                    | if $a >= 0.001 then "-$" + ($a | fixed3)
                      elif $a >= 0.0001 then "-$" + ($a | fixed5)
                      else "> -$0.0001" end
                end;
            def tokens: if . == null then "—"
                        elif . >= 1000000 then (((. / 1000000) * 10 | round / 10) | tostring) + "M"
                        elif . >= 1000 then (((. / 1000) * 10 | round / 10) | tostring) + "k"
                        else (. | tostring) end;
            def token_delta: if . == null then "—"
                             elif . > 0 then "+" + (. | tokens)
                             elif . < 0 then "-" + ((. * -1) | tokens)
                             else "0" end;
            def coverage($c): ($c.runs_known | tostring) + "/" + ($c.runs_total | tostring);
            def sum_delta($w; $o): if $w == null or $o == null then null else ($w - $o) end;
            . as $m
            | "<h1>Eval report: \($m.metadata.label | e)</h1>",
              "<p class=\"meta\">Suite: <code>\($m.metadata.suite_dir | e)</code><br>"
              + "Agent: <code>\($m.metadata.agent_cmd | e)</code>"
              + (if $m.metadata.model then "  ·  Model: <code>\($m.metadata.model | e)</code>" else "" end)
              + "<br>Git: <code>\($m.metadata.git_sha | e)</code>"
              + (if $m.metadata.git_dirty then " (dirty)" else "" end)
              + "  ·  Created: \($m.metadata.created_at | e)"
              + "  ·  n: \($m.metadata.n)"
              + (if $m.metadata.agent_invocations != null then "  ·  Agent invocations: \($m.metadata.agent_invocations)" else "" end)
              + "</p>",
              "<p class=\"note\"><strong>Fixed denominator.</strong> Scores use the full rubric "
              + "max_score sum; a run with only part of its checklist judged is reported as "
              + "<code>52/100 + 48 pending</code>, never renormalised to <code>52/52</code>. "
              + "Criteria are paired by name across arms, and only runs scored in both arms contribute to a delta.</p>",
              "<section class=\"verdict \($m.verdict.state | gsub(" "; "-") | @html)\">"
              + "<div class=\"state\">\($m.verdict.state | e)</div>"
              + "<div class=\"reason\">\($m.verdict.reason | e)</div></section>",
              "<div class=\"cards\">",
              "<div class=\"card\"><div class=\"k\">With</div><div class=\"v\">\($m.totals.with | fmt)</div></div>",
              "<div class=\"card\"><div class=\"k\">Without</div><div class=\"v\">\($m.totals.without | fmt)</div></div>",
              "<div class=\"card\"><div class=\"k\">Delta</div><div class=\"v \($m.totals.delta | cls)\">\($m.totals.delta | pfmt)</div></div>",
              "<div class=\"card\"><div class=\"k\">Criteria</div><div class=\"v\">\($m.totals.won) / \($m.totals.lost) / \($m.totals.tied)</div><div class=\"note\">won / lost / tied</div></div>",
              "<div class=\"card\"><div class=\"k\">Pending</div><div class=\"v\">\($m.totals.pending_points)</div><div class=\"note\">points unjudged</div></div>",
              "<div class=\"card\"><div class=\"k\">Coverage</div><div class=\"v\">\($m.totals.coverage_percent | fmt)%</div></div>",
              "<div class=\"card\"><div class=\"k\">Failures</div><div class=\"v\">\($m.totals.runner_failures)</div></div>",
              "<div class=\"card\"><div class=\"k\">Cost with</div><div class=\"v\">\($m.cost.with.combined.cost.total | money)</div><div class=\"note\">\(coverage($m.cost.with.combined.cost)) runs</div></div>",
              "<div class=\"card\"><div class=\"k\">Cost without</div><div class=\"v\">\($m.cost.without.combined.cost.total | money)</div><div class=\"note\">\(coverage($m.cost.without.combined.cost)) runs</div></div>",
              "<div class=\"card\"><div class=\"k\">Cost delta</div><div class=\"v \(sum_delta($m.cost.with.combined.cost.total; $m.cost.without.combined.cost.total) | cls)\">\(sum_delta($m.cost.with.combined.cost.total; $m.cost.without.combined.cost.total) | money_delta)</div></div>",
              "<div class=\"card\"><div class=\"k\">Tokens</div><div class=\"v\">\($m.cost.with.combined.tokens.total | tokens) / \($m.cost.without.combined.tokens.total | tokens)</div><div class=\"note\">with / without</div></div>",
              "</div>",
              "<h2>Criteria</h2>",
              "<table><thead><tr><th>Scenario</th><th>Criterion</th><th>With</th><th>Without</th><th>Delta</th><th>Status</th></tr></thead><tbody>",
              ($m.criteria | sort_by([(if .delta == null then 1 else 0 end), (.delta // 0)])[]
               | "<tr><td>\(.plugin | e)/\(.scenario | e)</td><td>\(.name | e)</td>"
                 + "<td class=\"num\">\(.with | fmt)</td><td class=\"num\">\(.without | fmt)</td>"
                 + "<td class=\"num \(.delta | cls)\">\(.delta | arrow) \(.delta | pfmt)</td>"
                 + "<td>\(.status | e)</td></tr>"),
              "</tbody></table>",
              "<h2>Scenarios</h2>",
              "<table><thead><tr><th>Scenario</th><th>With</th><th>Without</th><th>Delta</th><th>Pending</th><th>Status</th></tr></thead><tbody>",
              ($m.scenarios | sort_by(.delta // -1000000) | reverse[]
               | "<tr><td>\(.plugin | e)/\(.scenario | e)</td>"
                 + "<td class=\"num\">\(.arms.with.percent_mean | fmt)</td>"
                 + "<td class=\"num\">\(.arms.without.percent_mean | fmt)</td>"
                 + "<td class=\"num \(.delta | cls)\">\(.delta | arrow) \(.delta | pfmt)</td>"
                 + "<td class=\"num\">\(.pending_points) pts</td>"
                 + "<td>\(.status | e)</td></tr>"),
              "</tbody></table>",
              "<h2>Regressions</h2>",
              (if ($m.criteria | map(select(.delta != null and .delta < -0.0001)) | length) == 0 then
                  "<p class=\"note\">None.</p>"
               else
                  "<table><thead><tr><th>Scenario</th><th>Criterion</th><th>With</th><th>Without</th><th>Delta</th></tr></thead><tbody>",
                  ($m.criteria[] | select(.delta != null and .delta < -0.0001)
                   | "<tr><td>\(.plugin | e)/\(.scenario | e)</td><td>\(.name | e)</td>"
                     + "<td class=\"num\">\(.with | fmt)</td><td class=\"num\">\(.without | fmt)</td>"
                     + "<td class=\"num neg\">\(.delta | pfmt)</td></tr>"),
                  "</tbody></table>"
               end),
              "<h2>Cost</h2>",
              (if ([ $m.cost.with.agent.cost.total, $m.cost.without.agent.cost.total,
                     $m.cost.with.judge.cost.total, $m.cost.without.judge.cost.total,
                     $m.cost.with.combined.tokens.total, $m.cost.without.combined.tokens.total ]
                   | map(select(. != null)) | length) == 0 then
                  "<p class=\"note\">Cost: not reported by the agent command (0/"
                  + (($m.cost.with.runs_total + $m.cost.without.runs_total) | tostring)
                  + " runs); add <code>--format json</code>, <code>--output-format json</code>, "
                  + "or <code>--usage-cmd</code>.</p>"
               else
                  "<table><thead><tr><th>Cost</th><th>With</th><th>Without</th><th>Delta</th><th>Coverage</th></tr></thead><tbody>",
                  "<tr><td>agent</td><td class=\"num\">\($m.cost.with.agent.cost.total | money)</td><td class=\"num\">\($m.cost.without.agent.cost.total | money)</td><td class=\"num\">\(sum_delta($m.cost.with.agent.cost.total; $m.cost.without.agent.cost.total) | money_delta)</td><td>\(coverage($m.cost.with.agent.cost)) / \(coverage($m.cost.without.agent.cost))</td></tr>",
                  "<tr><td>judge</td><td class=\"num\">\($m.cost.with.judge.cost.total | money)</td><td class=\"num\">\($m.cost.without.judge.cost.total | money)</td><td class=\"num\">\(sum_delta($m.cost.with.judge.cost.total; $m.cost.without.judge.cost.total) | money_delta)</td><td>\(coverage($m.cost.with.judge.cost)) / \(coverage($m.cost.without.judge.cost))</td></tr>",
                  "<tr><td>total</td><td class=\"num\">\($m.cost.with.combined.cost.total | money)</td><td class=\"num\">\($m.cost.without.combined.cost.total | money)</td><td class=\"num\">\(sum_delta($m.cost.with.combined.cost.total; $m.cost.without.combined.cost.total) | money_delta)</td><td>\(coverage($m.cost.with.combined.cost)) / \(coverage($m.cost.without.combined.cost))</td></tr>",
                  "<tr><td>tokens</td><td class=\"num\">\($m.cost.with.combined.tokens.total | tokens)</td><td class=\"num\">\($m.cost.without.combined.tokens.total | tokens)</td><td class=\"num\">\(sum_delta($m.cost.with.combined.tokens.total; $m.cost.without.combined.tokens.total) | token_delta)</td><td>\(coverage($m.cost.with.combined.tokens)) / \(coverage($m.cost.without.combined.tokens))</td></tr>",
                  "</tbody></table>",
                  "<p class=\"note\">Unknown usage is never counted as zero. Coverage is known runs / total runs.</p>",
                  (if $m.cost.cost_per_delta_point != null then
                      "<p class=\"note\">Efficiency: \($m.totals.delta | pfmt) pts for \($m.cost.delta.combined_cost | money_delta) per run (\($cpp | e) per point).</p>"
                   else
                      "<p class=\"note\">Efficiency: — (partial cost coverage or no positive score delta).</p>"
                   end)
               end),
              "<h2>Evidence</h2>",
              "<p class=\"note\">Relative links; open from the suite directory. The raw JSON is beside this file.</p>",
              "<ul class=\"links\">",
              ($m.scenarios[] | "<li><a href=\"../runs/\(.plugin | @uri)/\(.scenario | @uri)\">\(.plugin | e)/\(.scenario | e)</a></li>"),
              "<li><a href=\"report.json\">report.json</a></li>",
              "</ul>"
        ' --arg cpp "$cpp_text" "$TMP/report.json"
        cat <<'HTML'
</body>
</html>
HTML
    } >"$TMP/report.html"
}

cli_num() {
    local value="$1"
    if [[ "$value" == "null" || -z "$value" ]]; then
        printf '—'
        return
    fi
    awk -v v="$value" 'BEGIN { printf "%.1f", v }'
}

cli_delta() {
    local value="$1"
    if [[ "$value" == "null" || -z "$value" ]]; then
        printf '—'
        return
    fi
    awk -v v="$value" 'BEGIN { if (v > 0) printf "+%.1f", v; else printf "%.1f", v }'
}

cli_money() {
    local value="$1"
    if [[ "$value" == "null" || -z "$value" ]]; then
        printf '—'
        return
    fi
    awk -v v="$value" 'BEGIN { printf "$%.3f", v }'
}

cli_money_delta() {
    local value="$1"
    if [[ "$value" == "null" || -z "$value" ]]; then
        printf '—'
        return
    fi
    awk -v v="$value" 'BEGIN {
        if (v == 0) { printf "$0.000"; exit }
        if (v > 0) {
            if (v >= 0.001) printf "+$%.3f", v
            else if (v >= 0.0001) printf "+$%.5f", v
            else print "< $0.0001"
        } else {
            a = -v
            if (a >= 0.001) printf "-$%.3f", a
            else if (a >= 0.0001) printf "-$%.5f", a
            else print "> -$0.0001"
        }
    }'
}

cli_tokens() {
    local value="$1"
    if [[ "$value" == "null" || -z "$value" ]]; then
        printf '—'
        return
    fi
    awk -v n="$value" 'BEGIN {
        if (n >= 1000000) printf "%.1fM", n / 1000000
        else if (n >= 1000) printf "%.1fk", n / 1000
        else printf "%.0f", n
    }'
}

cli_tokens_delta() {
    local value="$1"
    if [[ "$value" == "null" || -z "$value" ]]; then
        printf '—'
        return
    fi
    awk -v n="$value" 'BEGIN {
        sign = (n > 0) ? "+" : ((n < 0) ? "-" : "")
        if (n < 0) n = -n
        if (n >= 1000000) printf "%s%.1fM", sign, n / 1000000
        else if (n >= 1000) printf "%s%.1fk", sign, n / 1000
        else printf "%s%.0f", sign, n
    }'
}

cli_cost_per_point() {
    local value="$1"
    if [[ "$value" == "null" || -z "$value" ]]; then
        printf '—'
        return
    fi
    awk -v v="$value" 'BEGIN {
        if (v <= 0) print "—"
        else if (v >= 0.001) printf "$%.3f", v
        else if (v >= 0.0001) printf "$%.5f", v
        else print "< $0.0001"
    }'
}

render_cli() {
    local state reason with without delta won lost tied pending coverage failures
    local with_min with_max without_min without_max
    state="$(jq -r '.verdict.state' "$TMP/report.json")"
    reason="$(jq -r '.verdict.reason' "$TMP/report.json")"
    with="$(jq -r '.totals.with // "null"' "$TMP/report.json")"
    without="$(jq -r '.totals.without // "null"' "$TMP/report.json")"
    delta="$(jq -r '.totals.delta // "null"' "$TMP/report.json")"
    won="$(jq -r '.totals.won' "$TMP/report.json")"
    lost="$(jq -r '.totals.lost' "$TMP/report.json")"
    tied="$(jq -r '.totals.tied' "$TMP/report.json")"
    pending="$(jq -r '.totals.pending_points' "$TMP/report.json")"
    coverage="$(jq -r '.totals.coverage_percent' "$TMP/report.json")"
    failures="$(jq -r '.totals.runner_failures' "$TMP/report.json")"
    with_min="$(jq -r '.totals.with_min // "null"' "$TMP/report.json")"
    with_max="$(jq -r '.totals.with_max // "null"' "$TMP/report.json")"
    without_min="$(jq -r '.totals.without_min // "null"' "$TMP/report.json")"
    without_max="$(jq -r '.totals.without_max // "null"' "$TMP/report.json")"

    printf '%s\n' "$(c '1' "Eval report: $UNIT_LABEL")"
    printf 'Suite: %s\n' "$SUITE_DIR"
    printf 'Agent: %s\n' "$AGENT_CMD"
    [[ -n "$MODEL" ]] && printf 'Model: %s\n' "$MODEL"
    printf 'Git: %s%s   Created: %s   n: %s   Arm: %s\n' \
        "$GIT_SHA" "$([[ "$GIT_DIRTY" == "true" ]] && printf ' (dirty)')" \
        "$CREATED_AT" "$RUNS" "$ARM"
    [[ "$INVOCATIONS" != "null" ]] && printf 'Invocations: %s\n' "$INVOCATIONS"
    printf '\n'
    printf '%s\n' "$(c '1;33' "Verdict: $state")"
    printf '  %s\n\n' "$reason"
    printf 'With: %s/100    Without: %s/100    Delta: %s\n' \
        "$(cli_num "$with")" "$(cli_num "$without")" "$(cli_delta "$delta")"
    printf 'Criteria: %s won, %s lost, %s tied    Coverage: %s%%    Pending: %s points (both arms)    Failures: %s\n' \
        "$won" "$lost" "$tied" "$(cli_num "$coverage")" "$pending" "$failures"

    local cost_known cost_runs_total
    cost_known="$(jq -r '
        [.cost.with.agent.cost.total, .cost.without.agent.cost.total,
         .cost.with.judge.cost.total, .cost.without.judge.cost.total,
         .cost.with.combined.tokens.total, .cost.without.combined.tokens.total]
        | map(select(. != null)) | length
    ' "$TMP/report.json")"
    cost_runs_total="$(jq -r '.cost.with.runs_total + .cost.without.runs_total' "$TMP/report.json")"
    if [[ "$cost_known" == "0" ]]; then
        printf 'Cost: not reported by the agent command (0/%s runs); add --format json, --output-format json, or --usage-cmd.\n' \
            "$cost_runs_total"
    else
        local with_known without_known with_total without_total
        local agent_known judge_known combined_known
        with_known="$(jq -r '.cost.with.combined.cost.runs_known' "$TMP/report.json")"
        without_known="$(jq -r '.cost.without.combined.cost.runs_known' "$TMP/report.json")"
        with_total="$(jq -r '.cost.with.runs_total' "$TMP/report.json")"
        without_total="$(jq -r '.cost.without.runs_total' "$TMP/report.json")"
        agent_known=$(( $(jq -r '.cost.with.agent.cost.runs_known' "$TMP/report.json") + $(jq -r '.cost.without.agent.cost.runs_known' "$TMP/report.json") ))
        judge_known=$(( $(jq -r '.cost.with.judge.cost.runs_known' "$TMP/report.json") + $(jq -r '.cost.without.judge.cost.runs_known' "$TMP/report.json") ))
        combined_known=$((with_known + without_known))
        printf '\n%-18s %12s %12s %12s\n' "COST" "WITH" "WITHOUT" "DELTA"
        jq -r '
            def v: if . == null then "null" else tostring end;
            def sum_delta($w; $o):
                if $w == null or $o == null then null else ($w - $o) end;
            [
                ["agent", .cost.with.agent.cost.total, .cost.without.agent.cost.total,
                 sum_delta(.cost.with.agent.cost.total; .cost.without.agent.cost.total)],
                ["judge", .cost.with.judge.cost.total, .cost.without.judge.cost.total,
                 sum_delta(.cost.with.judge.cost.total; .cost.without.judge.cost.total)],
                ["total", .cost.with.combined.cost.total, .cost.without.combined.cost.total,
                 sum_delta(.cost.with.combined.cost.total; .cost.without.combined.cost.total)],
                ["tokens", .cost.with.combined.tokens.total, .cost.without.combined.tokens.total,
                 sum_delta(.cost.with.combined.tokens.total; .cost.without.combined.tokens.total)]
            ][] | map(v) | @tsv
        ' "$TMP/report.json" | while IFS=$'\t' read -r label w o d; do
            if [[ "$label" == "tokens" ]]; then
                printf '%-18s %12s %12s %12s\n' "$label" \
                    "$(cli_tokens "$w")" "$(cli_tokens "$o")" \
                    "$(cli_tokens_delta "$d")"
            else
                printf '%-18s %12s %12s %12s\n' "$label" \
                    "$(cli_money "$w")" "$(cli_money "$o")" "$(cli_money_delta "$d")"
            fi
        done
        local total_known total_runs partial=""
        total_known=$((with_known + without_known))
        total_runs=$((with_total + without_total))
        if ((total_known < total_runs)); then
            partial=" (partial)"
        fi
        printf '  coverage: agent %s/%s, judge %s/%s, total %s/%s%s\n' \
            "$agent_known" "$total_runs" "$judge_known" "$total_runs" \
            "$combined_known" "$total_runs" "$partial"
        local cpp score_delta cost_delta
        cpp="$(jq -r '.cost.cost_per_delta_point // "null"' "$TMP/report.json")"
        score_delta="$(jq -r '.totals.delta // "null"' "$TMP/report.json")"
        cost_delta="$(jq -r '.cost.delta.combined_cost // "null"' "$TMP/report.json")"
        if [[ "$cpp" != "null" ]]; then
            printf 'Efficiency: %s pts for %s per run  (%s per point)\n' \
                "$(cli_delta "$score_delta")" "$(cli_money_delta "$cost_delta")" \
                "$(cli_cost_per_point "$cpp")"
        else
            printf 'Efficiency: — (cost coverage or score delta does not support a per-point claim)\n'
        fi
    fi

    printf '\n%s\n' "$(c '1' 'Regressions')"
    if [[ "$(jq -r '.totals.regressions' "$TMP/report.json")" == "0" ]]; then
        printf '  none\n'
    else
        jq -r '
            def fmt: if . == null then "—" else ((. * 10 | round) / 10 | tostring) as $s
                     | if ($s | contains(".")) then $s else $s + ".0" end end;
            def pfmt: if . == null then "—" else (if . > 0 then "+" else "" end) + fmt end;
            .criteria[] | select(.delta != null and .delta < -0.0001)
            | [(.plugin + "/" + .scenario), .name, (.with | fmt), (.without | fmt), (.delta | pfmt)]
            | @tsv
        ' "$TMP/report.json" | while IFS=$'\t' read -r scenario criterion w o d; do
            printf '  %-36s %-42s %s -> %s (%s)\n' "$scenario" "$criterion" "$w" "$o" "$d"
        done
    fi

    printf '\n%s\n' "$(c '1' 'Scenarios (delta descending)')"
    printf '  %-44s %8s %8s %8s %10s  %s\n' "SCENARIO" "WITH" "WITHOUT" "DELTA" "PENDING" "STATUS"
    jq -r '
        def fmt: if . == null then "—" else ((. * 10 | round) / 10 | tostring) as $s
                 | if ($s | contains(".")) then $s else $s + ".0" end end;
        def pfmt: if . == null then "—" else (if . > 0 then "+" else "" end) + fmt end;
        .scenarios | sort_by(.delta // -1000000) | reverse[]
        | [(.plugin + "/" + .scenario), (.arms.with.percent_mean | fmt),
           (.arms.without.percent_mean | fmt), (.delta | pfmt),
           ((.pending_points | tostring) + " pts"), .status] | @tsv
    ' "$TMP/report.json" | while IFS=$'\t' read -r scenario w o d p s; do
        printf '  %-44s %8s %8s %8s %10s  %s\n' "$scenario" "$w" "$o" "$d" "$p" "$s"
    done
    printf '  arm range: with %s–%s, without %s–%s (min–max over runs)\n' \
        "$(cli_num "$with_min")" "$(cli_num "$with_max")" \
        "$(cli_num "$without_min")" "$(cli_num "$without_max")"

    printf '\n%s\n' "$(c '1' 'Criteria (regressions first)')"
    printf '  %-36s %-34s %7s %7s %8s  %s\n' "SCENARIO" "CRITERION" "WITH" "WITHOUT" "DELTA" "STATUS"
    jq -r '
        def fmt: if . == null then "—" else ((. * 10 | round) / 10 | tostring) as $s
                 | if ($s | contains(".")) then $s else $s + ".0" end end;
        def pfmt: if . == null then "—" else (if . > 0 then "+" else "" end) + fmt end;
        def arrow: if . == null then "—" elif . > 0.0001 then "▲" elif . < -0.0001 then "▼" else "=" end;
        .criteria | sort_by([(if .delta == null then 1 else 0 end), (.delta // 0)])[]
        | [(.plugin + "/" + .scenario), .name, (.with | fmt), (.without | fmt),
           ((.delta | arrow) + " " + (.delta | pfmt)), .status] | @tsv
    ' "$TMP/report.json" | while IFS=$'\t' read -r scenario criterion w o d s; do
        printf '  %-36s %-34s %7s %7s %8s  %s\n' "$scenario" "$criterion" "$w" "$o" "$d" "$s"
    done

    if [[ "$(jq -r '.totals.failed_runs' "$TMP/report.json")" != "0" ]] ||
        [[ "$(jq -r '.totals.pending_points' "$TMP/report.json")" != "0" ]]; then
        printf '\n%s\n' "$(c '1;31' 'Incomplete runs')"
        jq -r '
            .scenarios[]
            | select(.status != "ok" or .pending_points > 0)
            | "  " + .plugin + "/" + .scenario + " — " + (.pending_points | tostring)
              + " pending point(s), status " + .status
        ' "$TMP/report.json"
    fi
}

# --- write and print ----------------------------------------------------------

WROTE=()

# Always keep the machine-readable document: the Markdown and HTML reports
# link to it, and it is the stable input for another tool.
if ! jq '.' "$TMP/report.json" >"$OUT_DIR/report.json"; then
    die "cannot write report: $OUT_DIR/report.json"
fi
WROTE+=("$OUT_DIR/report.json")
if [[ "$FORMAT" == "md" || "$FORMAT" == "all" ]]; then
    render_md
    if ! cp "$TMP/report.md" "$OUT_DIR/report.md"; then
        die "cannot write report: $OUT_DIR/report.md"
    fi
    WROTE+=("$OUT_DIR/report.md")
fi
if [[ "$FORMAT" == "html" || "$FORMAT" == "all" ]]; then
    render_html
    if ! cp "$TMP/report.html" "$OUT_DIR/report.html"; then
        die "cannot write report: $OUT_DIR/report.html"
    fi
    WROTE+=("$OUT_DIR/report.html")
fi

case "$FORMAT" in
    json)
        jq '.' "$TMP/report.json"
        printf 'Report: %s\n' "$OUT_DIR/report.json" >&2
        ;;
    md)
        render_md
        cat "$TMP/report.md"
        printf 'Report: %s\n' "$OUT_DIR/report.md" >&2
        ;;
    html)
        printf 'Report: %s\n' "$OUT_DIR/report.html"
        ;;
    cli)
        render_cli
        printf '\nReport directory: %s\n' "$OUT_DIR"
        ;;
    all)
        render_cli
        printf '\nReports:\n'
        for file in ${WROTE[@]+"${WROTE[@]}"}; do
            printf '  %s\n' "$file"
        done
        ;;
    *) ;;
esac
