# Clarifying Questions Come Before the PRD

## Background

You are helping a product-minded colleague on the "ledger-sync" project. Your
colleague stops by with a single sentence: "We need a feature that retries
failed ledger syncs overnight."

You have been given the `create-prd` rule for generating a Product
Requirements Document. The rule is explicit about the order of operations:
you must ask questions first and only write the PRD once you have answers.

The user has NOT provided any answers yet. No clarifying dialogue has taken
place.

**Network actions must be skipped.** Do not fetch anything; work purely from
the prompt and your clarifying questions.

## Your Task

1. Ask the user a set of clarifying questions before writing any PRD.
2. Record the questions in a file named `clarifying-questions.md` in the
   current working directory, listing each question under an appropriate
   heading (e.g. Problem/Goal, Target User, Core Functionality, User
   Stories, Acceptance Criteria, Scope/Boundaries, Edge Cases). Number the
   options within each question as letter or number lists so the user can
   respond easily with a selection.
3. Do NOT create the PRD file yet. You must not write `/tasks/prd-*.md` in
   this step; the PRD only happens after the user answers.

## Output

- `clarifying-questions.md` — the questions you would ask, in the order the
  rule prescribes, with options formatted for easy reply.
- No file under `/tasks/` — evidence you respected the ordering.
