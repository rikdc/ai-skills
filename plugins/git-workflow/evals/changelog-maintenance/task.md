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

The maintainer has decided the next release is `2.2.0`. Cut it following the
team's changelog process:

1. Run the process's precondition checks before writing: there must be
   unreleased entries to release, `2.2.0` must not already appear in the file,
   and it must be higher than the most recent version present.
2. Stamp the release with today's date and keep the existing entries under
   their categories.
3. Leave a fresh empty `## [Unreleased]` section above the new release.

Do not tag, commit, or push.

## Output Specification

- `CHANGELOG.md` with a `## [2.2.0] - <today>` section containing the two
  previously unreleased entries, and a fresh empty `## [Unreleased]` above it
- `workflow.md` in the working directory recording the steps you took and the
  precondition checks you ran
