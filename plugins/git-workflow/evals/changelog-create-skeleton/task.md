# Create a Changelog from Scratch

## Background

The snapshot-tool repository has been keeping releases in Chandler's release
notes file for a while, but the team now wants a proper `CHANGELOG.md` that
follows the Keep a Changelog format. The repository currently has **no**
`CHANGELOG.md`.

Start with `bash setup.sh` to create the repository state.

## Your Task

1. Create `CHANGELOG.md` using the Keep a Changelog skeleton: header, the
   format/adheres lines, and an empty `## [Unreleased]` section. No version
   sections, no placeholder bullets under empty headings.

2. Seed it with two entries the team wants recorded as unreleased:
   - *Added*: "Detect duplicate snapshots during capture" — under Added.
   - *Fixed*: "Remove stale lockfiles that blocked snapshot cleanup" — under Fixed.

   Follow the style rules: one line per entry, verb-first, user-facing, no
   file paths or commit hashes.

3. Do not invent additional entries from git history unless asked, and do not
   fill category headings that have nothing to record.

## Output Specification

- `CHANGELOG.md` exists in the working directory
- Header lines and empty `## [Unreleased]` present, with the two seeded
  entries under `### Added` and `### Fixed`
- No other categories or version sections; no placeholder text
