---
name: forgejo-advisory-review
description: Step-by-step procedure for producing ONE advisory COMMENT review on a Forgejo pull request — clone at the head sha, fetch the diff, apply general and language review criteria, then hand a summary and a flat findings list to submit-review.py, which maps findings to inline comments, folds the rest into the body, recovers from 422s, and verifies. Use when a kanban card assigns an advisory PR review on git.home.claydon.co.
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

You describe the findings. A bundled helper,
`scripts/submit-review.py`, owns every fiddly part of getting them onto the
PR: parsing the diff, deciding which finding can be an inline comment,
folding the rest into the review body, the single Reviews-API call, 422
recovery, and the post-check. You do not hand-write the `curl`.

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

Save it to `pr.diff` next to your `repo` checkout — step 5 passes this same
file to the helper, so it is fetched once:

```sh
curl -fsS -H "Authorization: token $FORGEJO_TOKEN" \
  "https://git.home.claydon.co/api/v1/repos/<owner>/<name>/pulls/<n>.diff" \
  > pr.diff
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

## 5. Write the two inputs and run the helper

Produce two files:

**`summary.md`** — the overall read: what the PR does, your assessment, and
any finding that has no single changed line (cross-cutting issues, something
missing, a concern spanning several files). Plain Markdown, no front matter.

**`findings.json`** — a flat JSON array, one object per line-specific
finding:

```json
[
  {"path": "scripts/backup.sh", "line": 13,
   "body": "`cd $dest` is unquoted and its exit status is unchecked. If `$dest` is empty or has spaces the `cd` fails and, because `set -e` does not trigger here, the following `rm -rf ./cache/*` runs against the wrong directory. Use `cd \"$dest\" || exit 1`."},
  {"path": "scripts/backup.sh", "line": 14,
   "body": "`rm -rf ./cache/*` relies on the `cd` above having succeeded; with the unchecked `cd` this can delete `cache/` under whatever directory the script ran from. Guard the `cd` (see previous comment)."}
]
```

- `path` is the repo-relative path as it appears in the diff.
- `line` is the line number in the **NEW** version of the file (the `+`
  side). Count from the hunk's `@@ -a,b +c,d @@` header if unsure; the
  helper re-checks every number and will not post a bad one.
- `body` is the finding text. Backticks and fenced snippets are fine.
- Put a finding here whenever it points at a specific changed line. Do not
  pre-filter for "is this line in the diff" — the helper does that, and a
  finding whose line turns out to be outside the diff is moved into the
  review body automatically, not dropped.

Then run the helper from your workspace. It needs the Forgejo API base;
derive it from the scheme+host of the card's `url` and add `/api/v1`
(for `https://forge.example/rikdc/x/pulls/4` that is
`https://forge.example/api/v1`):

```sh
api_base="$(printf '%s' '<url>' | sed -E 's#(https?://[^/]+)/.*#\1/api/v1#')"
python3 "$HERMES_HOME/skills/forgejo-advisory-review/scripts/submit-review.py" \
  --repo <owner>/<name> --pr <n> --commit <head sha> \
  --api-base "$api_base" \
  --summary-file summary.md --comments-file findings.json \
  --diff-file pr.diff
```

On success it prints the review's `html_url` on stdout and a one-line
summary (`N inline comment(s), M finding(s) in the body`) on stderr. Capture
the URL for step 6.

Always run the helper, even with an empty `findings.json` (`[]`) and a
`summary.md` that says nothing blocking was found — there is always a
visible review on the PR.

### What the helper does for you

- Parses `pr.diff` into the set of valid NEW-file line numbers per file.
- Each finding whose `line` is in that set becomes an inline
  `comments[]` entry (`path` + `new_position`); every other finding is
  appended to the review body under **Findings outside the diff**, prefixed
  with `` `path:line` `` so nothing is lost.
- POSTs exactly one review with `event: "COMMENT"` and `commit_id` set to
  `<head sha>`. It never sends `APPROVED`, `PENDING`, or `REQUEST_CHANGES`.
- On Forgejo's all-or-nothing `422` it moves one more inline comment into
  the body and retries, up to 3 times, without ever emptying the array just
  to get a 2xx.
- Verifies the review is listed on the PR before exiting 0.

If the helper exits non-zero, it could not post or could not verify the
review. `kanban_comment` its stderr and `kanban_block` — do **not**
`kanban_complete`. Never hand-write a fallback `curl` to the Reviews API;
the rails forbid a second attempt outside this helper.

## 6. Close the card

1. `kanban_comment` the review's `html_url` (printed by the helper).
2. `kanban_complete` with a one-line `summary` (what you reviewed and how
   many findings) **and** metadata `{changed_files, findings_count,
   review_url}`. `kanban_complete` rejects the call if `summary` is
   missing.

The card is terminal. Do not `request-review`, do not merge, do not post a
commit status.

## If you run low on turns

Before your turn budget runs out, stop analysing, write `summary.md` and
whatever `findings.json` you have, and run the helper (step 5) — then do
step 6. A posted advisory review beats a card stuck in progress.
