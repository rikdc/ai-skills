# AI Skills Marketplace

This document explains the marketplace structure and plugin distribution system for the AI Skills repository.

## Overview

The repository is organized as a **Claude Code marketplace** with 9 independent, installable plugins. This allows users to install only the components they need rather than getting everything at once.

## Architecture

### Two Directory Structures

The repository maintains two parallel directory structures:

1. **`plugins/`** - Marketplace distribution (for end users)
   - Each plugin is self-contained with its own manifest
   - Ready for installation via Claude Code marketplace
   - Organized by functionality (security-hooks, dev-skills, etc.)

2. **`.claude/`** - Local development (for contributors)
   - All skills, commands, and hooks in one place
   - Easier for development and testing
   - Single source of truth for code

## Available Plugins

### 1. security-hooks

**Category**: Security
**Source**: `./plugins/security-hooks`

**Components**:

- MCP Security Scanner hook
- Protected Branch hook
- Security patterns configuration

**Features**:

- Scans all MCP requests for sensitive data
- Blocks direct edits to protected branches (main, master, production, release)
- Logs all security violations and branch protection events

**Installation**:

```bash
claude code plugins install security-hooks
```

**Documentation**: [plugins/security-hooks/README.md](../plugins/security-hooks/README.md)

---

### 2. dev-skills

**Category**: Development
**Source**: `./plugins/dev-skills`

**Components**:

- `/go-implementor` - Expert Go implementation
- `/go-review` - Go code review
- `/document` - Technical documentation
- `/mentor` - Senior engineer mentorship
- `/manager` - Project orchestration
- `/specify` - Technical specifications
- `/taskify` - Task decomposition
- `/shell-script-reviewer` - Bash/Zsh/Fish script review

**Features**:

- 8 specialized development skills
- Expert knowledge in Go, documentation, and architecture
- Project management and task coordination

**Installation**:

```bash
claude code plugins install dev-skills
```

**Documentation**: [plugins/dev-skills/README.md](../plugins/dev-skills/README.md)

---

### 3. git-workflow

**Category**: Productivity
**Source**: `./plugins/git-workflow`

**Components**:

- `/git-workflow:commit` - Brief conventional commits
- `/git-workflow:pr` - GitHub pull request creation
- `/git-workflow:changelog` - Changelog maintenance

**Features**:

- Automates Git workflows
- Follows conventional commit format
- Generates PR descriptions from commit history
- Maintains CHANGELOG.md in Keep a Changelog format

**Installation**:

```bash
claude code plugins install git-workflow
```

**Documentation**: [plugins/git-workflow/README.md](../plugins/git-workflow/README.md)

---

### 4. pm-tools

**Category**: Productivity
**Source**: `./plugins/pm-tools`

**Components**:

- `/pm:create-prd` - Product requirements documents
- `/pm:generate-tasks` - Task list generation
- `/pm:process-tasks` - Task management

**Features**:

- Structured PRD creation
- Breaks down PRDs into actionable tasks
- Tracks task progress and dependencies

**Installation**:

```bash
claude code plugins install pm-tools
```

**Documentation**: [plugins/pm-tools/README.md](../plugins/pm-tools/README.md)

---

### 5. code-quality

**Category**: Development
**Source**: `./plugins/code-quality`

**Components**:

- `/dev:check` - Comprehensive quality analysis
- `/dev:review` - Code review
- `/clean` - Comment cleanup

**Features**:

- Parallel code quality checks
- Security vulnerability scanning
- Automated fixes for common issues
- Removes redundant comments

**Installation**:

```bash
claude code plugins install code-quality
```

**Documentation**: [plugins/code-quality/README.md](../plugins/code-quality/README.md)

---

### 6. prompt-tools

**Category**: AI
**Source**: `./plugins/prompt-tools`

**Components**:

- `/prompt-reviewer` - Prompt review and improvement
- `/promptify` - Prompt generation

**Features**:

- Expert prompt engineering
- Analyzes prompt clarity and effectiveness
- Generates optimized prompts for LLMs
- Best practices guidance

**Installation**:

```bash
claude code plugins install prompt-tools
```

**Documentation**: [plugins/prompt-tools/README.md](../plugins/prompt-tools/README.md)

---

### 7. pr-review-triage

**Category**: Workflow
**Source**: `./plugins/pr-review-triage`

**Components**:

- `/triage-reviews` - PR review comment triage skill
- `/triage` - Legacy triage command

**Features**:

- Classifies review comments and accepts or rejects each with a reason
- Creates tracked follow-up tasks for deferred feedback
- Resolves handled threads on the pull request
- Supports `--dry-run` and per-category auto-approval

**Installation**:

```bash
claude code plugins install pr-review-triage
```

---

### 8. nix-skills

**Category**: Development
**Source**: `./plugins/nix-skills`

**Components**:

- `/nix-expert` - Expert guidance on Nix, NixOS, and home-manager

**Features**:

- Advisory guidance on Nix expressions, modules, derivations, and overlays
- Flake management and home-manager patterns
- Secrets handling with agenix and sops-nix
- NixOS hardening and the rebuild safety ladder

**Installation**:

```bash
claude code plugins install nix-skills
```

**Documentation**: [plugins/nix-skills/README.md](../plugins/nix-skills/README.md)

---

### 9. forgejo-review

**Category**: Workflow
**Source**: `./plugins/forgejo-review`

**Components**:

- `forgejo-advisory-review` - Procedure for one advisory `COMMENT` review on a Forgejo PR

**Features**:

- Clones the repo at the PR head sha (`refs/pull/<n>/head`, fork-safe) and reads whole files
- Applies language-agnostic criteria plus any language-specific reviewer skills named on the task
- Posts a single review with every line-specific finding as an inline comment
- Recovers from Forgejo's 422 by recomputing `new_position` or moving only that finding to the body
- Advisory and read-only: never approves, merges, posts a status, or runs the project toolchain

**Installation**:

```bash
claude code plugins install forgejo-review
```

**Documentation**: [plugins/forgejo-review/README.md](../plugins/forgejo-review/README.md)

---

## Plugin Structure

Each plugin follows this structure:

```text
plugins/<plugin-name>/
├── .claude-plugin/
│   └── plugin.json          # Plugin manifest
├── skills/                  # Skills (if applicable)
│   └── <skill-name>/
│       └── SKILL.md
├── commands/                # Commands (if applicable)
│   └── <command-name>.md
├── hooks/                   # Hooks (if applicable)
│   └── <hook-script>.sh
├── hooks.json               # Hook configuration (if applicable)
└── README.md                # Plugin documentation
```

### Plugin Manifest (plugin.json)

Each plugin has a `.claude-plugin/plugin.json` manifest:

```json
{
  "name": "plugin-name",
  "description": "Plugin description",
  "version": "1.0.0",
  "author": {
    "name": "Richard Claydon"
  },
  "license": "MPL-2.0",
  "category": "security|development|productivity|ai",
  "keywords": ["keyword1", "keyword2"],
  "skills": ["./skills/"],        // For skill plugins
  "commands": ["./commands/"],    // For command plugins
  "hooks": "./hooks.json"         // For hook plugins
}
```

### Marketplace Registry (marketplace.json)

The top-level `.claude-plugin/marketplace.json` registers all plugins:

```json
{
  "name": "ai-skills",
  "owner": {
    "name": "Richard Claydon"
  },
  "metadata": {
    "description": "Security hooks, branch protection, and development skills for Claude Code projects",
    "version": "1.0.0",
    "pluginRoot": "./plugins"
  },
  "plugins": [
    {
      "name": "plugin-name",
      "source": "./plugins/plugin-name",
      "description": "...",
      "version": "1.0.0",
      "keywords": ["..."],
      "category": "..."
    }
  ]
}
```

## Development Workflow

### For Contributors

1. **Make changes** directly in `plugins/` directories
2. **Test locally** using the test suite
3. **Commit** `plugins/` changes

### For Users

#### Option 1: Install specific plugins

```bash
claude code plugins install security-hooks
claude code plugins install dev-skills
```

#### Option 2: Clone entire repository

```bash
git clone https://github.com/rikdc/ai-skills.git
cd ai-skills
make install
```

## Installation Methods

### From Marketplace (when published)

```bash
claude code plugins install <plugin-name>
```

### From GitHub Repository

```bash
claude code plugins install github:rikdc/ai-skills/<plugin-name>
```

### From Local Directory

```bash
claude code plugins install file:./plugins/<plugin-name>
```

## Benefits of Plugin Architecture

1. **Modularity**: Install only what you need
2. **Discoverability**: Each plugin is separately documented and searchable
3. **Version Control**: Plugins can be versioned independently
4. **Reduced Overhead**: Smaller installation footprint
5. **Clear Organization**: Logical grouping by functionality
6. **Easier Maintenance**: Changes isolated to specific plugins

## Marketplace Categories

- **Security**: Security-focused plugins (scanning, protection)
- **Development**: Development workflow plugins (skills, code quality)
- **Productivity**: Workflow automation (git, project management)
- **AI**: AI and LLM tools (prompts, generation)

## Plugin Dependencies

Currently, all plugins are independent with no dependencies on each other. This allows users to install any combination of plugins.

## Testing

All tests still run against the `.claude/` directory:

```bash
make test           # Run all tests
make lint           # Lint shell scripts and markdown
```

Tests validate the core functionality that plugins are built from.

## Future Enhancements

- Plugin versioning (independent version numbers)
- Plugin dependencies (one plugin requiring another)
- Plugin settings and configuration
- Per-plugin documentation sites
- Automated marketplace publishing

## License

All plugins are licensed under the Mozilla Public License 2.0. See
[LICENSE](../LICENSE) for the full text.

MPL-2.0 is a per-file weak copyleft licence: if you modify a file from this
repository, that file must remain under MPL-2.0 and its source must be made
available. You are free to combine it with proprietary code in a larger work.

### Third-party content

The PM command suite (`/pm:create-prd`, `/pm:generate-tasks`,
`/pm:process-tasks`) is derived from
[AI Dev Tasks](https://github.com/snarktank/ai-dev-tasks/tree/main), which is
licensed under Apache-2.0.

Apache-2.0 material may be included in an MPL-2.0 project, but Apache-2.0 §4
imposes obligations that must be honoured on redistribution:

- **§4(a)** — a copy of the Apache-2.0 licence must be included. It lives at
  [LICENSES/Apache-2.0.txt](../LICENSES/Apache-2.0.txt).
- **§4(b)** — modified files must carry prominent notices stating they were
  changed. See the statement of changes in the root
  [README](../README.md#attribution).
- **§4(c)** — upstream `NOTICE` file contents must be reproduced. AI Dev Tasks
  ships no `NOTICE` file, so this does not apply.

If you add further Apache-2.0 material, place its licence copy under
`LICENSES/` and record the attribution and statement of changes in the root
README.
