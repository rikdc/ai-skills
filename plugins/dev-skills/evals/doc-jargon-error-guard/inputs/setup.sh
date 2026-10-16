#!/usr/bin/env bash
# Initialises the ingest repository: source with public functions and real
# failure modes, a jargon-heavy existing doc, and a stale legacy doc that
# must be marked obsolete. The agent rewrites the guide and deprecates the
# legacy doc.
set -euo pipefail

git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> .git/info/exclude
git config user.email "dev@example.com"
git config user.name "Dev"

git checkout -q -b main

cat > README.md << 'MDEOF'
# ingest

Record ingestion service for event streams.

- `src/ingest.py` — the ingest module
- `docs/ingest-api.md` — current module guide
- `docs/legacy-ingest.md` — historical (nightly batch pipeline)
MDEOF

mkdir -p src docs
cat > src/ingest.py << 'PYEOF'
"""Ingest module.

Public API:
- ingest_batch(records, *, batch_id): idempotent upsert of a batch of
  records into the record store through the WAL sink.
- purge(older_than): remove records older than the given timestamp.
"""

MAX_BATCH = 5000
MAX_INFLIGHT = 64


class DuplicateBatchError(Exception):
    """Raised when a batch_id has already been ingested successfully."""


_seen_batches: set = set()


def ingest_batch(records, *, batch_id: str) -> int:
    """Upsert a batch of records into the record store via the WAL sink.

    The batch_id makes ingestion idempotent: replaying the same batch is a
    no-op. Raises DuplicateBatchError if batch_id was already ingested.
    Backpressure applies when more than MAX_INFLIGHT batches are in
    flight; callers then block until capacity frees up.
    """
    if len(records) > MAX_BATCH:
        raise ValueError(f"batch exceeds {MAX_BATCH} records")
    if batch_id in _seen_batches:
        raise DuplicateBatchError(f"batch {batch_id} already ingested")
    _seen_batches.add(batch_id)
    return len(records)


def purge(older_than) -> int:
    """Delete records older than `older_than`.

    On a store failure mid-purge, already-deleted records stay deleted and
    the function raises; the next run continues from where it stopped.
    """
    return 0
PYEOF

cat > docs/ingest-api.md << 'MDEOF'
# Ingest API

The ingest module utilises idempotent upsert semantics with
backpressure-mediated flow control via the WAL sink to achieve
at-least-once delivery characteristics into the record store.

`ingest_batch` accepts a records iterable and performs the upsert.
`purge` performs retention-based record garbage collection.

Batch operations are deduplicated by batch correlation identifier.
MDEOF

cat > docs/legacy-ingest.md << 'MDEOF'
# Legacy nightly batch ingest

Describes the nightly batch pipeline (removed in v0.9). Run
`nightly-batch --window 24h` to kick off a batch load, then verify with
`batch-report --date yesterday`.
MDEOF

git add -A
git commit -q -m "chore: ingest module with current and legacy docs"

echo "Repo ready on main: src/ingest.py, docs/ingest-api.md, docs/legacy-ingest.md."
