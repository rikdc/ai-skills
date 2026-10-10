#!/usr/bin/env bash
# Initialises the svc-notifications repository: the current session cache
# implementation, an existing ADR showing the house template, and an
# engineering memo with the candidate cache options. The agent works in
# this directory; the deliverable is a new ADR.
set -euo pipefail

git init -q
git config user.email "dev@example.com"
git config user.name "Dev"

git checkout -q -b main

cat > README.md << 'MDEOF'
# svc-notifications

A small web service that sends email and push notifications for the
platform.

## Deployment

- Runs as **three instances** behind a load balancer (one per availability
  zone). Any instance can receive any request.
- Data store is PostgreSQL (see `docs/adr/ADR-001-use-postgres-for-primary-store.md`).
- Sessions are currently cached **in-process** to keep request latency low.

## Local development

```bash
pip install -r requirements.txt
uvicorn app.main:app --reload
```
MDEOF

mkdir -p docs/adr notes src/app
cat > docs/adr/ADR-001-use-postgres-for-primary-store.md << 'MDEOF'
# ADR-001: Use PostgreSQL for Primary Data Store

**Status**: Accepted
**Date**: 2024-03-14
**Deciders**: Platform Team

## Context and Problem Statement

The service needs a primary data store for users, notification templates
and delivery records.

## Decision Drivers

- **Data integrity**: delivery records must not be lost
- **Query complexity**: analytics needs joins and aggregations
- **Team expertise**: the team knows PostgreSQL well

## Decision Outcome

**Chosen option**: PostgreSQL (managed instance).

### Consequences

**Positive**:

- Strong consistency for delivery records
- Rich querying for analytics

**Negative**:

- Connection pool sizing becomes part of scaling work

## Related decisions

- None yet.
MDEOF

cat > src/app/session_cache.py << 'PYEOF'
"""In-process cache for session state.

Caveats:
- Each worker process has its own cache instance; entries are not
  shared between processes or instances.
- The cache is empty after a process restart, so cached sessions do not
  survive restarts or deploys.
"""

_cache: dict = {}


def get(key: str):
    """Return a cached session or None."""
    return _cache.get(key)


def put(key: str, value, ttl: int = 300) -> None:
    """Cache a session entry for ttl seconds."""
    # ttl is ignored in this placeholder implementation
    _cache[key] = value
PYEOF

cat > notes/cache-options.md << 'MDEOF'
# Memo: session cache backend candidates

From: platform-eng
Date: 2026-10-08

Two candidates are on the table for caching session state. Pick one.

## Option A — Managed Redis

- Shared across all instances; writes from one instance are visible to
  the others.
- Entries survive instance restarts and deploys.
- Requires new infrastructure: a managed Redis instance, connection
  credentials, and an ops runbook entry.
- Adds one network hop (~1-2 ms in-region) on cache reads.

## Option B — In-process LRU

- No new infrastructure; drop-in change to `session_cache.py`.
- State is per-process: not visible across instances.
- Cache is lost on every restart and deploy.
- Zero added latency.
MDEOF

cat > requirements.txt << 'EOF'
fastapi==0.115.0
uvicorn==0.30.0
EOF

git add -A
git commit -q -m "chore: add session cache, ADR-001 template and cache options memo"

echo "Repo ready on main: README, src/app/session_cache.py, docs/adr/ADR-001, notes/cache-options.md."
