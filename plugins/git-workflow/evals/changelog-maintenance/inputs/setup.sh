#!/usr/bin/env bash
# Initialises a git repo containing a CHANGELOG.md with an Unreleased
# section (Added + Fixed) and a prior 2.1.0 release, so the agent can
# perform the implied maintenance cut.
set -euo pipefail

git init -q
git config user.email "dev@example.com"
git config user.name "Dev"

cat > CHANGELOG.md << 'MDEOF'
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Add CSV export for audit summaries

### Fixed

- Align summary totals with source ledger

## [2.1.0] - 2026-09-12

### Fixed

- Restore zero-downtime reload after schema bump
MDEOF

git add CHANGELOG.md
git commit -q -m "chore: changelog state for 2.1.1 cut"

echo "Repo ready; CHANGELOG.md has Unreleased entries and a prior 2.1.0 release."
