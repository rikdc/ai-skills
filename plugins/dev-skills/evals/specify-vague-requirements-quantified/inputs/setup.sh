#!/usr/bin/env bash
# Initialises a git repository containing Sam's raw product-review notes
# for the media upload service — a deliberately vague "design" the agent
# must turn into a precise, testable specification.
set -euo pipefail

git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> .git/info/exclude
git config user.email "eng@example.com"
git config user.name "Docs Platform"

git checkout -q -b main
mkdir -p docs
cat > docs/media-upload-design.md << 'MDEOF'
# Design: Media Upload Service

**Author**: Sam (docs platform)
**Status**: Raw notes from the 2026-10-06 product review — not yet a spec

## Problem

Users attach screenshots and diagrams to help-centre articles. Today
they host images elsewhere and paste links, which break. We need an
upload service.

## Decided

- Storage: S3-compatible object storage.
- Metadata (owner, article link, size, content type) in Postgres.

## Requirements (verbatim from the product review)

- The upload endpoint should respond quickly, even for large files.
- We should probably support at least a thousand users uploading at the
  same time.
- Very large uploads might fail sometimes — handle those gracefully.
- Files that are too big should be rejected. Priya said "something like
  50 MB?" — unconfirmed.
- Old unused uploads should maybe get cleaned up at some point.
- Users shouldn't upload arbitrary junk; a malware scan came up once but
  nobody committed to it.
- Uploads should be visible only to the uploader (or maybe the whole
  workspace — left open).

## Sketch

- One HTTP endpoint that accepts the file (streaming, not buffered).
- Store bytes in object storage, metadata row in Postgres, return an ID.
- Attach the ID to the article on publish.

## Undecided

- Whether malware scanning is in scope at all.
- Whether uploads are private per user or shared within the workspace.
MDEOF

git add docs/media-upload-design.md
git commit -q -m "docs: add raw product-review notes for media upload service"

echo "Repo ready with docs/media-upload-design.md committed on main."
echo "Agent must write spec.md in the repository root."
