# Maintain the Auditor's Changelog

## Background

The audit-checker repository maintains `CHANGELOG.md` in Keep a Changelog
format. The current release is `2.1.0`. The changelog has an `## [Unreleased]`
section with two entries, and an older `## [2.1.0]` section with one entry:

- Unreleased → **Added**: "Add CSV export for audit summaries"
- Unreleased → **Fixed**: "Align summary totals with source ledger"
- 2.1.0 → **Fixed**: "Restore zero-downtime reload after schema bump"

The repository lives in the working directory. Run the fixture setup script
(`bash inputs/setup.sh`, or `bash setup.sh` if it sits at the working-directory
root) to create it; the changelog will be present in the state described above.

## Your Task

Perform the maintenance actions the current state implies:

1. Decide from the changelog's state whether anything needs releasing now. If
   so, cut the release following the team's process (verify unreleased
   entries exist, version is unused and higher than the last, stamp the
   current date, and leave a fresh empty `## [Unreleased]` above). Do not
   tag, commit, or push unless asked.

2. Decide whether any additional maintenance action is warranted based purely
   on the changelog content (e.g. a release already exists, or Unreleased is
   empty, or the version sequence doesn't go backwards). Perform it, or state
   plainly that no further action is needed and why.

## Output Specification

- `CHANGELOG.md` reflecting whatever maintenance action was taken
- `workflow.md` in the working directory describing which actions you took
  and why, or why no further action was needed
