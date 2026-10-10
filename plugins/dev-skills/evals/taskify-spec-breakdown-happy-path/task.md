# Break Down the Search Service Spec into an Atomic Task Plan

## Background

You are on the platform team at a company building an in-app document search
service. A colleague, Priya, has written the specification for the service and
dropped it in the working directory as `search-service-spec.md`. Run the setup
script to materialise it:

```bash
bash inputs/setup.sh
```

The team runs sprints straight out of task breakdown files. Tasks get handed
across three developers who do not talk daily, so every task in a breakdown
must be something one developer can pick up and finish in a single sitting,
with its dependencies, files to touch, and testing written down explicitly.
Vague or bundled tasks have repeatedly caused work to stall mid-sprint, so the
team treats the decomposition rules as hard requirements, not suggestions.

Your job: turn Priya's specification into a structured task breakdown the team
can schedule from.

## Your Task

1. Read the entire specification (`search-service-spec.md`) before writing
   anything, so you understand all components and their natural ordering.
2. Group the work into logical phases (foundation first, integration and
   testing later), and break each phase into tasks that are independently
   completable in a 2-4 hour sitting with a single clear outcome.
3. For every task, record: priority, effort estimate, explicit dependencies
   (task IDs, or "None"), testable acceptance criteria as checkboxes, the
   files to create or modify, and the testing required.
4. Add the dependency graph, the parallel work tracks, and the critical path
   so the team can see what can run concurrently and what gates delivery.
5. Write the result to `task-breakdown.md` in the working directory, using the
   structured task file format (Option 2) — the "Project Task Breakdown"
   document with tasks by phase, not a list of GitHub issue templates.

Do not create GitHub issues, call `gh`, or touch the network — this is a
dry-run exercise. The structured task file is the deliverable.

## Output Specification

After you finish, the working directory should contain:

- `task-breakdown.md` — the complete task breakdown generated from
  `search-service-spec.md`, organised by phase with per-task priority, effort,
  dependencies, acceptance criteria, files, and testing, plus a dependency
  graph, parallel work opportunities, and a critical path.
