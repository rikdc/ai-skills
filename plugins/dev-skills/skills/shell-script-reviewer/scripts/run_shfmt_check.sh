#!/usr/bin/env bash
#
# Report formatting drift for a Bash/POSIX script via `shfmt -d`, so the
# reviewing skill can cite a concrete diff instead of prose about style.
# Flags match this skill's adopted house style: 2-space indent, indented
# switch cases, binary operators may start a new line, space after
# redirect operators. Not applicable to Fish (no shfmt support) or Zsh
# (shfmt targets bash/POSIX/mksh only and will misreport Zsh-only syntax
# as drift).
#
# Usage: run_shfmt_check.sh <file>

set -o pipefail
IFS=$'\n\t'

if [[ $# -ne 1 ]]; then
    echo "Usage: $0 <file>" >&2
    exit 2
fi

file="$1"

if [[ ! -f "$file" ]]; then
    echo "run_shfmt_check: no such file: $file" >&2
    exit 2
fi

if ! command -v shfmt >/dev/null 2>&1; then
    echo "shfmt not installed: skipping formatting check" >&2
    exit 0
fi

# shfmt -d exits 1 when it produces a diff (formatting drift found) — that is
# a successful check, not a wrapper failure. Only exit codes >1 indicate
# shfmt itself errored (e.g. parse failure).
diff_output=$(shfmt -d -i 2 -ci -bn -sr -- "$file")
status=$?

if [[ $status -gt 1 ]]; then
    echo "run_shfmt_check: shfmt exited with status $status" >&2
    exit "$status"
fi

if [[ -z "$diff_output" ]]; then
    echo "no formatting drift"
else
    echo "$diff_output"
fi
