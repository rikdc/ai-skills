# Security Hooks Plugin

MCP security scanner and protected branch hooks for Claude Code projects.

## Features

### 1. MCP Security Scanner

- Scans all MCP tool requests for sensitive data before execution
- Detects API keys, tokens, credentials, database URLs, and personal information
- Logs all security violations to `.claude/security-scan.log`
- Supports optional external tools (trufflehog, gitleaks, git-secrets)

### 2. Protected Branch Hook

- Prevents direct edits to protected branches (main, master, production, release)
- Blocks Edit, Write, Bash, and Task tools on protected branches
- Enforces PR-based workflows
- Logs all violations to `.claude/protect-main-branch.log`

## Installation

Install via Claude Code marketplace:

```bash
claude code plugins install security-hooks
```

Or install from this repository:

```bash
claude code plugins install github:rikdc/ai-skills/security-hooks
```

## Configuration

### Security Patterns

The plugin includes a `security-patterns.conf` file with regex patterns for detecting:

- Authentication & API keys (AWS, GitHub, OpenAI, Slack, Discord, etc.)
- Database connection strings (PostgreSQL, MySQL, MongoDB)
- Personal information (emails, credit cards, SSNs, phone numbers)
- Cloud provider credentials and private keys

You can customize patterns by editing `security-patterns.conf` in the plugin directory.

### Hook Behavior

Both hooks run automatically when:

- **MCP Scanner**: Any MCP tool is called (pattern: `mcp__.*`)
- **Branch Protection**: Edit, Write, Bash, or Task tools are called on protected branches

## Usage

### Security Scanner

The scanner runs automatically on all MCP requests. Check the log for violations:

```bash
cat .claude/security-scan.log | grep "SECURITY VIOLATION"
```

### Branch Protection

Create a feature branch before making changes:

```bash
git checkout -b feature/my-changes
```

The hook will block operations on protected branches with guidance to create a feature branch.

## Dependencies

**Required**: `jq`, `grep`, `awk`, `mktemp`

**Optional** (enhance detection):

- `trufflehog`: Advanced secret scanning
- `gitleaks`: Git secret detection
- `git-secrets`: Prevent secrets in commits

## Monitoring

### Security Scan Log

Location: `.claude/security-scan.log`

View recent activity:

```bash
tail -n 50 .claude/security-scan.log
```

### Branch Protection Log

Location: `.claude/protect-main-branch.log`

View recent blocks:

```bash
grep "PROTECTED BRANCH VIOLATION" .claude/protect-main-branch.log
```

## License

Mozilla Public License 2.0 — see [LICENSE](../../LICENSE).
