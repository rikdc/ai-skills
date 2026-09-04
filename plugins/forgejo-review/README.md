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
- describe the findings as a flat list, then hand them plus a summary to
  the bundled `scripts/submit-review.py`
- the helper parses the PR diff, turns each finding whose line is inside a
  hunk into an inline `new_position` comment, folds every other finding into
  the review body (prefixed `path:line`), posts one `event: "COMMENT"`
  review, retries past Forgejo's all-or-nothing 422 by moving one more
  comment into the body, and verifies the review landed
- record the review URL the helper prints and close the task

It is **advisory**: it never approves, requests changes, merges, or posts a
commit status. It is **read-only**: it does not run the project's tests,
build, `nix`, linters, or formatters.

This skill is a workflow procedure, not a slash command. It is designed to
be force-loaded onto a review task (for example a Hermes `pr-reviewer`
kanban card) so the procedure is never left to model choice.

### `scripts/submit-review.py`

The deterministic half of the workflow above: given `--summary-file`,
`--comments-file` (a JSON array of `{path, line, body}`), `--commit`, and a
diff (`--diff-file` or fetched from the API), it posts exactly one
`COMMENT` review and prints its `html_url`. Host-agnostic — token from
`$FORGEJO_TOKEN`, API base from `--api-base` / `$FORGEJO_API_BASE`
(required — no default). `scripts/submit-review-test` is a
standalone harness (no network; a fake Forgejo records the POST body).

## Fan-out specialist skills

In addition to the advisory-review procedure above, this plugin (plus two
skills in `dev-skills`) provides eight specialist review skills for a
Hermes-driven Forgejo PR-review fan-out. Each specialist is dispatched
against a subset of the PR's changed files, reviews only through its own
lens, and writes findings as JSON matching
[`findings-contract.schema.json`](./findings-contract.schema.json) — the
authoritative output shape every specialist emits (see
[`findings-contract.golden.json`](./findings-contract.golden.json) for a
worked example). They are not user-invocable; an orchestrator dispatches
them by goal.

### Language reviewers

| Skill | Description | Dispatch trigger |
| --- | --- | --- |
| `go-review` (`dev-skills`) | Go-specific correctness, security, performance, and maintainability review | `.go` file suffix |
| `shell-script-reviewer` (`dev-skills`) | Bash/Zsh/shell script review against ShellCheck, BashPitfalls, and style conventions | `.sh` / `.bash` file suffix |

### Topic reviewers

| Skill | Description | Dispatch trigger |
| --- | --- | --- |
| `general-code-review` | Language-agnostic PR review criteria — correctness, security, error handling, repo conventions | always (every PR) |
| `security-review` | Focused security review for changes touching auth, crypto, secrets, tokens, or access control | path matches `(auth\|crypto\|secret\|login\|token\|password\|acl\|firewall)` |
| `test-coverage-review` | Flags source changes with no corresponding test signal in the same diff | a source file changed with no matching test file touched |
| `schema-migration-review` | Reviews database schema migrations for backward compatibility, destructive operations, and rollout safety | path matches `(migrations?/\|\.sql$\|schema\.)` |
| `docs-review` | Reviews Markdown/docs changes for accuracy against the accompanying code change, broken references, staleness | path matches `(\.md$\|docs/)` |
| `api-surface-review` | Flags breaking changes to exported/public function signatures, types, and API contracts with no migration path | a public/exported signature may have changed |

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
