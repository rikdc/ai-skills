#!/usr/bin/env bash
# Initialise the git repo with the bug fix present but unstaged.
set -euo pipefail

git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> .git/info/exclude
git config user.email "dev@example.com"
git config user.name "Dev"

# The harness may copy this fixture as-is or flatten it into the working
# directory; make sure the Makefile the checks target sits at the root.
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
[ -f Makefile ] || cp "$here/Makefile" Makefile

# Create the original broken version and commit it
cat > calculator.py << 'PYEOF'
def divide(a, b):
    return a / b


def add(a, b):
    return a + b


def subtract(a, b):
    return a - b


def multiply(a, b):
    return a * b
PYEOF

git add calculator.py Makefile
git commit -q -m "chore: initial project scaffold"

# Now apply the bug fix (add zero-division guard) but leave it unstaged
cat > calculator.py << 'PYEOF'
def divide(a, b):
    if b == 0:
        return None
    return a / b


def add(a, b):
    return a + b


def subtract(a, b):
    return a - b


def multiply(a, b):
    return a * b
PYEOF

echo "Repository ready. calculator.py has an unstaged bug fix."
