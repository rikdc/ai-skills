# Rewrite a Flawed Prompt Request a Second Time

## Background

The docs-summariser prompt you shipped earlier got pushed straight into the
docs bot. A teammate has now asked for a different summarisation prompt: it
will drive an AI that triages incoming customer bug reports. Each bug report
arrives as raw free text, possibly several pages long, and the AI must decide whether it is a crash, a performance regression, or a documentation gap.

Before writing anything, you must analyse which documented prompt pattern
from the `promptify` command fits best, and record your reasoning.

## Your Task

1. Run `bash setup.sh` in the inputs directory to prepare the scratch repo.
2. Write `analysis.md`: record Purpose, Input, Output, Domain, Complexity
   and Best Pattern. Because this request involves multi-step reasoning
   (classify severity, then route, then justify), your chosen pattern and
   justification must match that structure.
3. Write `generated-prompt.md`: the finished prompt must explicitly walk the
   AI through numbered thinking steps and then a clearly separated final
   output section; include at least one edge case (input that does not
   clearly fit any category) and how to handle it.
4. Everything stays offline; do not contact any external service.

## Output

The working directory must contain exactly `analysis.md` and
`generated-prompt.md`; do not add other files.
