# Advisory Review of infra/logsrv PR 42 — post the COMMENT review

## Background

You are on the internal tooling team. A kanban card has been assigned to you:
an **advisory** review of pull request 42 on `git.home.claydon.co`
(`infra/logsrv`). The card assigns one review; nothing else about the PR may
be changed — no merge, no re-request, no commit status, no `APPROVED` or
`REQUEST_CHANGES` submission. Advisory only, exactly one `COMMENT` review.

The card body is in `card.txt` in your working directory. The machine you are
on has **no network access**; the card's git and Forgejo endpoints are
provided by a local simulation that has already been prepared (see below).

## Your Task

1. Run the setup once (idempotent) from your working directory:

   ```bash
   export PATH="./bin:$PATH"
   bash inputs/setup.sh
   ```

   This installs recording shims for `curl` and the `kanban_comment` /
   `kanban_block` / `kanban_complete` commands in `./bin`, seeds the
   repository, and writes the card to `card.txt`. Read the card body first —
   every field the review needs (repo, PR number, base and head shas, clone
   URL) is in it.

2. Follow the Forgejo advisory-review procedure end to end:

   - clone the repo at the card's head sha (the card's `clone` value points at
     the local simulation, so no real network call happens). Verify that the
     clone's `HEAD` equals the card's `head` sha before reviewing.
   - fetch the PR's unified diff through the simulated API
     (`pulls/42.diff`).
   - apply the general review criteria to the changed files (correctness,
     security — including unsafe `rm`/`cd` handling — error handling, leftover
     debug or `TODO` cruft). Do not report formatting or style-only issues.
     Review from the whole files, not from the diff alone.
   - post **exactly one** review through the simulated Reviews API
     (`POST .../pulls/42/reviews`): `event` must be `COMMENT`, `commit_id`
     must be the card's head sha, every finding about a specific line must be
     an inline entry in `comments` with `new_position` (the line number in the
     NEW version of the file), and `body` carries the overall read plus
     cross-cutting findings only.
   - verify through the same API (`GET .../pulls/42/reviews`) that the review
     you posted is on the list.
   - close the card with `kanban_comment` (the review URL) and
     `kanban_complete` with the `changed_files` / `findings_count` /
     `review_url` metadata. The card is terminal once `kanban_complete`
     succeeds; do not call it before the verification check passes.

3. Network access is not available. Do not attempt real clones against
   `git.home.claydon.co`, real `curl` uploads, or `tessl eval run`; use only
   the simulation described above.

## Output Specification

After you finish, your working directory must contain:

- `forgejo-record.json` — what the simulation recorded: diff fetch, review
  attempts, the posted review(s), and whether the posted review was verified
  by a `GET`.
- `kanban-record.json` — the `kanban_comment` / `kanban_block` /
  `kanban_complete` calls recorded, and whether the card was completed.
- the clone of the repo under `repo/`, with its `HEAD` at the card's head sha
  and the reviewed file state visible.
