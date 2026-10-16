#!/usr/bin/env bash
# Initialises a git repository containing the audit-log export design
# document that the agent must review with the mentor skill's System
# Design Review framework.
set -euo pipefail

git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> .git/info/exclude
git config user.email "eng@example.com"
git config user.name "Payments Platform"

git checkout -q -b main
mkdir -p docs
cat > docs/design-audit-export.md << 'MDEOF'
# Design: Scheduled Audit Log Export

**Author**: Priya (mid-level engineer, payments platform)
**Status**: Draft for architecture review board

## Problem

Compliance requires a nightly export of all audit events (who changed what
in billing records) to cold storage. The audit_events table in the billing
PostgreSQL database currently holds ~2 million rows and grows by roughly
15k rows per day. The export must be retained for 7 years.

## Proposed Design

The billing service gains an in-process scheduler (a goroutine started at
boot, similar to the existing nightly reconciliation job). Each night at
02:00 UTC it:

1. Runs `SELECT id, actor, action, target, payload, created_at
   FROM audit_events WHERE created_at >= $1 AND created_at < $2` against
   the billing PostgreSQL database (the same one the service uses for
   request handling), with the query window anchored to the previous
   successful run ID stored in a small `export_runs` table.
2. Streams the rows through the standard `encoding/csv` writer into an
   in-memory buffer.
3. Uploads the resulting CSV to the S3 bucket `acme-audit-archive` with
   the key `audit/YYYY/MM/DD/<run-id>.csv`. A run ID (ULID) is generated
   per execution; a row is inserted into `export_runs` before upload, so
   re-running after a crash cannot double-import a window.
4. Posts a success message to the #compliance Slack webhook with the row
   count and run ID.

## Notes

- The query hits the existing `created_at` B-tree index, so no new index
  is needed.
- We reuse the billing service's existing database pool and S3 credentials
  (instance profile).
- Payload sizes are small (usually under 200 bytes per row); at 15k rows
  per day the in-memory buffer is expected to stay under ~10 MB.
- Failures: if the upload errors, the scheduler logs an error. The next
  night's window anchor is derived from the last successful run ID, so the
  failed window is included in the next export.

## What is explicitly out of scope for this document

- Retention/lifecycle rules for the S3 bucket.
- Any monitoring beyond the scheduler's error log line.
MDEOF

git add docs/design-audit-export.md
git commit -q -m "docs: add draft design for nightly audit log export"

echo "Repo ready with docs/design-audit-export.md committed on main."
echo "Agent must write design-review.md in the repository root."
