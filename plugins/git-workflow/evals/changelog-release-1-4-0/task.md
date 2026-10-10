# Update the Changelog and Cut a Release

## Background

The relay library repository has accumulated several unreleased fixes. The
team records every user-facing change in `CHANGELOG.md` following the Keep a
Changelog format and Semantic Versioning. The current changelog has an
`## [Unreleased]` section with three entries already recorded.

The repository lives in the working directory. Run `bash setup.sh` to create
it; the changelog will be present with the three unreleased entries.

Your job: record one more unreleased entry and then cut a release.

The new entry: `Fixed` — the socket connector no longer retries
authentication failures, which previously caused a duplicate-login loop.
Use the same style as the existing entries.

Then release version `1.4.0`. The release step must follow the team's
process: verify there are unreleased entries to release, verify the version
is not already used and is higher than the last release, rename the
Unreleased section with today's date, and leave a fresh empty Unreleased
section at the top. Do not tag, commit, or push.

## Output Specification

- `CHANGELOG.md` with the new `Fixed` entry, a `## [1.4.0] - <today>` section
  containing all four entries, and a fresh empty `## [Unreleased]` above it
- `changelog.md` is untouched elsewhere — existing entry wording and
  category order preserved
