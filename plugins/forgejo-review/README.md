# Forgejo Review Plugin

Advisory Forgejo pull-request review workflow — clone at head, fetch the
diff, review, post one COMMENT review with inline comments.

## Skills Included

### `forgejo-advisory-review`

The step-by-step procedure for producing **one** advisory `COMMENT` review
on a Forgejo pull request:

- clone the repo at the PR head sha (`refs/pull/<n>/head`, fork-safe) and
  read whole files for context
- fetch the PR's unified diff from the Forgejo API
- apply language-agnostic review criteria (correctness, security, clear
  bugs, `CLAUDE.md` violations) plus any language-specific reviewer skills
  named on the task
- post a single review via `POST /repos/{owner}/{name}/pulls/{n}/reviews`
  with `event: "COMMENT"`, every line-specific finding as an inline comment
- recover from Forgejo's 422 (a comment outside the diff hunks rejects the
  whole call) by recomputing `new_position` or moving only that finding to
  the body
- verify the review landed, then record its URL and close the task

It is **advisory**: it never approves, requests changes, merges, or posts a
commit status. It is **read-only**: it does not run the project's tests,
build, `nix`, linters, or formatters.

This skill is a workflow procedure, not a slash command. It is designed to
be force-loaded onto a review task (for example a Hermes `pr-reviewer`
kanban card) so the procedure is never left to model choice.

## Installation

Install via Claude Code marketplace:

```bash
claude code plugins install forgejo-review
```

Or install from this repository:

```bash
claude code plugins install github:rikdc/ai-skills/forgejo-review
```

## License

Mozilla Public License 2.0 — see [LICENSE](../../LICENSE).
