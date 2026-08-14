# Git Workflow Plugin

Git workflow automation skills for commits, PRs, and changelog management.

## Skills Included

### `/git-workflow:commit`

Creates commits with brief conventional commit messages.

**Features**:

- Conventional commit format (feat, fix, docs, etc.), no emoji
- Subject-only messages by default; body only when the diff cannot explain the change
- Automatic commit message generation from staged changes
- Suggests splitting when staged changes mix concerns

### `/git-workflow:pr`

Creates a PR on GitHub with proper title and description.

**Features**:

- Analyzes full commit history for the branch
- Generates a concise PR title (under 70 characters, no emoji)
- Writes a succinct description covering only what the branch changes
- Automatically pushes branch if needed

### `/git-workflow:changelog`

Maintains a CHANGELOG.md document following Keep a Changelog format.

**Features**:

- Semantic versioning
- Categorized changes (Added, Changed, Fixed, etc.)
- Automatic date formatting
- Preserves existing changelog structure

## Installation

Install via Claude Code marketplace:

```bash
claude code plugins install git-workflow
```

Or install from this repository:

```bash
claude code plugins install github:rikdc/ai-skills/git-workflow
```

## Usage Examples

```bash
# Create a commit
/git-workflow:commit

# Create a commit, skipping pre-commit checks
/git-workflow:commit --no-verify

# Create a draft pull request
/git-workflow:pr

# Add a changelog entry
/git-workflow:changelog --add-entry "Add user authentication" --type added
```

Each skill also triggers on plain requests such as "commit this" or "open a
PR" — the slash form is just the explicit way to invoke it.

## License

Mozilla Public License 2.0 — see [LICENSE](../../LICENSE).
