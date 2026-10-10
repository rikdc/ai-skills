#!/usr/bin/env bash
# Initialises a git repo with no CHANGELOG.md so the agent can create one
# from scratch following Keep a Changelog conventions.
set -euo pipefail

git init -q
git config user.email "dev@example.com"
git config user.name "Dev"

mkdir -p src
cat > src/capture.py << 'PYEOF'
"""Snapshot capture module."""


def capture(target: str) -> str:
    """Capture a snapshot of target and return the snapshot id."""
    return f"snapshot-{target}"
PYEOF
git add src/capture.py
git commit -q -m "chore: initial scaffold"

echo "Repo ready; no CHANGELOG.md exists yet."
