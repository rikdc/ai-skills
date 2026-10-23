---
name: pr
description: Creates a draft pull request with a plain-text conventional-commit title and a succinct description of the branch. Detects whether the repo lives on GitHub or a Forgejo/Gitea host and uses gh or tea accordingly. Use when the user asks to open, raise, or create a pull request or PR.
user-invocable: true
argument-hint: "[--base <branch>]"
allowed-tools: Bash(git add:*), Bash(git status:*), Bash(git commit:*), Bash(git log:*), Bash(git diff:*), Bash(git push:*), Bash(git remote:*), Bash(git config:*), Bash(git branch:*), Bash(gh *), Bash(tea *), Bash(curl:*), Read, Write
---

# PR - Pull Request Creation

Creates a draft pull request with a conventional-commit title and a short
description of what the branch actually changes. Works against GitHub (`gh`)
and Forgejo/Gitea (`tea`).

## Workflow

1. **Detect the forge**: run the detection below. Never assume GitHub.
2. **Validate**: auth for that forge, current branch, uncommitted changes.
3. **Read the branch**: `git log <base>..HEAD` and `git diff <base>...HEAD` —
   the description covers exactly this, nothing more.
4. **Write the body**: use the PR template if present, otherwise the sections
   below.
5. **Push**: `git push -u <remote> HEAD`. Neither CLI reliably pushes for you.
6. **Create**: the command for the detected forge (see Commands by Forge).

## Forge Detection

Resolve the host from the remote the branch actually pushes to, then classify
it:

```bash
branch=$(git branch --show-current)
remote=$(git config --get "branch.$branch.remote" || echo origin)
url=$(git remote get-url "$remote")
host=$(printf '%s' "$url" | sed -E 's#^[a-z+]+://##; s#^[^@]+@##; s#[:/].*$##')
echo "$remote $host"
```

Classify in this order — the first match wins:

1. `host` is `github.com` → **GitHub**.
2. `gh auth status` lists `host` → **GitHub** (Enterprise).
3. `curl -sf -m 5 "https://$host/api/v1/version"` returns `{"version":"..."}`
   → **Forgejo/Gitea**. This endpoint is the reliable tell; GitHub does not
   serve it.
4. Anything else (GitLab, Bitbucket, unreachable host) → **stop and ask**. Do
   not guess a CLI, and do not fall back to `gh` because it happens to be
   installed.

Check the `gh` hosts before probing the API — GitHub Enterprise has no
`/api/v1/version`, so probing first would misclassify it as unknown.

State the detected forge and host in one line before creating anything, in the
form `<forge> (<host>) — using <cli>`.

## Title Rules

- Conventional commit format: `<type>(<scope>): <subject>`.
- **No emoji.** The title is plain text.
- Under 70 characters, imperative mood, no trailing period.
- For a single-commit PR, reuse that commit's subject.

Examples:

- `feat(auth): add session refresh endpoint`
- `fix(scanner): match patterns case-insensitively`
- `docs: document the scanner exit codes`

## Body Rules

Keep it succinct. The PR body describes the commits on the branch and nothing
else.

Look for a template at `.forgejo/pull_request_template.md`,
`.gitea/pull_request_template.md`, then `.github/pull_request_template.md`,
and use the first that exists — Forgejo reads all three, GitHub only the last.

- **Summary**: one to three sentences. What changed and why. No restating the
  title, no background essay.
- **Changes**: one bullet per meaningful change, one line each. Group trivial
  edits rather than listing every file. Under six bullets for a normal PR — if
  it needs more, the PR is probably too large.
- **Testing**: one line stating what was run (`make test`, `go test ./...`) and
  the result. Say so plainly if nothing was run.
- Drop any template section that has nothing real to say.
- No emoji, no headings beyond the template's, no "Notes for reviewers",
  "Future work", "Impact", or risk-assessment sections unless asked.
- Do not claim behaviour you have not verified.

Default body when there is no template:

```markdown
## Summary

<1-3 sentences>

## Changes

- <change>
- <change>

## Testing

<command and result>
```

Example body:

```markdown
## Summary

Pattern matching in the MCP scanner was case-sensitive, so uppercase AWS keys
passed the scan. Matching is now case-insensitive.

## Changes

- Add `-i` to the pattern grep in `mcp-security-scanner.sh`
- Cover uppercase keys in `tests/test-scanner.sh`

## Testing

`make test` — all suites pass.
```

## Commands by Forge

The title and body rules above are identical for both. Only the CLI changes.

| Action | GitHub | Forgejo/Gitea |
| --- | --- | --- |
| Check auth | `gh auth status` | `tea login list` |
| Create draft | `gh pr create --draft --base <base> --title "<title>" --body-file <file>` | `tea pr create --draft --base <base> --title "<title>" --description "$(cat <file>)"` |
| Draft to ready | `gh pr ready <N>` | `tea pulls edit <N> --ready` |
| Add reviewers | `gh pr edit <N> --add-reviewer u1,u2` | `tea pulls edit <N> --add-reviewers u1,u2` |
| Status / list | `gh pr status` | `tea pulls list` |

`tea` specifics worth knowing:

- No `--body-file`. Write the body to a file as usual, then pass it inline with
  `--description "$(cat <file>)"` so the heredoc-style body survives intact.
- `--draft` prepends `WIP: ` to the title; Gitea and Forgejo treat a
  WIP-prefixed PR as a draft. `tea pulls edit --ready` strips it again. The
  prefix is part of the title, so keep the underlying title within the 70-char
  rule with room for it.
- Use long flags only. On `tea pulls edit`, `-r` is claimed by both `--repo`
  and `--add-reviewers`.
- If a repo has several logins configured, disambiguate with
  `--remote <remote>` rather than `--login`, so the PR follows the branch's
  actual push target.

## Error Handling

- Forge not identified: stop and ask which forge and CLI to use. Do not default
  to `gh`.
- Missing CLI: prompt to install (`gh`, or `tea` for Forgejo/Gitea).
- Not authenticated: GitHub — `gh auth status`, prompt to log in. Forgejo —
  `tea login list`; if the host is absent, prompt for
  `tea login add --name <name> --url https://<host> --token <token>` rather
  than attempting an interactive login.
- Missing PR template: use the default body above; do not create the template
  file unless asked.
