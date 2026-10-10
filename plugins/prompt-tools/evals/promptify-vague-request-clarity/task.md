# Tone Down a Vague Prompt Request

## Background

A colleague pings you: *"I need a prompt that makes an AI write good
documentation summaries."* That is the entire request. The team's convention
is that prompt deliverables are never shipped with undefined quality words
like "good" — every requirement must be concrete enough for two different AI
runs to produce consistent output.

Your job is to run the `promptify` workflow for this vague request without
letting its vagueness leak into the finished prompt.

## Your Task

1. Run `bash setup.sh` in the inputs directory to prepare the scratch repo.
2. Record the missing information the request does not supply in
   `assumptions.md`: for each gap (purpose, input, output, constraints, or
   domain knowledge), state either the assumption you are making or the
   clarifying question you would ask the requester — do not leave blanks.
3. Author the finished prompt in `summary-prompt.md`. Wherever the request
   was vague, replace it with a concrete, checkable requirement implied by
   your stated assumptions, and state in the prompt what the AI should do
   if the input is unclear (error-handling path).
4. Everything stays offline; do not contact any external service.

## Output

The working directory must contain exactly `assumptions.md` and
`summary-prompt.md`; do not add other files.
