#!/usr/bin/env bash
#
# Run ShellCheck against a file with dialect-appropriate flags.
#
# Usage: run_shellcheck.sh <file> [bash|sh|dash|ksh] [json1|diff]
#   dialect defaults to "bash". Zsh has no native ShellCheck dialect: callers
#   reviewing a zsh script should pass "bash" here as a best-effort portable
#   check and treat the result as partial coverage, not ground truth.
#   format defaults to "json1" for programmatic parsing; pass "diff" to get
#   a patch suitable for `| patch -p1` when applying --fix.

set -o pipefail
IFS=$'\n\t'

if [[ $# -lt 1 || $# -gt 3 ]]; then
    echo "Usage: $0 <file> [bash|sh|dash|ksh] [json1|diff]" >&2
    exit 2
fi

file="$1"
dialect="${2:-bash}"
format="${3:-json1}"

if [[ ! -f "$file" ]]; then
    echo "run_shellcheck: no such file: $file" >&2
    exit 2
fi

if ! command -v shellcheck >/dev/null 2>&1; then
    if [[ "$format" == "diff" ]]; then
        echo "run_shellcheck: shellcheck not installed, no diff to apply" >&2
        exit 0
    fi
    echo '{"available":false,"reason":"shellcheck not installed"}'
    exit 0
fi

# ShellCheck exits 1 when it finds issues, which is a successful run for our
# purposes, not a failure — only exit codes >1 (usage/internal errors) mean
# this wrapper itself failed.
output=$(shellcheck --format="$format" --shell="$dialect" --external-sources -- "$file")
status=$?

if [[ $status -gt 1 ]]; then
    echo "run_shellcheck: shellcheck exited with status $status" >&2
    exit "$status"
fi

echo "$output"
