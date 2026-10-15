# Triage PR Review Comments in Dry-Run Mode

## Background

You are triaging review feedback on the payments service repository. The
service lives in the working directory. Run `bash inputs/setup.sh` (or `bash setup.sh` if the fixture sits at the working-directory root) to prepare it:
it creates the repo (`CLAUDE.md` conventions plus `src/handler.py` and
`src/service.py`) and installs a fake `gh` shim at `./bin/gh` (put `./bin`
first on your PATH so the shim is used instead of any real `gh`). The
repository's open PR is #42 "Add payment processing endpoint" with four
review comments from two reviewers.

**Network actions must be skipped** — there is no network in this sandbox.
Use the `gh` shim for everything the review-triage skill would do remotely.
Every write the shim receives (reactions, replies, thread-minimisation) is
recorded in `gh-record.json`, so nothing is graded on backend state you
cannot observe.

## Your Task

The team wants to see proposed decisions before anything is acted on:

1. Run the PR review triage workflow for PR #42 exactly as the
   `triage-reviews` skill describes, in **dry-run mode**
   (`/triage-reviews 42 --dry-run`): fetch the comments, reviews, and diff,
   read the code the comments reference, classify each comment into one of
   the skill's categories, and evaluate each against the project's
   `CLAUDE.md` conventions and the actual code.

2. Do not perform any offline execution actions this round: no reactions,
   no replies, no thread resolution, no follow-up issue creation. The
   summary and the report are the whole deliverable.

## Output Specification

- `triage-report.md` in the working directory containing:
  - the PR summary header (number, title, comment count)
  - one entry per review comment with: the comment id, its file and line,
    its **category** from the skill's classification table, its decision
    (**accept**, **reject**, or **acknowledge**), and the proposed action
    or response
  - for any rejected comment: the specific reason, citing the project
    convention or code intent
  - any grouping of related accepted items into shared follow-up work
- No recorded write actions: `gh-record.json` remains an empty list.
