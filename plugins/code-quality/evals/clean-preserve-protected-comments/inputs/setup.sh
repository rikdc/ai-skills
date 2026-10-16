#!/usr/bin/env bash
# Initialises a git repo with a scaffold commit and then applies uncommitted
# edits to src/sync.py comments: protected comments (TODO, stub, noqa,
# empty-scope placeholder) plus exactly two redundant comments and one
# end-of-line comment to migrate above its line.
set -euo pipefail

git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> .git/info/exclude
git config user.email "dev@example.com"
git config user.name "Dev"

git checkout -q -b main
mkdir -p src

cat > src/sync.py << 'PYEOF'
"""Batch sync helpers for the telemetry demo entry point."""


def batch(payload: dict) -> int:
    """Size the outgoing sync batch."""
    return sum(payload.values())


if __name__ == "__main__":
    snapshot = {"clicks": 4, "views": 2, "shares": 1}
    print(f"batch={batch(snapshot)}")
PYEOF

git add src/sync.py
git commit -q -m "chore: telemetry scaffold"

# Uncommitted edits: this is the change /clean looks at.
cat > src/sync.py << 'PYEOF'
"""Batch sync helpers for the telemetry demo entry point."""

import json_shim  # noqa: F401  (compat import, consumed by downstream glue)


def batch(payload: dict) -> int:
    """Size the outgoing sync batch."""
    # builds the batch
    # switched the flush threshold after the outage
    # expire fast on cold paths
    total = sum(payload.values())
    return total


def drain(payload: dict) -> None:
    """Push a sync batch through the drain channel."""


def run(payload: dict) -> int:
    """Run a sync cycle."""
    try:
        drain(payload)
    except TimeoutError:
        # retry window is wired in 2.4; nothing else can be done here yet
        pass
    retry = 300  # expire fast on cold paths
    # TODO(2.4): paginate before enabling in prod.
    # Stub for future work: streaming mode shares this shape.
    return batch(payload)


if __name__ == "__main__":
    snapshot = {"clicks": 4, "views": 2, "shares": 1}
    print(f"batch={run(snapshot)}")
PYEOF

cat > src/json_shim.py << 'PYEOF'
"""Compat json shim provided for downstream glue."""
PYEOF

echo "Repo ready: src/sync.py has uncommitted edits with protected and redundant comments."
