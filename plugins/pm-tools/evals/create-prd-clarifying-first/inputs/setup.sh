#!/usr/bin/env bash
# Empty workspace: the user has delivered only a one-line feature prompt and
# no answers, so the PRD step cannot legitimately start yet.
set -euo pipefail

git init -q
git config user.email "dev@example.com"
git config user.name "Dev"

echo "ledger-sync workspace ready — no /tasks directory, no prior answers."
