#!/usr/bin/env bash
#
# Detect the shell dialect (bash, zsh, fish, sh/POSIX, or unknown) for a
# script file, based on its shebang first and its extension as a fallback.
#
# Usage: detect_shell.sh <file>
# Output: one word on stdout — bash | zsh | fish | sh | unknown

set -o pipefail
IFS=$'\n\t'

if [[ $# -ne 1 ]]; then
    echo "Usage: $0 <file>" >&2
    exit 2
fi

file="$1"

if [[ ! -f "$file" ]]; then
    echo "detect_shell: no such file: $file" >&2
    exit 2
fi

shebang=$(head -n 1 -- "$file" 2>/dev/null || true)

case "$shebang" in
    '#!'*bash*) echo "bash"; exit 0 ;;
    '#!'*zsh*) echo "zsh"; exit 0 ;;
    '#!'*fish*) echo "fish"; exit 0 ;;
    '#!'*/env\ sh* | '#!'*/bin/sh) echo "sh"; exit 0 ;;
    *) ;;
esac

case "$file" in
    *.bash) echo "bash"; exit 0 ;;
    *.zsh) echo "zsh"; exit 0 ;;
    *.fish) echo "fish"; exit 0 ;;
    *.sh)
        # .sh with no shebang match: fall through to content sniffing below
        # rather than guessing, since .sh is used for both bash and POSIX sh.
        ;;
    *) ;;
esac

# No conclusive shebang or extension: sniff for dialect-specific syntax that
# only that shell accepts, so a plain `.sh` file with bash-only features
# (arrays, [[, local -n) isn't misreported as POSIX sh.
if grep -qE '(\[\[|^[[:space:]]*local -n|\bmapfile\b|\breadarray\b|\$\{[A-Za-z_][A-Za-z0-9_]*\[@\]\})' -- "$file" 2>/dev/null; then
    echo "bash"
    exit 0
fi

if grep -qE '^[[:space:]]*(function [A-Za-z_][A-Za-z0-9_]* )?[A-Za-z_][A-Za-z0-9_]*[[:space:]]*$' -- "$file" 2>/dev/null \
    && grep -qE '^[[:space:]]*end[[:space:]]*$' -- "$file" 2>/dev/null; then
    echo "fish"
    exit 0
fi

echo "unknown"
