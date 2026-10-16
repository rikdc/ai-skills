# Re-Triage a Partially Processed PR Without Duplicating Actions

## Background

A previous, partial triage pass ran against the payments repository's PR
before the reviewer pushed new feedback. The service lives in the working
directory. Run `bash inputs/setup.sh` (or `bash setup.sh` if the fixture sits at the working-directory root) to prepare it: it creates the repo
(`CLAUDE.md` conventions plus `src/handler.py`), provides
`triage-report-previous.md` summarising what the earlier pass did, and
installs a fake `gh` shim at `./bin/gh` (put `./bin` first on your PATH so
the shim is used instead of any real `gh`). The shim answers the fetch
calls from fixtures and records every WRITE action in `gh-record.json`,
which already contains the previous pass's recorded actions.

**Network actions must be skipped.** The green light from the team is given:
process everything that still needs action, but exactly once.

## Your Task

1. Run the PR review triage workflow for PR #42 as the `triage-reviews`
   skill describes, through to execution — but enforce the skill's
   **idempotency** rule: before acting on any comment, check whether the
   reaction, reply, resolution, or follow-up item already exists (the
   `gh-record.json` rows and `triage-report-previous.md` are your record of
   prior actions) and skip it silently if so.

2. Comments 201 and 202 were fully handled in the previous pass; comment
   203 (a new security finding) is unhandled. Take exactly the actions
   still owed: none for 201 or 202, the full accept flow for 203.

## Output Specification

- `gh-record.json`: the previous pass's seeded rows stay verbatim, and your
  new rows are only the actions owed for comment 203 (plus nothing else).
- `followups.json`: a JSON array of accepted follow-up items. Comment 201's
  follow-up from the previous pass (error-handling, src/handler.py:9) must
  appear exactly once and must NOT be duplicated.
- `triage-report.md`: a fresh summary stating for each comment the
  category, decision, and which actions were taken or skipped as already
  performed.
