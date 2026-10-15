# Create a Changelog from Scratch

## Background

The snapshot-tool repository keeps its release notes in a shared document,
but the team now wants a proper `CHANGELOG.md` following the Keep a Changelog
format. The repository currently has **no** `CHANGELOG.md`.

Start by running the fixture setup script (`bash inputs/setup.sh`, or
`bash setup.sh` if it sits at the working-directory root) to create the
repository state.

## Your Task

1. Create `CHANGELOG.md` for the project, following the team's changelog
   process.

2. Record these two unreleased changes:
   - *Added*: "Detect duplicate snapshots during capture"
   - *Fixed*: "Remove stale lockfiles that blocked snapshot cleanup"

3. Leave the new file uncommitted — do not commit or push.

## Output Specification

- `CHANGELOG.md` exists in the working directory with the two changes
  recorded
