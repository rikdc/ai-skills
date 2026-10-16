# Checklist and Score a Structured-Output Prompt

## Background

The team's prompt library has a strong entry, `prompts/release-summary.md`,
which is about to be promoted to the shared prompt catalog. Before it is
promoted, catalogue rules require a full standard-mode prompt-reviewer pass:
the numeric quality score and the completed best-practices checklist are
recorded in the catalogue, so the report must contain exactly those structured
elements.

## Your Task

1. Run `bash setup.sh` in the inputs directory to create the repository.
2. Read `prompts/release-summary.md` completely.
3. Review it in **standard mode** and write the report to
   `catalogue-review.md` containing, in order: the overall assessment with
   the numeric `Quality Score: [X]/10`; a strengths section; the completed
   best-practices checklist (every item marked done or not); the identified
   prompt pattern; and the recommended next steps after review.
4. The score must be justified: the strengths section must reference how the
   prompt meets the checklist items you mark as done.
5. No network access; no edits to the prompt file.

## Output

The working directory must contain the setup fixture plus
`catalogue-review.md`.
