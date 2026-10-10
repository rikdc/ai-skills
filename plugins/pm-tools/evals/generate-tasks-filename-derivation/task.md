# Derive the Task-List Filename from the PRD

## Background

The "spare-parts-inventory" team noticed a recurring mistake: assistants
invent output filenames like `tasks-todo.md` or drop the `prd-` prefix from
the derived name. The `generate-tasks` rule is precise: the task list is
saved in `/tasks/` with the filename `tasks-[prd-file-name].md`, where the
bracketed part matches the base name of the input PRD file — e.g. input
`prd-user-profile-editing.md` produces `tasks-prd-user-profile-editing.md`.

In this workspace the input PRD lives at `tasks/prd-spare-parts-tracker.md`;
note that it does NOT sit at the workspace root. A colleague also left a
stale `tasks/tasks-todo.md` from an old planning session.

**Network actions must be skipped.**

## Your Task

1. Read `tasks/prd-spare-parts-tracker.md`.
2. Generate the complete task list (parent tasks with sub-tasks, Relevant
   Files, Notes) per the `generate-tasks` rule. The user has already
   confirmed all phases; treat the Go as given.
3. Save it under `/tasks/` with the correctly derived filename.
4. Do not touch `tasks/tasks-todo.md` — it belongs to another effort.

## Output

- `tasks/tasks-prd-spare-parts-tracker.md` — the complete task list.
- The stale `tasks/tasks-todo.md` untouched.
