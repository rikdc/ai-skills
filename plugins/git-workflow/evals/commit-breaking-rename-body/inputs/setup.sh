#!/usr/bin/env bash
# Initialises a git repository in repo/ with one staged change ready to
# commit: the parse_record -> read_record rename that migrates the reader
# from the end-of-life legacy-parser package to csv_io.
set -euo pipefail

mkdir -p repo
cd repo

git init -q
git config user.email "dev@example.com"
git config user.name "Dev"

# -- Commit 0: original reader module (imports legacy-parser) ----------------
mkdir -p src
cat > src/reader.py << 'PYEOF'
"""Data record reader module."""

import legacy_parser


def parse_record(data: dict) -> dict:
    """Parse a raw record dict and return a normalised record."""
    return legacy_parser.parse(data)


def validate_record(record: dict) -> bool:
    """Return True if the record passes schema validation."""
    required_fields = {"id", "timestamp", "payload"}
    return required_fields.issubset(record.keys())
PYEOF

git add src/reader.py
git commit -q -m "chore: initial reader module"

# -- Apply the breaking rename and stage it ----------------------------------
cat > src/reader.py << 'PYEOF'
"""Data record reader module."""

import csv_io


def read_record(data: dict) -> dict:
    """Read a raw record dict and return a normalised record."""
    return csv_io.read(data)


def validate_record(record: dict) -> bool:
    """Return True if the record passes schema validation."""
    required_fields = {"id", "timestamp", "payload"}
    return required_fields.issubset(record.keys())
PYEOF

git add src/reader.py

echo "Repository ready: the reader upgrade is staged for commit in repo/."
echo "Do NOT commit here; the agent must commit the staged change."
