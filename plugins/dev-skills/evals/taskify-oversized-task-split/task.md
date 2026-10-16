# Fix the Oversized Task in the Checkout Service Breakdown

## Background

You are on the commerce team. A colleague, Marcelo, has drafted the task
breakdown for the new checkout service, and the team lead has asked you to
review it before it goes into sprint planning. Run the setup script to
materialise his draft and the short specification it came from:

```bash
bash inputs/setup.sh
```

The working directory then contains `checkout-service-spec.md` (the spec) and
`checkout-draft.md` (Marcelo's draft breakdown).

Marcelo's draft is mostly fine — the foundation and rollout tasks follow the
team's decomposition rules. But there is one problem the team lead flagged:
task 2.1, "Build the checkout service", is a single task that bundles order
validation, the payment gateway client, receipt emails, REST handlers, and
integration tests into one 16-hour blob with a one-line acceptance criterion
("Checkout works correctly"). Marcelo insists it should stay as one task
because Samir, a senior developer, has already mentally claimed it and "can
just work through it in one go" — the draft even says so. The team's breakdown
rules say otherwise: no task may exceed 4 hours, each task must be
independently completable with a single clear outcome and unambiguous done
criteria, and large tasks must be split into subtasks with explicit
dependencies so other developers can pick up pieces in parallel.

Your job: produce the revised breakdown the team can actually schedule from,
even though a colleague prefers the oversized task stays whole.

## Your Task

1. Read `checkout-service-spec.md` and `checkout-draft.md`.
2. Review the draft against the decomposition rules: atomic 2-4 hour tasks
   with a single outcome, explicit task-ID dependencies, testable checkbox
   acceptance criteria, files to modify, and per-phase testing.
3. Fix every violation you find — in particular decompose the oversized task
   2.1 into independently completable subtasks, each with its own estimate,
   dependencies, acceptance criteria, files, and testing — while keeping the
   draft's correct tasks and phase structure intact.
4. Write the corrected full breakdown to `revised-breakdown.md` in the working
   directory, in the same structured task file format as the draft.

Do not create GitHub issues, call `gh`, or touch the network — this is a
dry-run review exercise. The revised breakdown file is the deliverable; do not
edit `checkout-draft.md`.

## Output Specification

After you finish, the working directory should contain:

- `revised-breakdown.md` — the full corrected breakdown: the draft's valid
  tasks preserved, the oversized non-atomic task 2.1 replaced by atomic
  2-4 hour subtasks with explicit dependencies and testable acceptance
  criteria, and the dependency graph / parallel tracks / critical path
  updated to match.
