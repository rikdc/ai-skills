# Prompt Tools Plugin

AI prompt generation and review tools for LLM development.

## Commands Included

### `/prompt-reviewer`

Review and improve AI prompts with expert feedback on clarity, effectiveness, and best practices.

**Features**:

- Analyzes prompt structure and clarity
- Identifies ambiguities and edge cases
- Suggests improvements for better results
- Provides best practices guidance
- Evaluates prompt engineering techniques

### `/promptify`

Generate high-quality prompts for AI systems, LLMs, and agents with proven patterns and best practices.

**Features**:

- Creates prompts from high-level requirements
- Applies prompt engineering best practices
- Includes context and constraints
- Optimizes for specific LLM models
- Supports various prompt formats (chat, completion, system)

## Installation

Install via Claude Code marketplace:

```bash
claude code plugins install prompt-tools
```

Or install from this repository:

```bash
claude code plugins install github:rikdc/ai-skills/prompt-tools
```

## Usage Examples

```bash
# Review an existing prompt
/prompt-reviewer Review this prompt: "Write a function to sort an array"

# Generate a new prompt
/promptify Create a prompt for generating API documentation

# Improve prompt for specific use case
/promptify Generate a code review prompt for Go services
```

## Best Practices

### When to Use `/prompt-reviewer`

- Before deploying prompts to production
- When prompt results are inconsistent
- To optimize prompt performance
- To learn prompt engineering best practices

### When to Use `/promptify`

- Starting a new AI-powered feature
- Converting requirements to prompts
- Optimizing existing prompts
- Creating system prompts for agents

## License

Mozilla Public License 2.0 — see [LICENSE](../../LICENSE).
