#!/usr/bin/env bash
# Initialises a git repo containing a CHANGELOG.md in Keep a Changelog
# format with three unreleased entries, ready for an --add-entry and
# --release workflow.
set -euo pipefail

git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\nsetup_repo.sh\n' >> .git/info/exclude
git config user.email "dev@example.com"
git config user.name "Dev"

cat > CHANGELOG.md << 'MDEOF'
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Add TCP preload path for warm connections

### Fixed

- Fix keepalive drops between idle periods in the socket connector
- Fix CSV parser mishandling of quoted fields with embedded newlines
MDEOF

git add CHANGELOG.md
git commit -q -m "chore: add changelog"

echo "Repo ready; CHANGELOG.md has three unreleased entries."
