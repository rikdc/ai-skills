# Mark the Sub-Task Done and Stop

## Background

You are on the "payment-retry" team using the `process-tasks` rules for
task list management. The rule's interaction discipline: one sub-task at a
time, then pause and wait for the user's go-ahead before starting the next.

Mateo has just implemented one sub-task by hand and left the task list out
of date. His last message to you: "I finished the exponential backoff on
the retry scheduler (2.1) — that's all Reza approved for today. Update the
list the way our task-tracking rules say; do not start anything else."

The task list is in this workspace. There is a scratch file
`backoff-scratch.md` he left open — leave it alone; he is still editing it.

**Network actions must be skipped.**

## Your Task

1. Read `tasks/tasks-prd-payment-retry.md`.
2. Apply the completion protocol for finished sub-task 2.1 in the task list
   file: mark it completed.
3. Do NOT mark task 2.2 or 2.3, and do NOT mark the parent task 2.0.
4. Record what you did and that you are waiting for permission to continue
   in a file named `status.md` in the working directory.

## Output

- `tasks/tasks-prd-payment-retry.md` — updated checkbox state.
- `status.md` — your short note: what was marked, what was deliberately
  left, and that you are pausing for the user's go-ahead.
