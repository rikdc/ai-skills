#!/usr/bin/env bash

# Mechanical checks for this scenario
#
# Scores the criteria a script can decide, from the recording the gh shim wrote
# (pr-record.json at the repository root). Everything else in criteria.json -
# whether the body reads well, whether the title is imperative - needs
# judgement and is left to a judge; see the review-evals skill.
#
# Run by bin/evalctl run with the sandbox as the working directory:
#   SANDBOX       the sandbox root
#   SCENARIO_DIR  this scenario's directory
#   CRITERIA      this scenario's criteria.json
#
# Emits a JSON array of {name, score, max, note}. Names and max values must
# match criteria.json; the runner rejects anything else.

set -uo pipefail

record="${SANDBOX:-.}/pr-record.json"
items_file="$(mktemp)"
trap 'rm -f "$items_file"' EXIT

value() {
    [[ -f "$record" ]] || return 0
    jq -r --arg key "$1" '.[$key] // empty' "$record" 2>/dev/null
}

emit() {
    jq -nc --arg name "$1" --argjson score "$2" --argjson max "$3" --arg note "$4" \
        '{name: $name, score: $score, max: $max, note: $note}' >>"$items_file"
}

score() {
    local name="$1" max="$2" ok="$3" good="$4" bad="$5"
    if [[ "$ok" == "1" ]]; then
        emit "$name" "$max" "$max" "$good"
    else
        emit "$name" 0 "$max" "$bad"
    fi
}

draft="$(value draft)"
base="$(value base)"
head="$(value head)"
title="$(value title)"

[[ "$draft" == "true" ]] && ok=1 || ok=0
score "Draft PR created" 8 "$ok" "recorded invocation asked for a draft" "no recording, or not a draft"

[[ "$base" == "main" ]] && ok=1 || ok=0
score "Base branch set" 5 "$ok" "base is main" "base is '${base:-unset}'"

[[ "$head" == "feature/replica-routing" ]] && ok=1 || ok=0
score "PR targets the feature branch" 15 "$ok" "head is the feature branch" "head is '${head:-unset}'"

# Conventional prefix: <type> or <type>(<scope>), optionally marked breaking.
if [[ "$title" =~ ^[a-z]+(\([a-z0-9._-]+\))?!?:[[:space:]] ]]; then ok=1; else ok=0; fi
score "Title is conventional commit format" 12 "$ok" "title starts with a conventional prefix" "title '${title:-unset}' has no conventional prefix"

subject="${title#*: }"
[[ "${subject:0:1}" =~ [a-z0-9] ]] && ok=1 || ok=0
score "Title lower case" 5 "$ok" "subject starts lower-case" "subject starts with '${subject:0:1}'"

[[ -n "$title" && "$title" != *"." ]] && ok=1 || ok=0
score "Title no trailing period" 3 "$ok" "no trailing period" "title ends with a period, or is empty"

(( ${#title} > 0 && ${#title} < 70 )) && ok=1 || ok=0
score "Title under 70 chars" 4 "$ok" "title is ${#title} characters" "title is ${#title} characters"

jq -s 'sort_by(.name)' "$items_file"
