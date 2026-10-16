# Write the Full PRD from the Prompt and Answers

## Background

You are supporting the "project-pulse" team. A feature request came in:
"Add an expense-report export that lets managers download their team's
expenses as CSV for a chosen month."

The team has already collected the answers to the standard clarifying
questions. They are recorded in a file you must read first:

- `initial-prompt.txt` — the original feature request.
- `answers.md` — the user's answers (target user, goals, scope, metrics).

You are applying the `create-prd` rule for generating Product Requirements
Documents. All clarifying work is done, so your job is the PRD itself.

**Network actions must be skipped.** Work only from the two fixture files.

## Your Task

1. Read `initial-prompt.txt` and `answers.md`.
2. Generate a PRD for the expense-report export feature following the rule's
   required structure.
3. Save it as `/tasks/prd-expense-report-export.md` (the `/tasks` directory
   means the `tasks/` directory in the current working directory).
4. Write the PRD for a junior developer: explicit, unambiguous requirements
   in plain language, minimal jargon. Do not begin implementation.

## Output

- `tasks/prd-expense-report-export.md` — the generated PRD containing:
  Introduction/Overview, Goals, User Stories, Functional Requirements,
  Non-Goals (Out of Scope), Design Considerations, Technical Considerations,
  Success Metrics, Open Questions.
