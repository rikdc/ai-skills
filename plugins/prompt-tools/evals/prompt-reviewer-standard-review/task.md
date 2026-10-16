# Standard-Mode Review of the SQL Generator Prompt

## Background

Your team keeps its AI prompts in `prompts/` of this repository. Someone
committed `prompts/sql-generator.md` right before going on leave; the docs
bot built from it returns wildly different SQL for the same question. A
reviews-runner asked you to review it with the `prompt-reviewer` command in
**standard mode** (direct, actionable feedback) and leave the review on disk
so the absent teammate can act on it.

## Your Task

1. Run `bash setup.sh` in the inputs directory. It creates the repository
   including the flawed `prompts/sql-generator.md`.
2. Read `prompts/sql-generator.md` completely before judging anything.
3. Perform the prompt-reviewer review **in standard mode** and write the
   review to `review-report.md` using the command's standard review format:
   an overall assessment with a quality score, prioritised issue sections,
   the best-practices checklist, and concrete revised prompt sections for
   the critical problems.
4. Identify which documented prompt pattern is in use and whether it fits.
5. No network access; review the file as-is.

## Output

The working directory must contain exactly the setup fixture plus
`review-report.md`; do not modify `prompts/sql-generator.md` itself.
