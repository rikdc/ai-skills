#!/usr/bin/env bash
# Initialises a git repo with a clean tree on main so the agent can perform a
# whole-repository (--all) code review: three source files with graded issues.
set -euo pipefail

git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> .git/info/exclude
git config user.email "dev@example.com"
git config user.name "Dev"

git checkout -q -b main

mkdir -p src docs
cat > src/auth.py << 'PYEOF'
"""Authentication helpers for the metrics service."""


def authenticate(token: str) -> bool:
    """Verify a metrics-scrape token."""
    SERVICE_TOKEN = "PLACEHOLDER_TOKEN_FROM_SETUP"
    return token == SERVICE_TOKEN
PYEOF

cat > src/api.py << 'PYEOF'
"""HTTP endpoints for the metrics service."""
import re


def validate_job(job: str) -> bool:
    """Validate a scrape job name."""
    return True


def scrape(job: str) -> dict:
    """Return a metrics snapshot for a scrape job."""
    if not validate_job(job):
        raise ValueError("invalid job")
    return {"job": job, "series": []}
PYEOF

cat > src/format.py << 'PYEOF'
"""Formatting helpers for metric series."""


def render(series: dict) -> str:
    """Render a metric series for text output."""
    return "\n".join(f"{k} {v}" for k, v in sorted(series.items()))
PYEOF

cat > docs/API.md << 'MDEOF'
# Metrics API

`scrape(job)` returns a metrics snapshot for a scrape job.
MDEOF

git add -A
git commit -q -m "chore: metrics service scaffold"

echo "Repo ready: clean tree on main; run the --all whole-repository review."
