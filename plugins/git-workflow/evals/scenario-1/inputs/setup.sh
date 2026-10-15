#!/usr/bin/env bash
# Initialise a small git repository that simulates an in-progress feature branch
# with three separate unstaged changes ready to be committed.
set -euo pipefail

# Bootstrap an initial commit so we have a history to build on.
git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> .git/info/exclude
git config user.email "dev@acme.example.com"
git config user.name "Acme Dev"

# ── Commit 0: project skeleton (the "before" state) ──────────────────────────
mkdir -p src docs

cat > src/parser.py << 'PYEOF'
"""CSV record parser for the Acme data pipeline."""

import csv
import io
from typing import Iterator


def parse_records(raw: str) -> Iterator[dict]:
    """Yield parsed dicts from a CSV string."""
    reader = csv.DictReader(io.StringIO(raw))
    for row in reader:
        yield dict(row)
PYEOF

cat > docs/README.md << 'MDEOF'
# Acme Data Pipeline

A lightweight ETL tool for processing CSV exports from Acme's CRM.

## Overview

The pipeline ingests CSV files produced by the CRM, validates and transforms
each record, and loads the results into the data warehouse.

## Modules

- **`src/parser.py`** — Reads raw CSV input and yields cleaned record dicts.

## Getting Started

```bash
pip install -r requirements.txt
python -m acme.pipeline --input data/export.csv
```
MDEOF

git add src/parser.py docs/README.md
git commit -q -m "chore: initial project skeleton"

# ── Working-tree changes (unstaged) ──────────────────────────────────────────

# Change 1 — new feature: welcome-email module
cat > src/feature.py << 'PYEOF'
"""User notification module."""

import smtplib
from email.message import EmailMessage


def send_welcome_email(user_email: str, user_name: str) -> bool:
    """Send a welcome email to a newly registered user."""
    msg = EmailMessage()
    msg["Subject"] = f"Welcome to Acme, {user_name}!"
    msg["From"] = "noreply@acme.example.com"
    msg["To"] = user_email
    msg.set_content(
        f"Hi {user_name},\n\n"
        "Thank you for signing up. Your account is ready.\n\n"
        "— The Acme Team"
    )
    try:
        with smtplib.SMTP("localhost") as server:
            server.send_message(msg)
        return True
    except Exception:
        return False
PYEOF

# Change 2 — parser: strip whitespace from parsed values
cat > src/parser.py << 'PYEOF'
"""CSV record parser for the Acme data pipeline."""

import csv
import io
from typing import Iterator


def parse_records(raw: str) -> Iterator[dict]:
    """Yield parsed dicts from a CSV string."""
    reader = csv.DictReader(io.StringIO(raw))
    for row in reader:
        # Strip whitespace from every value to avoid downstream mismatches.
        yield {k: v.strip() for k, v in row.items()}
PYEOF

# Change 3 — docs update: expand README with config table and contributing guide
cat > docs/README.md << 'MDEOF'
# Acme Data Pipeline

A lightweight ETL tool for processing CSV exports from Acme's CRM.

## Overview

The pipeline ingests CSV files produced by the CRM, validates and transforms
each record, and loads the results into the data warehouse.

## Modules

- **`src/parser.py`** — Reads raw CSV input and yields cleaned record dicts.
- **`src/feature.py`** — Sends transactional emails (e.g., welcome messages).

## Getting Started

```bash
pip install -r requirements.txt
python -m acme.pipeline --input data/export.csv
```

## Configuration

Set the following environment variables before running:

| Variable         | Description                          | Default               |
|------------------|--------------------------------------|-----------------------|
| `SMTP_HOST`      | Hostname of the outbound SMTP server | `localhost`           |
| `SMTP_PORT`      | Port for SMTP                        | `25`                  |
| `FROM_ADDRESS`   | Sender address for system emails     | `noreply@acme.example.com` |

## Contributing

1. Fork the repository and create a feature branch.
2. Run `make checks` before opening a pull request.
3. Follow the project's commit message conventions.
MDEOF

echo "Git repository initialised. Three unstaged changes are ready:"
git status --short
