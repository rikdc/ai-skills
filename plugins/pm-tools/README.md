# Project Management Tools Plugin

Project management commands for PRDs, task generation, and task tracking.

## Commands Included

### `/pm:create-prd`

Generates a Product Requirements Document (PRD) following industry best practices.

**Features**:

- Structured PRD format with all essential sections
- User stories and acceptance criteria
- Success metrics and KPIs
- Technical considerations

### `/pm:generate-tasks`

Generates a task list from a PRD with priorities and dependencies.

**Features**:

- Breaks down PRD into atomic tasks
- Assigns priorities (P0, P1, P2, P3)
- Identifies task dependencies
- Estimates complexity

### `/pm:process-tasks`

Task list management for tracking progress and updating status.

**Features**:

- Mark tasks as in-progress or completed
- Track blockers and dependencies
- Generate progress reports
- Update task priorities

## Installation

Install via Claude Code marketplace:

```bash
claude code plugins install pm-tools
```

Or install from this repository:

```bash
claude code plugins install github:rikdc/ai-skills/pm-tools
```

## Usage Examples

```bash
# Create a PRD
/pm:create-prd Create PRD for user authentication feature

# Generate tasks from PRD
/pm:generate-tasks Generate tasks from the authentication PRD

# Process and update tasks
/pm:process-tasks Mark authentication tasks as completed
```

## Workflow

1. **Create PRD**: Define the feature or project requirements
2. **Generate Tasks**: Break down the PRD into actionable tasks
3. **Process Tasks**: Track progress and manage task lifecycle

## License

Mozilla Public License 2.0 — see [LICENSE](../../LICENSE).
