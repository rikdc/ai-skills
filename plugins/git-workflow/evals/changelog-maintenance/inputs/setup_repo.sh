#!/usr/bin/env bash
# Initialises a local git repo with one staged rename change.
set -euo pipefail

git init repo
cd repo

git config user.email "dev@example.com"
git config user.name "Dev"

# Create the original file and make an initial commit
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
git commit -m "initial commit"

# Apply the breaking rename: parse_record -> read_record
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
# Staged and ready — do NOT commit here; the agent must commit it.
echo "Repo initialised. One change is staged and ready to commit."
