# Execute the Approved Triage Actions

## Background

You are triaging review feedback on the payments service repository. The
service lives in the working directory. Run `bash inputs/setup.sh` (or `bash setup.sh` if the fixture sits at the working-directory root) to prepare it:
it creates the repo (`CLAUDE.md` conventions plus `src/handler.py` and
`src/service.py`) and installs a fake `gh` shim at `./bin/gh` (put `./bin`
first on your PATH so the shim is used instead of any real `gh`). The
repository's open PR is #42 "Add payment processing endpoint" with four
review comments from three reviewers.

**Network actions must be skipped** — there is no network in this sandbox.
Use the `gh` shim for everything the review-triage skill would do remotely.
The shim answers the fetch calls (`gh pr view`, `gh pr diff`,
`gh api .../reviews`, `gh api .../comments`) from fixtures and records every
write action in `gh-record.json` (with `action`, and for replies the `body`
and `in_reply_to` comment id), so submission quality is graded offline.

The team's tech lead has already confirmed: **process all four comments now**
(treat this as step 4's user confirmation, so do not stop at the summary).

## Your Task

1. Run the PR review triage workflow for PR #42 as the `triage-reviews`
   skill describes, through to **execution**: fetch the comments, reviews,
   and diff, read the code the comments reference, classify each comment,
   evaluate each against `CLAUDE.md` and the actual code, and act on every
   one.

2. For each comment, perform the action the skill prescribes:
   - Accepted → react with a thumbsup through the shim and register the fix
     as a tracked follow-up item (see artifacts below).
   - Rejected → post a respectful reply that cites the specific project
     convention (e.g. the `CLAUDE.md` snake_case rule) or code intent, then
     resolve the thread through the shim.
   - Praise → thank the reviewer via a reply, then resolve the thread.
   - Question → answer it in a reply based on the actual code, then resolve
     the thread.

## Output Specification

- `gh-record.json` in the working directory populated by the shim:
  one entry per write action (react / reply / resolve) referencing the
  right comment ids, with rejection replies citing a concrete convention.
- `triage-report.md` summarising every comment: id, file/line, category,
  decision, and the action taken.
- `followups.json` in the working directory: a JSON array of the accepted
  follow-up items, each with a short title, the file/line to fix, the PR
  comment link (html_url), a description of what needs to change and why,
  and a priority consistent with the category weight.
