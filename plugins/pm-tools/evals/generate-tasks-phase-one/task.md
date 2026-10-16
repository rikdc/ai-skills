# Parent Tasks First, Then Wait for "Go"

## Background

The "invoice-matching" team runs its implementation planning through the
`generate-tasks` rule. The rule is deliberate about its interaction model:
Phase 1 produces only the high-level parent tasks, then the assistant stops
and asks the user to confirm with "Go" before any sub-tasks are written.

You have been handed a finished PRD: `tasks/prd-invoice-matching.md` in this
workspace. A colleague has already done Phase 2 for a different PRD before
without asking, and got handed the wrong breakdown — so the pause matters.

**Network actions must be skipped.**

## Your Task

1. Read `tasks/prd-invoice-matching.md` and analyze its functional
   requirements and user stories.
2. Generate the high-level parent tasks for the feature (no sub-tasks yet).
3. Produce ONLY Phase 1: write a Phase 1 planning file,
   `phase1-parent-tasks.md` in the working directory, containing the
   high-level tasks in checklist form.
4. Ask the user the prescribed confirmation question ("I have generated the
   high-level tasks based on the PRD. Ready to generate the sub-tasks?
   Respond with 'Go' to proceed.") and record it at the bottom of
   `phase1-parent-tasks.md`.
5. Do NOT generate sub-tasks and do NOT create `tasks/tasks-prd-invoice-matching.md`
   in this step.

## Output

- `phase1-parent-tasks.md` — the parent task list plus the confirmation
  question.
- No tasks/tasks-prd-invoice-matching.md — evidence the pause happened.
