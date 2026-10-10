#!/usr/bin/env bash
# Initialises a git repo with a scaffold commit and then applies uncommitted
# edits to src/ledger.py: redundant comments, an end-of-line comment, a
# commented-out block, and a protected TODO.
set -euo pipefail

git init -q
git config user.email "dev@example.com"
git config user.name "Dev"

git checkout -q -b main
mkdir -p src

cat > src/ledger.py << 'PYEOF'
"""Ledger total helpers for the ledger demo entry point."""


def total(items: list) -> int:
    """Sum the logged amounts of a ledger record."""
    return sum(p["amount"] for p in items)


if __name__ == "__main__":
    entries = [
        {"label": "coffee", "amount": 15},
        {"label": "book", "amount": 95},
        {"label": "monitor", "amount": 120},
    ]
    totalled = sum(p["amount"] for p in entries)
    print(f"total={totalled}")
PYEOF

git add src/ledger.py
git commit -q -m "chore: ledger scaffold"

# Uncommitted edits: this is the change /clean looks at.
cat > src/ledger.py << 'PYEOF'
"""Ledger total helpers for the ledger demo entry point."""


def total(items: list) -> int:
    """Sum the logged amounts of a ledger record."""
    return sum(p["amount"] for p in items)


def taxable(items: list) -> list:
    """Filter rows that are taxable events."""
    # commented out while auditing totals
    # old = [p for p in items if p["label"] != "coffee"]
    # old = [p for p in items if p["amount"] > 50]
    # return old
    # changed the loop to enumerate()
    # removed the taxable-min floor after the audit
    rows = []
    for i, item in enumerate(items):
        # lists the total
        rows.append(item)
    # TODO(auditor): reconcile against ledger-2026-03 in ops sheet.
    return rows


if __name__ == "__main__":
    entries = [
        {"label": "coffee", "amount": 15},
        {"label": "book", "amount": 95},
        {"label": "monitor", "amount": 120},
        {"label": "ink", "amount": 42},   # restock-only items
    ]
    totalled = sum(p["amount"] for p in entries)
    print(f"total={totalled}")
PYEOF

echo "Repo ready: src/ledger.py has uncommitted edits with redundant comments."
