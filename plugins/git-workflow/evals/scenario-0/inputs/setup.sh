#!/bin/bash
# Initialize the git repo and stage the bug fix for the agent to commit.
set -e

git init
git config user.email "dev@example.com"
git config user.name "Dev"

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
git commit -m "chore: initial project scaffold"

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
