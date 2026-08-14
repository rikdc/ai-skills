# AI Skills

A Claude Code marketplace of seven plugins. Two are security hooks: one scans
MCP traffic for secrets, the other stops you editing protected branches
directly. The rest are skills and commands for Go work, git workflow, code
review, project management, and prompt engineering.

Install only the plugins you want, or clone the repository and work on them.

## Installation

Add the marketplace, then install plugins from it:

```bash
claude plugin marketplace add rikdc/ai-skills

claude plugin install security-hooks@ai-skills
claude plugin install dev-skills@ai-skills
claude plugin install git-workflow@ai-skills
claude plugin install pm-tools@ai-skills
claude plugin install code-quality@ai-skills
claude plugin install prompt-tools@ai-skills
claude plugin install pr-review-triage@ai-skills
```

The `/plugin` command does the same thing from inside a session.

Hooks only take effect after a Claude Code restart.

### Clone for local development

```bash
git clone https://github.com/rikdc/ai-skills.git
cd ai-skills
make install
make test
```

## Available plugins

| Plugin | Category | Description |
|--------|----------|-------------|
| [security-hooks](plugins/security-hooks/) | Security | MCP security scanner and protected branch hooks |
| [dev-skills](plugins/dev-skills/) | Development | Skills for Go, documentation, specs, and architecture |
| [git-workflow](plugins/git-workflow/) | Productivity | Commits, pull requests, and changelog maintenance |
| [pm-tools](plugins/pm-tools/) | Productivity | PRDs, task generation, and task tracking |
| [code-quality](plugins/code-quality/) | Development | Code quality analysis, review, and comment cleanup |
| [prompt-tools](plugins/prompt-tools/) | AI | Prompt generation and review |
| [pr-review-triage](plugins/pr-review-triage/) | Workflow | Triage of PR review comments and follow-up tracking |

## Security hooks

Both hooks run on `PreToolUse` and are configured by the plugin manifest, so
there is nothing to wire up by hand.

| Hook | Trigger | Behaviour |
|------|---------|-----------|
| [MCP security scanner](docs/mcp-security-scanner.md) | Any `mcp__*` tool call | Scans requests for API keys, tokens, database URLs, and PII, then logs what it finds |
| [Protected branch hook](docs/protect-main-branch-hook.md) | `Edit`, `Write`, `Bash`, `Task` | Blocks writes on `main`, `master`, `production`, and `release`, and tells you to branch first |

Detection patterns live in `.claude/security-patterns.conf`. Copy
`.claude/security-patterns.conf.example` to start from, then add your own.

## Skills

`dev-skills`:

- `/dev-skills:golang-expert`: Go advice across concurrency, errors, project structure, performance, and testing
- `/dev-skills:go-implementor`: writes production Go with tests and observability
- `/dev-skills:go-review`: Go review for correctness, security, and performance
- `/dev-skills:document`: API docs, ADRs, architecture docs, and runbooks
- `/dev-skills:mentor`: senior-engineer perspective on design and technical strategy
- `/dev-skills:manager`: coordinates work that spans several specialists
- `/dev-skills:specify`: turns designs into implementable specifications
- `/dev-skills:taskify`: breaks specifications into atomic tasks

`git-workflow`:

- `/git-workflow:commit`: conventional commit messages, with a nudge to split when the diff mixes concerns
- `/git-workflow:pr`: opens a draft pull request describing the branch
- `/git-workflow:changelog`: maintains `CHANGELOG.md` in Keep a Changelog format

`pr-review-triage`:

- `/pr-review-triage:triage-reviews`: classifies review feedback, applies what holds up, and explains what does not

## Commands

`code-quality`:

- `/code-quality:check`: quality analysis and auto-fix, run as parallel sub-tasks
- `/code-quality:review`: full review of the repository
- `/code-quality:clean`: strips redundant comments

`pm-tools`:

- `/pm-tools:create-prd`: PRD creation, with clarifying questions up front
- `/pm-tools:generate-tasks`: phased task lists from a PRD
- `/pm-tools:process-tasks`: one sub-task at a time, with checkpoints

`prompt-tools`:

- `/prompt-tools:promptify`: generates prompts for LLMs and agents
- `/prompt-tools:prompt-reviewer`: reviews prompts for clarity and effectiveness

`pr-review-triage`:

- `/pr-review-triage:triage`: triages unresolved review comments on a PR

## Working on the repository

```bash
make install        # Make the hook scripts executable
make test           # Scanner, branch protection, and marketplace consistency tests
make validate       # Check the plugin manifest, hook scripts, and dependencies
make lint           # ShellCheck and markdownlint
make check-tools    # Report required and optional tools
make status         # Show configuration and tool status
make help           # List every target
```

`jq`, `grep`, `awk`, and `mktemp` are required. `trufflehog`, `gitleaks`, and
`git-secrets` are optional, and improve secret detection when present.

## Project structure

```text
plugins/                        # Everything the marketplace ships
├── security-hooks/             # MCP scanner and branch protection hooks
├── dev-skills/                 # Go, docs, specs, architecture
├── git-workflow/               # Commit, PR, changelog
├── pm-tools/                   # PRDs and task management
├── code-quality/               # Review and cleanup
├── prompt-tools/               # Prompt generation and review
└── pr-review-triage/           # PR comment triage

.claude-plugin/marketplace.json # Marketplace manifest
.claude/                        # Local config: security patterns, logs
docs/                           # Hook and marketplace documentation
scripts/                        # Validation tooling
tests/                          # Test suite
Makefile                        # Everything above, in one place
```

Plugin content lives under `plugins/` and nowhere else. Each plugin owns its
manifest in `.claude-plugin/plugin.json`, which is also where hook wiring is
declared.

## Attribution

### Product management commands

The PM command suite (`/pm-tools:create-prd`, `/pm-tools:generate-tasks`,
`/pm-tools:process-tasks`) is based on patterns and workflows from:

- **Source**: [AI Dev Tasks](https://github.com/snarktank/ai-dev-tasks/tree/main)
- **License**: Apache License 2.0. Full text at [LICENSES/Apache-2.0.txt](LICENSES/Apache-2.0.txt)
- **Usage**: Adapted and extended for Claude Code project management workflows

**Statement of changes** (Apache-2.0 §4(b)): the upstream `create-prd.md` and
`generate-tasks.md` have been modified. Changes include restructuring them as
Claude Code slash commands, adding frontmatter, adjusting the task-list format,
and extending the workflow with `process-tasks`. The files in
`plugins/pm-tools/commands/` are not byte-identical to their upstream
originals.

Thanks to the AI Dev Tasks project for the underlying patterns.

## License

Copyright (c) Richard Claydon.

Licensed under the Mozilla Public License, v. 2.0. See [LICENSE](LICENSE) for
the full text, or obtain a copy at <https://mozilla.org/MPL/2.0/>.

MPL-2.0 is a per-file weak copyleft licence. In practice:

- **Use it freely**, including commercially, with no obligation to open your own code
- **Credit is required**: retain the copyright and licence notices
- **Modifications to these files must stay open** under MPL-2.0, with source made available
- **Larger works may be proprietary**, so you can combine this with closed code

The PM command suite retains its upstream Apache-2.0 licence, as noted under
[Attribution](#attribution). Apache-2.0 material may be included in an MPL-2.0
project provided its licence copy, attribution, and statement of changes are
preserved. See [LICENSES/Apache-2.0.txt](LICENSES/Apache-2.0.txt).
