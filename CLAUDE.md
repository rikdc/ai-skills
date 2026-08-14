# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is the AI Skills project, which provides security hooks and testing infrastructure for Claude Code projects. The main features are:
1. Security scanning to prevent sensitive data from being sent to external MCP services
2. Branch protection to enforce PR-based workflows by preventing direct edits to protected branches (main, master, production, release)

## Marketplace Structure

The repository is organized as a **marketplace with 7 installable plugins**:

- **security-hooks**: MCP security scanner and protected branch hooks
- **dev-skills**: Expert skills for Go, documentation, and architecture
- **git-workflow**: Git automation for commits, PRs, and changelog
- **pm-tools**: Project management commands for PRDs and tasks
- **code-quality**: Code quality analysis, review, and cleanup
- **prompt-tools**: AI prompt generation and review tools
- **pr-review-triage**: Triage of PR review comments and follow-up tracking

### Directory Structure

- **`plugins/`**: Marketplace plugin distribution (for users to install individual plugins)
- **`.claude/`**: Local development structure (for contributors working on the codebase)


## Key Commands

All project operations are managed through the Makefile:

```bash
make test               # Run complete test suite
make lint               # Run ShellCheck on all shell scripts
make install            # Install hooks (make scanner script executable)
make sync-codex-skills  # Sync .codex/skills symlinks with plugins/*/skills
make clean              # Remove test artifacts and logs
make status             # Show current status and configuration
make check-tools        # Check for required and optional security tools
make help               # Display all available commands
```

## Claude Code Skills and Commands

This project uses the modern Claude Code skills structure alongside legacy commands.

### Skills (`plugins/*/skills/`)

Skills are the recommended approach for reusable Claude Code behaviors:

- `/mentor` - Senior Staff Engineer mentor for architecture, design decisions, and career guidance
- `/document` - Technical documentation expert for API docs, ADRs, runbooks, and guides
- `/go-implementor` - Expert Go engineer for production-grade service implementation
- `/go-review` - Senior Go code reviewer for quality, security, and best practices
- `/manager` - Engineering project orchestrator for coordinating complex initiatives
- `/specify` - Software specification expert for detailed technical specs
- `/taskify` - Task decomposition expert for breaking specs into actionable tasks

### Migration from Agents

Previous agent definitions have been migrated to skills:

| Old Agent | New Skill |
|-----------|-----------|
| `document-agent.md` | `/document` |
| `go-implementor-agent.md` | `/go-implementor` |
| `go-review-agent.md` | `/go-review` |
| `manager-agent.md` | `/manager` |
| `specify-agent.md` | `/specify` |
| `staff-eyes-agent.md` | `/mentor` (renamed for clarity) |
| `taskify-agent.md` | `/taskify` |

### Commands (`plugins/*/commands/`)

Legacy slash commands (still functional):

- `/check` - Comprehensive code quality analysis and auto-fix with parallel sub-task strategy
- `/clean` - Remove redundant and obvious comments from codebase
- `/git-workflow:changelog` - Create and maintain project changelog following Keep a Changelog format (migrated to a skill in the `git-workflow` plugin)

## Architecture

### Core Components

- **Security Scanner Hook** (`plugins/security-hooks/hooks/mcp-security-scanner.sh`): Main security scanner that intercepts MCP requests and scans for sensitive data using regex patterns and optional external tools
- **Protected Branch Hook** (`plugins/security-hooks/hooks/protect-main-branch.sh`): Prevents direct edits to protected branches (main, master, production, release) by blocking Edit, Write, Bash, and Task tools, enforcing PR-based workflows
- **Hook Configuration** (`plugins/security-hooks/.claude-plugin/plugin.json`): Claude Code hook configuration that automatically runs hooks for MCP tools and code-modifying operations
- **Security Patterns** (`.claude/security-patterns.conf`): Configurable regex patterns for detecting sensitive data (API keys, tokens, database URLs, etc.)
- **Custom Commands** (`plugins/*/commands/`): Slash commands that provide specialized workflows for code quality, cleanup, and changelog maintenance

### Testing Infrastructure

The project uses a simple, focused testing approach:

- **Security Scanner Tests** (`tests/test-scanner.sh`): Validates core security scanner functionality including sensitive data detection and MCP tool filtering
- **Protected Branch Tests** (`tests/test-protect-main-branch.sh`): Validates branch protection logic, tool blocking, and feature branch allowance
- **Fast Execution**: Tests run in seconds and focus on essential functionality
- **Test Coverage**: Both hooks have comprehensive test suites ensuring correct behavior

### Security Pattern Detection

The scanner detects multiple categories of sensitive data:

- Authentication & API keys (AWS, GitHub, OpenAI, Slack, Discord, etc.)
- Database connection strings (PostgreSQL, MySQL, MongoDB)
- Personal information (emails, credit cards, SSNs, phone numbers)
- Cloud provider credentials and private keys

## Development Workflow

1. **Testing**: Always run `make test` before committing changes
2. **Linting**: Use `make lint` to ensure shell script quality
3. **Pattern Updates**: Modify `.claude/security-patterns.conf` to add custom security patterns
4. **Hook Testing**: Test security patterns with sample MCP requests as shown in the documentation
5. **Quality Checks**: Use `/check` command for comprehensive code quality analysis and auto-fixing
6. **Code Cleanup**: Use `/clean` command to remove redundant comments before commits
7. **Changelog**: Use `/git-workflow:changelog` to maintain project changelog with proper versioning

## Hook System Architecture

The project uses Claude Code's PreToolUse hook mechanism with two complementary hooks:

### Security Scanner Hook
- **Trigger**: All MCP tool calls matching pattern `mcp__.*`
- **Flow**: Request → Security Scanner → Pattern Detection → Logging/Blocking
- **Scope**: Monitors all MCP servers (Sequential, Context7, Magic, Playwright, etc.)
- **Response**: Provides monitoring/auditing with violation logging

### Protected Branch Hook
- **Trigger**: Edit, Write, Bash, and Task tool calls matching pattern `^(Edit|Write|Bash|Task)$`
- **Flow**: Tool Call → Branch Detection → Protection Check → Allow/Block
- **Protected Branches**: main, master, production, release
- **Response**: Blocks operations with guidance to create feature branches

**Restart Requirement**: Hook configuration changes require Claude Code restart

## External Dependencies

**Required tools**: `jq`, `grep`, `awk`, `mktemp`

**Optional security tools** (enhance detection capabilities):

- `trufflehog`: Advanced secret scanning
- `gitleaks`: Git secret detection  
- `git-secrets`: Prevent secrets in commits

## Key Files

- `plugins/security-hooks/.claude-plugin/plugin.json`: Hook configuration for all hooks (MCP scanner and branch protection)
- `.claude/security-patterns.conf`: Security detection patterns (generated at runtime from `security-patterns.conf.example`)
- security-scan.log (in `.claude/`): Audit log of all security scan activity, generated at runtime and gitignored
- protect-main-branch.log (in `.claude/`): Audit log of branch protection activity, generated at runtime and gitignored
- `plugins/*/skills/`: Modern skills with SKILL.md format (recommended)
- `plugins/*/commands/`: Legacy slash commands (check, clean, changelog)
- `tests/test-scanner.sh`: Security scanner test suite
- `tests/test-protect-main-branch.sh`: Protected branch hook test suite
- `tests/test-codex-skills.sh`: Verifies `.codex/skills/` symlinks match `plugins/*/skills/`
- `scripts/sync-codex-skills.sh`: Creates, fixes, and prunes `.codex/skills/` symlinks (`--check` for CI/verification)
- `Makefile`: Primary interface for all project operations
- `docs/mcp-security-scanner.md`: Comprehensive security scanner documentation
- `docs/protect-main-branch-hook.md`: Comprehensive branch protection documentation

## Monitoring and Debugging

**Security Scanning**: Check the runtime-generated security-scan.log in `.claude/` for:
- Hook execution: `grep "DEBUG: Hook script started"`
- Clean requests: `grep "Security scan passed"`
- Violations: `grep "SECURITY VIOLATION"`

**Branch Protection**: Check the runtime-generated protect-main-branch.log in `.claude/` for:
- Hook execution: `grep "DEBUG: Hook script started"`
- Allowed operations: `grep "INFO: Branch.*is not protected"`
- Violations: `grep "ERROR: PROTECTED BRANCH VIOLATION"`
- Blocked tools: `grep "blocked"` against that log

**MCP Coverage**: The scanner monitors all MCP servers and extracts content from various parameter fields (`thought`, `prompt`, `topic`, `libraryName`, etc.) for comprehensive security coverage.

**Branch Protection Coverage**: The hook monitors Edit, Write, Bash, and Task tools on protected branches (main, master, production, release) while allowing Read-only operations.
