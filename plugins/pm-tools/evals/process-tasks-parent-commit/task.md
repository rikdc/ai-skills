# Run the Parent-Task Completion Protocol

## Background

You are closing out parent task 1.0 on the "audit-export" team, which
manages work with the `process-tasks` rules. A teammate has finished all of
parent task 1.0's sub-tasks — the code is in place and it all works — but
stopped right there; the completion protocol has NOT been followed yet.

The workspace contains the task list, an implementation tree with a temp
file that must not be committed, and a `ci.sh` script that runs the test
suite (in this sandbox it replaces the real test suite and exits 0 when
everything passes; you must actually run it). Git is NOT initialised with a
remote — there is nothing to push, and network actions must be skipped.

## Your Task

1. Read `tasks/tasks-prd-audit-export.md` and confirm all sub-tasks under
   parent 1.0 are done (they are; the checkboxes may say so).
2. Follow the rule's completion protocol for the finished parent 1.0, in
   order:
   - Run the full test suite via `bash ci.sh`.
   - Only if it passes, stage the changes.
   - Remove temporary files so they don't get committed.
   - Commit staged changes with a descriptive message using multiple
     `git commit -m` flags: conventional-commit type and subject line,
     then key changes/additions, then a reference to the task number and
     PRD context.
3. After committing, mark parent task 1.0 as `[x]` in the task list and
   update the Relevant Files section with the files created.
4. Do not start parent task 2.0.

## Output

- One commit containing the parent task's implementation (minus temp
  files) with the documented -m message structure.
- `tasks/tasks-prd-audit-export.md` — parent 1.0 marked [x] and Relevant
  Files updated.
