# PR Review Triage Plugin

Triage PR review comments — classify, accept/reject, and track follow-up work.

## What's Included

### `/triage-reviews` (skill)

Systematically processes unresolved review comments on a pull request: classifies each one, evaluates it against the actual code and project conventions, presents a summary, and acts only after you confirm.

**Features**:

- Classifies comments into weighted categories (security, correctness, error-handling, performance, design, testing, style, nit, praise, question)
- Reads the referenced file and line before judging a comment, rather than evaluating it in isolation
- Cites project conventions when rejecting a suggestion
- Resolves comment threads and creates tracked follow-up tasks for accepted work
- Filters out self-review comments and replies to existing threads

### `/triage` (command)

Slash-command entry point to the same triage workflow.

## Installation

Install via Claude Code marketplace:

```bash
claude code plugins install pr-review-triage
```

Or install from this repository:

```bash
claude code plugins install github:rikdc/ai-skills/pr-review-triage
```

## Requirements

The [`gh` CLI](https://cli.github.com/) must be installed and authenticated — the plugin reads PR comments and diffs through `gh pr` and `gh api`.

## Usage Examples

```bash
# Triage reviews on the current branch's PR
/triage-reviews

# Triage a specific PR
/triage-reviews 123

# Preview decisions without acting
/triage-reviews --dry-run

# Auto-accept an entire category
/triage-reviews --auto-approve security

# Leave comment threads unresolved
/triage-reviews --no-resolve
```

## Best Practices

- Start with `--dry-run` on an unfamiliar PR to see the proposed classifications before anything is acted on.
- Reserve `--auto-approve` for categories where the decision is never in doubt, such as `security`.
- Make sure the repository's CLAUDE.md and linter config are current — rejections are argued from those conventions.

## License

Mozilla Public License 2.0 — see [LICENSE](../../LICENSE).
</content>
</invoke>
