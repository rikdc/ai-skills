#!/usr/bin/env bash
# Initialises a repo whose changeset is empty: single scaffold commit, clean
# tree, no diverged branch — so /dev:review's default scope finds nothing.
set -euo pipefail

git init -q
git config user.email "dev@example.com"
git config user.name "Dev"

git checkout -q -b main

mkdir -p src
cat > src/invoice.py << 'PYEOF'
"""Invoice helpers for the billing service."""


def total(items: list) -> int:
    """Sum the logged amounts of an invoice."""
    return sum(p["amount"] for p in items)
PYEOF

git add -A
git commit -q -m "chore: billing service scaffold"

echo "Repo ready: working tree clean, single scaffold commit, no pending changes."
