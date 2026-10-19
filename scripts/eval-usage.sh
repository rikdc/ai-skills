#!/usr/bin/env bash

# Normalize one eval invocation's token and cost usage.
#
# This is a read-only parser plus an optional hook. It never calls a model or
# a network: it reads an agent or judge log and writes one JSON object.
#
# Built-in extraction, in order:
#   1. opencode --format json JSONL: sum step_finish part.tokens and part.cost
#   2. Claude --output-format json: the last type:"result" object
#   3. anything else: unknown
#
# A --cmd hook runs after the invocation and may write USAGE_FILE or print a
# normalized object on stdout. A valid hook result overrides the built-ins; a
# failing or invalid hook is reported on stderr and ignored.
#
# Unknown usage never becomes zero: the output is {"known":false,"source":"none"}.
#
# Usage:
#   scripts/eval-usage.sh --log FILE [--output FILE] [--cmd CMD] \
#       [--sandbox DIR] [--run-dir DIR] [--arm ARM] [--run N] [--agent-exit N]

set -uo pipefail

LOG=""
OUTPUT=""
USAGE_CMD=""
SANDBOX=""
RUN_DIR=""
ARM=""
RUN=""
AGENT_EXIT=""
HOOK_FILE=""
HOOK_ERR=""

usage() {
    cat <<'EOF'
Normalize token and cost usage from one eval invocation.

Usage:
  scripts/eval-usage.sh --log FILE [options]

Options:
  --log FILE        agent.log or judge.log to parse (required)
  --output FILE     write the JSON object here instead of stdout
  --cmd CMD         optional post-run usage hook
  --sandbox DIR     hook working directory when it still exists
  --run-dir DIR     run evidence directory (hook fallback cwd)
  --arm ARM         with or without (passed to the hook)
  --run N           run number (passed to the hook)
  --agent-exit N    agent/judge exit status (passed to the hook)

The hook runs with USAGE_FILE, AGENT_LOG, RUN_DIR, SANDBOX, ARM, RUN and
AGENT_EXIT in its environment. It may write USAGE_FILE or print a normalized
JSON object on stdout. Unknown usage is reported as known:false, never as 0.
EOF
}

die() {
    echo "eval-usage: $*" >&2
    exit 2
}

cleanup() {
    [[ -n "$HOOK_FILE" ]] && rm -f "$HOOK_FILE"
    [[ -n "$HOOK_ERR" ]] && rm -f "$HOOK_ERR"
}

trap cleanup EXIT

while [[ $# -gt 0 ]]; do
    case "$1" in
        --log | --output | --cmd | --sandbox | --run-dir | --arm | --run | --agent-exit)
            if [[ $# -lt 2 || -z "$2" ]]; then
                echo "eval-usage: $1 needs a value" >&2
                usage >&2
                exit 2
            fi
            case "$1" in
                --log) LOG="$2" ;;
                --output) OUTPUT="$2" ;;
                --cmd) USAGE_CMD="$2" ;;
                --sandbox) SANDBOX="$2" ;;
                --run-dir) RUN_DIR="$2" ;;
                --arm) ARM="$2" ;;
                --run) RUN="$2" ;;
                --agent-exit) AGENT_EXIT="$2" ;;
                *) ;;
            esac
            shift 2
            ;;
        -h | --help)
            usage
            exit 0
            ;;
        *)
            echo "eval-usage: unknown argument: $1" >&2
            usage >&2
            exit 2
            ;;
    esac
done

[[ -n "$LOG" ]] || die "--log is required"

# Normalize any object that has at least one usable usage field. Missing or
# invalid fields are dropped, never coerced to 0.
normalize_usage() {
    jq -c '
        def clean: if type == "number" and . >= 0 then . else null end;
        . as $in
        | ($in.source // "hook") as $source
        | {
            input_tokens: ($in.input_tokens | clean),
            output_tokens: ($in.output_tokens | clean),
            reasoning_tokens: ($in.reasoning_tokens | clean),
            cache_read_tokens: ($in.cache_read_tokens | clean),
            cache_write_tokens: ($in.cache_write_tokens | clean),
            total_tokens: ($in.total_tokens | clean),
            cost_usd: ($in.cost_usd | clean)
        }
        | with_entries(select(.value != null))
        | . as $fields
        | (if ($fields | has("total_tokens")) then $fields
           else
               ([$fields.input_tokens, $fields.output_tokens,
                 $fields.cache_read_tokens, $fields.cache_write_tokens,
                 (if $source == "claude-json" then null else $fields.reasoning_tokens end)]
                | map(select(. != null)) | add // null) as $sum
               | if $sum != null then $fields + {total_tokens: $sum} else $fields end
           end) as $normalized
        | if ($normalized | length) == 0 then
              {known: false, source: "none"}
          else
              $normalized + {
                  known: true,
                  source: (if ($source | type) == "string" and $source != ""
                           then $source else "hook" end)
              }
              + (if ($normalized | has("cost_usd")) then {currency: "USD"} else {} end)
          end
    '
}

opencode_usage() {
    jq -R -s '
        [split("\n")[] | fromjson? | select(type == "object" and .type == "step_finish")]
        | . as $events
        | def values($path):
            [$events[] | getpath($path) | select(type == "number" and . >= 0)];
          def sum_or_null($values): if ($values | length) > 0 then ($values | add) else null end;
          {
              source: "opencode-json",
              input_tokens: sum_or_null(values(["part", "tokens", "input"])),
              output_tokens: sum_or_null(values(["part", "tokens", "output"])),
              reasoning_tokens: sum_or_null(values(["part", "tokens", "reasoning"])),
              cache_read_tokens: sum_or_null(values(["part", "tokens", "cache", "read"])),
              cache_write_tokens: sum_or_null(values(["part", "tokens", "cache", "write"])),
              total_tokens: sum_or_null(values(["part", "tokens", "total"])),
              cost_usd: sum_or_null(values(["part", "cost"]))
          }
    ' "$1" | normalize_usage
}

claude_usage() {
    jq -R -s '
        [split("\n")[] | fromjson? | select(type == "object" and .type == "result")]
        | last
        | if . == null then empty
          else
              {
                  source: "claude-json",
                  input_tokens: (.usage.input_tokens // null),
                  output_tokens: (.usage.output_tokens // null),
                  reasoning_tokens: (.usage.output_tokens_details.thinking_tokens
                                     // .output_tokens_details.thinking_tokens // null),
                  cache_read_tokens: (.usage.cache_read_input_tokens // null),
                  cache_write_tokens: (.usage.cache_creation_input_tokens // null),
                  total_tokens: (.usage.total_tokens // null),
                  cost_usd: (.total_cost_usd // null)
              }
          end
    ' "$1" | normalize_usage
}

unknown_usage() {
    printf '{"known":false,"source":"none"}\n'
}

hook_output_valid() {
    local json="$1"
    printf '%s' "$json" | jq -s -e '
        length == 1
        and (.[0] | type == "object")
        and (
            (.[0].known | type) == "boolean"
            or ([.[0].input_tokens, .[0].output_tokens, .[0].reasoning_tokens,
                 .[0].cache_read_tokens, .[0].cache_write_tokens,
                 .[0].total_tokens, .[0].cost_usd]
                | any(type == "number" and . >= 0))
        )
    ' >/dev/null 2>&1
}

run_hook() {
    [[ -n "$USAGE_CMD" ]] || return 1

    local cwd="${RUN_DIR:-.}"
    [[ -n "$SANDBOX" ]] && cwd="$SANDBOX"

    HOOK_FILE="$(mktemp "${TMPDIR:-/tmp}/eval-usage-hook-XXXXXX")" || {
        echo "eval-usage: cannot create a hook output file; falling back to built-ins" >&2
        return 1
    }
    HOOK_ERR="$(mktemp "${TMPDIR:-/tmp}/eval-usage-hook-err-XXXXXX")" || {
        echo "eval-usage: cannot create a hook error file; falling back to built-ins" >&2
        return 1
    }

    local stdout rc
    stdout="$(cd "$cwd" 2>/dev/null &&
        USAGE_FILE="$HOOK_FILE" AGENT_LOG="$LOG" RUN_DIR="$RUN_DIR" \
            SANDBOX="$SANDBOX" ARM="$ARM" RUN="$RUN" AGENT_EXIT="$AGENT_EXIT" \
            bash -c "$USAGE_CMD" 2>"$HOOK_ERR")"
    rc=$?
    if ((rc != 0)); then
        echo "eval-usage: usage command failed (exit $rc); falling back to built-in extraction" >&2
        return 1
    fi

    local candidate=""
    if [[ -s "$HOOK_FILE" ]]; then
        candidate="$(cat "$HOOK_FILE" 2>/dev/null)"
    fi
    if [[ -z "$candidate" && -n "$stdout" ]]; then
        candidate="$stdout"
    fi
    if [[ -n "$candidate" ]] && hook_output_valid "$candidate"; then
        printf '%s' "$candidate" | normalize_usage
        return 0
    fi

    echo "eval-usage: usage command produced no valid JSON; falling back to built-in extraction" >&2
    return 1
}

usage=""
if [[ -f "$LOG" ]]; then
    if hook="$(run_hook)"; then
        usage="$hook"
    else
        usage="$(opencode_usage "$LOG")"
        if [[ "$(printf '%s' "$usage" | jq -r '.known' 2>/dev/null)" != "true" ]]; then
            claude="$(claude_usage "$LOG")"
            if [[ "$(printf '%s' "$claude" | jq -r '.known' 2>/dev/null)" == "true" ]]; then
                usage="$claude"
            fi
        fi
    fi
fi
[[ -n "$usage" ]] || usage="$(unknown_usage)"

if [[ -n "$OUTPUT" ]]; then
    printf '%s\n' "$usage" >"$OUTPUT" || {
        echo "eval-usage: cannot write usage file: $OUTPUT" >&2
        exit 1
    }
else
    printf '%s\n' "$usage"
fi
