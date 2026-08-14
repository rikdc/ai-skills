# Code Quality Plugin

Code quality analysis, review, and cleanup commands.

## Commands Included

### `/dev:check`

Comprehensive code quality analysis and auto-fix with parallel sub-task strategy.

**Features**:

- Static analysis and linting
- Security vulnerability scanning
- Code complexity analysis
- Auto-fix for common issues
- Parallel execution for speed

### `/dev:review`

Performs a comprehensive code review of the repository.

**Features**:

- Best practices review
- Code organization analysis
- Security review
- Performance optimization suggestions
- Maintainability assessment

### `/clean`

Removes redundant and obvious comments from codebase.

**Features**:

- Identifies redundant comments
- Preserves important documentation
- Improves code readability
- Safe automated cleanup

## Installation

Install via Claude Code marketplace:

```bash
claude code plugins install code-quality
```

Or install from this repository:

```bash
claude code plugins install github:rikdc/ai-skills/code-quality
```

## Usage Examples

```bash
# Check and fix code quality issues
/dev:check

# Perform comprehensive code review
/dev:review

# Clean up redundant comments
/clean
```

## Best Practices

1. Run `/dev:check` before committing changes
2. Use `/dev:review` before major releases
3. Run `/clean` periodically to maintain code cleanliness

## License

Mozilla Public License 2.0 — see [LICENSE](../../LICENSE).
