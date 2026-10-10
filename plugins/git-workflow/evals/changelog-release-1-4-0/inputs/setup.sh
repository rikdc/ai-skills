#!/usr/bin/env bash
# Initialises a git repo containing a CHANGELOG.md in Keep a Changelog
# format with three unreleased entries, ready for an --add-entry and
# --release workflow.
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

- TCP preload path for warm connections

### Fixed

- Socket connector drops keepalive between idle periods
- CSV parser mishandles quoted fields containing embedded newlines
MDEOF

git add CHANGELOG.md
git commit -q -m "chore: add changelog"

echo "Repo ready; CHANGELOG.md has three unreleased entries."
