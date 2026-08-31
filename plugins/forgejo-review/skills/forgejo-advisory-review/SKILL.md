---
name: forgejo-advisory-review
description: Step-by-step procedure for producing ONE advisory COMMENT review on a Forgejo pull request — clone at the head sha, fetch the diff, apply general and language review criteria, post a single Reviews-API review with inline comments, recover from 422s, then verify and close the kanban card. Use when a kanban card assigns an advisory PR review on git.home.claydon.co.
user-invocable: false
allowed-tools: Read, Glob, Grep, Bash
---

# Forgejo advisory review — procedure

You have been handed a kanban card that asks you to review one pull request
on `git.home.claydon.co` and post **advisory** feedback. This skill is the
procedure. Your profile carries the rails (advisory only, read-only, one
`COMMENT` review, `git.home.claydon.co` is the only host); this skill tells
you how to carry it out.

The whole job is: **one Forgejo review of type `COMMENT`, then a completed
kanban card.** Nothing else touches the PR.

## 0. Read the card body

The card body is a block of `key: value` lines:

| key | meaning |
| --- | --- |
| `repo` | `owner/name` on `git.home.claydon.co` |
| `pr` | PR number (`<n>` below) |
| `title` | PR title |
| `author` | PR author's login |
| `base` | base ref `@` base sha |
| `head` | head ref `@` head sha |
| `url` | PR web URL |
| `clone` | git clone URL (`https://git.home.claydon.co/<owner>/<name>.git`) |
| `changed_files` | count, or `unknown` |
| `review_skills` | comma-separated language reviewer skill names — **MAY BE EMPTY** |

`review_skills` lists *language-specific* reviewers only (for example
`shell-script-reviewer`, `go-review`). This workflow skill is separate and
is always loaded for you; it is never listed there.

## 1. Clone the repo at the head sha

Work in your scratch workspace:

```sh
git clone --depth 50 <clone> repo && cd repo
git fetch --depth 50 origin refs/pull/<n>/head
git checkout FETCH_HEAD
```

`refs/pull/<n>/head` also covers fork PRs. After checkout, `git rev-parse
HEAD` should equal the card's `head` sha — if it does not, note the mismatch
in the review body and continue against what you actually checked out.

Read whole files for context. **Never review from the diff alone.**

If the clone or the checkout fails, `kanban_block` with the reason and stop.

## 2. Fetch the PR's unified diff

```sh
curl -fsS -H "Authorization: token $FORGEJO_TOKEN" \
  "https://git.home.claydon.co/api/v1/repos/<owner>/<name>/pulls/<n>.diff"
```

The diff tells you which files and which lines changed. If the diff fetch
fails, `kanban_block` with the reason and stop.

## 3. General review criteria

Apply these to **every changed file, in every language**:

- correctness and logic errors
- security issues: injection, auth/authz gaps, unsafe deserialisation,
  secrets or credentials committed in the diff
- clear bugs and unhandled error / edge cases
- violations of the repo's `CLAUDE.md` (read it if present)
- leftover debug statements, newly-added `TODO` / `FIXME`, merge-conflict
  markers

Do **not** report:

- formatting, lint, or type errors — CI covers those
- issues on lines this PR did not change
- style preferences that no explicit project convention backs

Keep only findings you are confident are real. A short, correct review beats
a long speculative one.

## 4. Apply the language reviewers

For each name in `review_skills` (skip this section entirely if it is
empty):

1. Read its `SKILL.md` at `$HERMES_HOME/skills/<name>/SKILL.md`.
2. If that path does not exist, run `hermes skills inspect <name>`.
3. If it genuinely cannot be found, note that one line in the review body
   and carry on with the rest — **do not** block the card for a missing
   language skill.

Apply each language skill only to the file types it covers (for example
`shell-script-reviewer` to `*.sh` / `*.bash`, `go-review` to `*.go`).

This is a **read-only** review. Do not run the project's test suite, build,
`nix` commands, linters, or formatters — your scratch workspace does not
have the toolchain and a failing command here tells you nothing. If a
finding would normally need a command to confirm, state it as "likely" and
move on.

## 5. Post exactly ONE review (Reviews API)

```text
POST /api/v1/repos/<owner>/<name>/pulls/<n>/reviews
{
  "event": "COMMENT",
  "commit_id": "<head sha>",
  "body": "<summary + only findings with no single changed line>",
  "comments": [
    {"path": "<file>", "new_position": <line in NEW file>, "body": "<finding>"}
  ]
}
```

```sh
curl -fsS -X POST \
  -H "Authorization: token $FORGEJO_TOKEN" \
  -H "Content-Type: application/json" \
  -d @- \
  "https://git.home.claydon.co/api/v1/repos/<owner>/<name>/pulls/<n>/reviews"
```

Rules:

- `event` is **always** `"COMMENT"`. The API also accepts `APPROVED`,
  `PENDING`, `REQUEST_CHANGES` — never send any of them.
- `commit_id` is the card's `head` sha.
- **Every finding about a specific line MUST be an inline entry in
  `comments`** — `path` plus `new_position`, the line number in the NEW
  version of the file (the `+` side of the diff). Do not summarise
  line-specific findings in `body`. A review that had line findings but put
  them only in `body` is a failure — redo it.
- `body` is for the overall read (what the PR does, your assessment) and for
  findings that genuinely have no single changed line: cross-cutting issues,
  something missing, a concern spanning several files.
- Always post a review, even with an empty `comments` array and a `body`
  that says nothing blocking was found. There is always a visible artifact
  on the PR.

### Worked example

PR adds a helper to `scripts/backup.sh`; the new hunk is:

```diff
@@ -10,6 +10,11 @@ set -eu
 main() {
+  dest=$1
+  cd $dest
+  rm -rf ./cache/*
+  echo "cleaned $dest"
+}
```

`cd $dest` is line 13 in the new file, `rm -rf ./cache/*` is line 14. A
correct multi-comment body:

```json
{
  "event": "COMMENT",
  "commit_id": "9f3a1c2e5b7d0a4f6c8e1b2d3a5f7c9e0b1d2f3a",
  "body": "Adds a cache-cleaning helper to scripts/backup.sh. The logic is small and readable; two robustness issues on the new lines, noted inline. No security or correctness blockers beyond those.",
  "comments": [
    {"path": "scripts/backup.sh", "new_position": 13,
     "body": "`cd $dest` is unquoted and its exit status is unchecked. If `$dest` is empty or has spaces the `cd` fails and, because `set -e` does not trigger on a failed command in this position, the following `rm -rf ./cache/*` then runs against the wrong directory. Use `cd \"$dest\" || exit 1`."},
    {"path": "scripts/backup.sh", "new_position": 14,
     "body": "`rm -rf ./cache/*` relies on the `cd` above having succeeded; combined with the unchecked `cd` this can delete `cache/` under whatever directory the script was invoked from. Guard the `cd` (see previous comment)."}
  ]
}
```

Note both line findings are inline; `body` only carries the overall read.

## 6. 422 recovery

`new_position` / `old_position` are **file line numbers**, not diff offsets.
Forgejo rejects the **whole call** with `422 Unprocessable Entity` if any
comment points at a line outside the diff hunks.

On a 422:

1. Recompute the offending `new_position` by counting `+` and context lines
   in the hunk from its `@@ -a,b +c,d @@` header — do not guess.
2. Re-POST with the corrected number.
3. If a specific finding genuinely has no in-diff line, move **only that
   finding** into `body` and keep every other finding inline.
4. Retry up to 3 times.

Never collapse the whole `comments` array to empty just to get a 2xx.

## 7. Verify the review landed

```sh
curl -fsS -H "Authorization: token $FORGEJO_TOKEN" \
  "https://git.home.claydon.co/api/v1/repos/<owner>/<name>/pulls/<n>/reviews"
```

Confirm the response contains the review you just posted: its `commit_id`
matches the card's `head` sha and its `user` is your own bot login.

**You MUST NOT call `kanban_complete` unless this check passes.** If it does
not — including any case where your only successful output went somewhere
other than `git.home.claydon.co` — `kanban_comment` the failure and
`kanban_block`. Never complete a card over a review that never reached the
Forgejo PR.

## 8. Close the card

1. `kanban_comment` the review's `html_url`.
2. `kanban_complete` with metadata `{changed_files, findings_count,
   review_url}`.

The card is terminal. Do not `request-review`, do not merge, do not post a
commit status.

## If you run low on turns

Before your turn budget runs out, stop analysing and post the review you
have (step 5) — still with each line-specific finding inline, anything
unresolved in `body` — then run steps 7–8. A posted advisory review beats a
card stuck in progress.
