#!/usr/bin/env bash
# Empty workspace: the user has delivered only a one-line feature prompt and
# no answers, so the PRD step cannot legitimately start yet.
set -euo pipefail

git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> .git/info/exclude
git config user.email "dev@example.com"
git config user.name "Dev"

echo "ledger-sync workspace ready — no /tasks directory, no prior answers."
