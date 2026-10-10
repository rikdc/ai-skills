# Generate a Security Review Prompt for the Go Bot

## Background

You are working with a colleague who maintains a set of internal AI prompts
used across the engineering team. The team is building an automated Go code
security reviewer and needs a prompt that a code-review LLM agent can follow.
The agent will receive a single Go source file and must return a structured
vulnerability report the team can paste into their review tracker.

Your job is to run the `promptify` workflow for this request: analyse it, pick
an appropriate prompt pattern, and author the finished prompt.

## Your Task

1. Create a scratch repository for the deliverables by running
   `bash setup.sh` in the inputs directory.
2. **Analyse the request** and record your analysis in `analysis.md`, covering
   exactly these fields: Purpose, Input, Output, Domain, Complexity, and
   Best Pattern (choose one of: Role-Based, Task-Oriented, Few-Shot Learning,
   Chain-of-Thought, Structured Output). Justify the pattern choice in one or
   two sentences.
3. **Author the generated prompt** in `generated-prompt.md`. It must be a
   complete, ready-to-use prompt a CI team could paste into their agent with
   no further editing.
4. Do not attempt to contact any external service; everything is offline.

## Output

The working directory must contain exactly these two artifacts
(`analysis.md` and `generated-prompt.md`); do not add other files.
