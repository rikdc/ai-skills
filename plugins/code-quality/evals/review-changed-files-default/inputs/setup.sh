#!/usr/bin/env bash
# Initialises a git repo with main and a feature branch whose pending commit
# contains review findings; leaves HEAD on the feature branch for a
# default-scope code review.
set -euo pipefail

git init -q
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


def scrape(job: str) -> dict:
    """Return a metrics snapshot for a scrape job."""
    return {"job": job, "series": []}
PYEOF

cat > docs/API.md << 'MDEOF'
# Metrics API

`scrape(job)` returns a metrics snapshot for a scrape job.
MDEOF

git add -A
git commit -q -m "chore: metrics service scaffold"

git checkout -q -b feature/retry-backoff

cat > src/retry.py << 'PYEOF'
"""Retry helper for scraping flaky upstream targets."""
import time


def attempt(fetch, target):
    """Call fetch(target) until it succeeds."""
    while True:
        try:
            return fetch(target)
        except:  # noqa: E722
            pass
        time.sleep(2)
PYEOF

git add src/retry.py
git commit -q -m "feat(retry): retry failed scrapes until they succeed"

echo "Repo ready on feature/retry-backoff with the retry commit pending review."
