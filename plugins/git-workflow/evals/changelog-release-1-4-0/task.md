# Update the Changelog and Cut a Release

## Background

The relay library repository has accumulated several unreleased fixes. The
team records every user-facing change in `CHANGELOG.md` following the Keep a
Changelog format and Semantic Versioning. The current changelog has an
`## [Unreleased]` section with three entries already recorded.

The repository lives in the working directory. Run the fixture setup script
(`bash inputs/setup.sh`, or `bash setup.sh` if it sits at the working-directory
root) to create it; the changelog will be present with the three unreleased
entries.

## Your Task

1. Record one more unreleased entry: `Fixed` — the socket connector no longer
   retries authentication failures, which previously caused a duplicate-login
   loop.

2. Then cut the release as version `1.4.0`.

## Output Specification

- `CHANGELOG.md` reflecting both actions
- `CHANGELOG.md` is untouched otherwise — existing entry wording and
  category order preserved
