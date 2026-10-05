# Protected Branch Hook

## Overview

The Protected Branch Hook prevents direct edits to critical branches (main, master, production, release) by blocking tool executions that could modify code or execute commands on protected branches. This enforces PR-based workflows and ensures code quality through proper review processes.

## Features

- **Branch Protection**: Blocks Edit, Write, Bash, and Task tools on protected branches
- **Configurable Protection**: Easy to add/remove protected branch patterns
- **Clear Guidance**: Provides actionable error messages with workflow instructions
- **Emergency Override**: Supports override for critical situations
- **Audit Logging**: All violations logged to `.claude/protect-main-branch.log`
- **Zero False Positives**: Only triggers on protected branches

## Protected Branches

By default, the following branches are protected:

- `main`
- `master`
- `production`
- `release`

## How It Works

The hook runs on `PreToolUse` for tools that can modify code or execute commands:

- `Edit` - File editing operations
- `Write` - File creation operations
- `Bash` - Command execution
- `Task` - Agent task spawning

When triggered on a protected branch, the hook:

1. Works out where the write lands and reads the branch checked out there:
   - `Edit` / `Write`: the work tree containing `tool_input.file_path`
   - `Bash`: the session directory (`cwd` in the hook payload), moved by any
     `cd <dir>` segment and by `git -C <dir>`, judged per write segment
   - `Task`: the session directory
2. Checks if that branch matches the protected pattern
3. Blocks the operation and provides guidance if protected
4. Allows the operation to proceed if not protected

## Usage

### Normal Workflow

When on a protected branch, Claude Code will be blocked from making changes:

```text
❌ BLOCKED: Direct edits to protected branch 'main' are not allowed.

To proceed:
1. Create a feature branch:
   git checkout -b your-name/feature-description

2. Make your changes on the feature branch

3. Push and create a Pull Request:
   git push -u origin your-name/feature-description
```

### Worktrees

Because the branch is read where the write lands, a session started in a
repository's main checkout can edit files in a linked worktree that is on a
feature branch, and `cd <worktree> && git commit` is allowed. The reverse is
still blocked: a session in a worktree cannot `cd` into the main checkout and
push. A `cd` or `git -C` target the hook cannot resolve statically (a
variable, command substitution, a missing path) leaves the directory
unchanged, so it errs toward blocking.

### Creating a Feature Branch

Follow your team's branching conventions:

```bash
# With issue tracker ticket
git checkout -b your-name/PROJ-123-feature-description

# Without ticket
git checkout -b your-name/feature-description
```

### Emergency Override

For critical hotfixes, you can override the protection:

```bash
# Set environment variable before starting Claude Code
export ALLOW_PROTECTED_BRANCH_EDIT=1

# Or as a one-time override
ALLOW_PROTECTED_BRANCH_EDIT=1 claude-code
```

**Note**: Overrides are logged and should be used sparingly with proper justification.

## Configuration

### Adding Protected Branches

Edit `.claude/hooks/protect-main-branch.sh`:

```bash
PROTECTED_BRANCHES=("main" "master" "production" "release" "staging")
PROTECTED_PATTERN="^(main|master|production|release|staging)$"
```

### Customizing Protected Tools

Edit `.claude/settings.json` to modify which tools are protected:

```json
{
  "matcher": "(Edit|Write|Bash|Task|Read).*",
  "hooks": [
    {
      "type": "command",
      "command": "${WORKSPACE}/.claude/hooks/protect-main-branch.sh"
    }
  ]
}
```

## Monitoring

Check the log file for hook activity:

```bash
# View recent violations
tail -f .claude/protect-main-branch.log

# Search for specific violations
grep "PROTECTED BRANCH VIOLATION" .claude/protect-main-branch.log

# Count violations by branch
grep "PROTECTED BRANCH VIOLATION" .claude/protect-main-branch.log | \
    awk -F"'" '{print $4}' | sort | uniq -c
```

## Benefits

### Code Quality

- Ensures all changes go through code review
- Validates changes against CI/CD pipelines
- Maintains consistent quality standards

### Collaboration

- Encourages knowledge sharing through PRs
- Creates audit trail for all changes
- Prevents accidental direct commits

### Risk Management

- Reduces production incidents from unreviewed code
- Enforces testing requirements
- Maintains deployment safety

## Integration with CI/CD

This hook complements CI/CD workflows by:

- Preventing bypassing of PR checks
- Ensuring branch protection rules are respected
- Maintaining deployment pipeline integrity

## Troubleshooting

### Hook Not Triggering

1. Check Claude Code settings:

   ```bash
   cat .claude/settings.json
   ```

2. Verify hook is executable:

   ```bash
   ls -l .claude/hooks/protect-main-branch.sh
   ```

3. Restart Claude Code after configuration changes

### False Positives

If the hook triggers unexpectedly:

1. Check your current branch:

   ```bash
   git branch --show-current
   ```

2. Review protected branch patterns in the hook script

3. Check the log for details:

   ```bash
   tail .claude/protect-main-branch.log
   ```

### Permission Issues

If you see permission errors:

```bash
chmod +x .claude/hooks/protect-main-branch.sh
```

## Best Practices

### Branch Naming

Follow your team's conventions:

- Use lowercase with dashes
- Include your name: `your-name/feature-description`
- Include issue ticket when applicable: `your-name/PROJ-123-feature`

### Feature Branch Workflow

1. Create feature branch from main
2. Make changes and commit frequently
3. Push to remote and create PR
4. Address review feedback
5. Merge after approval and CI passes

### Emergency Hotfixes

1. Document the emergency in PR description
2. Get expedited review from team lead
3. Monitor deployment closely
4. Follow up with proper testing

## Examples

### Successful Workflow

```bash
# On main branch - Claude Code is blocked
git checkout main

# Create feature branch
git checkout -b alice/PROJ-456-add-validation

# Now Claude Code can make changes
# ... make changes ...

# Commit and push
git add .
git commit -m "feat: add input validation"
git push -u origin alice/PROJ-456-add-validation

# Create PR through GitHub UI or CLI
gh pr create --title "feat: add input validation"
```

### Emergency Override Example

```bash
# Critical production issue requires immediate fix
export ALLOW_PROTECTED_BRANCH_EDIT=1

# Make emergency fix on main
git checkout main
# ... use Claude Code to make fix ...

# Document in commit message
git commit -m "hotfix: resolve critical issue [emergency-override]"

# Remove override after fix
unset ALLOW_PROTECTED_BRANCH_EDIT
```

## Related Documentation

- [Claude Code Hooks](https://docs.claude.ai/docs/hooks)
- [MCP Security Scanner](./mcp-security-scanner.md)

## Support

For issues or questions:

- Check the log file: `.claude/protect-main-branch.log`
- Review this documentation
- Consult your team's workflow documentation
