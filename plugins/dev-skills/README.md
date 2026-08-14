# Development Skills Plugin

Expert development skills for Go, documentation, architecture, and project management.

## Skills Included

### `/go-implementor`

Expert Go software engineer for implementing production-grade backend services with idiomatic Go patterns, testing, and observability.

**Use when**: Implementing Go code following best practices.

### `/go-review`

Senior Go code reviewer for comprehensive code reviews focused on correctness, security, performance, and maintainability.

**Use when**: Reviewing Go code for bugs, best practices, and production readiness.

### `/golang-expert`

Senior Go engineer and technical advisor covering all aspects of Go development — idiomatic patterns, concurrency, error handling, project structure, performance optimisation, observability, and testing. Operates in implement, review, debug, advise, or optimise mode depending on the task.

**Use when**: Any Go question, design decision, debugging session, performance investigation, or implementation task.

### `/document`

Technical documentation expert for creating clear, comprehensive documentation including API docs (OpenAPI), ADRs, system architecture docs, developer guides, and runbooks.

**Use when**: Creating or improving technical documentation.

### `/mentor`

Senior Staff Engineer mentor for architecture, design decisions, technical mentorship, and career guidance.

**Use when**: Seeking senior-level perspective on system design, code review, technical strategy, or career growth.

### `/manager`

Engineering manager agent for orchestrating complex software development tasks by coordinating specialized sub-agents and managing parallel work streams.

**Use when**: Large initiatives requiring coordination across multiple specialists.

### `/specify`

Software specification expert for transforming high-level designs into detailed, implementable technical specifications.

**Use when**: Converting requirements or designs into precise specs that developers can implement directly.

### `/taskify`

Task decomposition expert for breaking technical specifications into atomic, implementable tasks with dependencies and priorities.

**Use when**: Converting specs into actionable task lists for development teams.

### `/shell-script-reviewer`

Reviews Bash, Zsh, and Fish shell scripts for bugs, security vulnerabilities, portability issues, and style violations, grounded in the Google Shell Style Guide, ShellCheck, BashPitfalls, and shell-specific idiom references.

**Use when**: Reviewing, auditing, linting, or improving a `.sh`/`.bash`/`.zsh`/`.fish` file.

## Installation

Install via Claude Code marketplace:

```bash
claude code plugins install dev-skills
```

Or install from this repository:

```bash
claude code plugins install github:rikdc/ai-skills/dev-skills
```

## Usage Examples

```bash
# Implement a new Go service
/go-implementor Implement a user authentication service

# Review Go code
/go-review Review the authentication middleware for security issues

# Create API documentation
/document Create OpenAPI spec for the user service

# Get architecture guidance
/mentor Should I use microservices or monolith for this project?

# Break down a specification
/taskify Convert the authentication spec into implementable tasks

# Create detailed specification
/specify Create detailed spec for user authentication flow

# Orchestrate complex project
/manager Implement user authentication with tests and docs

# Get comprehensive Go expert guidance
/golang-expert How should I structure error handling across my service layers?

# Debug a Go concurrency issue
/golang-expert Why is my worker pool leaking goroutines?

# Review a shell script
/shell-script-reviewer Review scripts/deploy.sh for security issues
```

## License

Mozilla Public License 2.0 — see [LICENSE](../../LICENSE).
