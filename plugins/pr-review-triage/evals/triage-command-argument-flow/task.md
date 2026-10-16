# Run the /triage Command with Explicit Arguments

## Background

You received the slash-command invocation `/triage 42 --dry-run` in the
payments service repository. The service lives in the working directory.
Run `bash inputs/setup.sh` (or `bash setup.sh` if the fixture sits at the working-directory root) to prepare it: it creates the repo (`CLAUDE.md`
conventions plus `src/handler.py` and `src/service.py`), provides the
offline GitHub state (the target PR #42 is "Add payment processing
endpoint"), and installs a fake `gh` shim at `./bin/gh` (put `./bin` first
on your PATH so the shim is used instead of any real `gh`). Beware: the
shim's `gh pr view` reports the *current branch's* PR, which is a different
PR (#7) — the command's arguments are what decide the target.

**Network actions must be skipped.** The shim answers the fetch calls
(`gh pr view`, `gh pr diff`, `gh api .../reviews`,
`gh api .../comments`) from fixtures and records every write action in
`gh-record.json`, so "nothing happened" is gradeable.

## Your Task

1. Execute the `/triage` command exactly as its definition describes for
   arguments `42 --dry-run`:
   - the number in `$ARGUMENTS` identifies the target PR (not `gh pr view`)
   - fetch the review comments for PR #42, filtering out self-reviews
     (comments authored by the PR owner) and replies inside existing
     threads
   - classify each remaining comment per the triage-reviews skill
   - evaluate each against `CLAUDE.md` and the actual code
   - present the summary — then **stop**, because the command was run with
     `--dry-run`.

2. Take no write actions: no reactions, no replies, no thread resolution.
   The summary and report are the whole deliverable.

## Output Specification

- `triage-report.md` in the working directory containing:
  - a PR header for **PR #42** ("Add payment processing endpoint")
  - one entry per top-level reviewer comment (ids 101-104) with category,
    decision, and proposed action or response
  - an explicit note that the run was dry-run and the self-review comment
    (105) and the thread reply (106) were excluded per the command's
    filtering rules
- No recorded write actions: `gh-record.json` remains an empty list.
