# Advisory Review of infra/logsrv PR 42 — recover from a 422 and keep the findings inline

## Background

Same team, same kind of work as a normal advisory review: a kanban card
assigns you **one** `COMMENT` review of pull request 42 on `git.home.claydon.co`
(`infra/logsrv`). The PR adds a `prune()` helper to `scripts/rotate.sh`; the
diff and the card are in your working directory.

Note about this simulation: the Forgejo endpoint is a local recording shim
with **no network**. The shim deliberately rejects the **first** review-POST
attempt with `422 Unprocessable Entity` (a reconciliation pass is scheduled at
the moment of the first submission) so that the review's §6 recovery path gets
exercised. Every attempt is recorded, so how you recover is visible: recompute
the offending `new_position` values by counting `+` and context lines in the
diff hunk from its `@@ -a,b +c,d @@` header — do not guess, do not drop or
over-merge findings, and re-POST. Retry up to 3 times; never collapse the
`comments` array to empty just to obtain a 2xx.

## Your Task

1. Run the setup once (idempotent) from your working directory:

   ```bash
   export PATH="./bin:$PATH"
   bash inputs/setup.sh
   ```

   This installs the recording `curl` and `kanban_*` shims, seeds the
   repository and diff fixture, and writes `card.txt`. Read the card body
   first.

2. Run the full Forgejo advisory-review procedure:

   - clone from the card's `clone` URL (points at the local simulation) at the
     card's `head` sha; verify `HEAD` equals the card's `head` sha.
   - fetch the PR's unified diff from the simulated API (`pulls/42.diff`).
   - apply the general review criteria (correctness, security — the fixture
     line containing an unguarded `rm -rf` relative glob, and its neighbouring
     unchecked `cd` — error handling, debug cruft). Review from whole files,
     not the diff alone; report only findings you are confident are real.
   - post one review with the PR-anchored metadata the skill mandates:
     `event` = `COMMENT`, `commit_id` = the card's head sha, line findings
     inline in `comments` with `new_position` = the line number in the NEW
     version, overall read in `body` only.
   - when the first POST 422s, recover per §6 and re-POST the **same review**,
     corrected if needed.
   - verify with `GET .../pulls/42/reviews` that the review is on the list.
   - close the card (only after the verify passes) with `kanban_comment` and
     `kanban_complete` (`changed_files`, `findings_count`, `review_url`).

3. No network access; the shims are the only way anything reaches the Forgejo
   endpoints. Do not invent additional review behaviour, do not push, and do
   not post an `APPROVED` or `REQUEST_CHANGES` review at any point.

## Output Specification

After you finish, your working directory must contain:

- `forgejo-record.json` — every review attempt recorded (attempt number,
  status, reject_reason) plus the accepted review(s) and the verify state.
- `kanban-record.json` — recorded card calls and the `completed` bool.
- the clone of the repo under `repo/`, at the card's head sha.
