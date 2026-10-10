# Produce the Final Task List After "Go"

## Background

The "kyc-refresh" team uses the `generate-tasks` rule. Planning is already
half done: the parent tasks were generated and the user replied "Go", which
you'll find recorded in `go-ahead.md`. In fact, the whole flow has already
been worked through by a teammate who left mid-sprint; a partially written
final task list exists at `tasks/tasks-prd-kyc-refresh.md` with parent
tasks but no sub-tasks and no Relevant Files section.

You are applying the `generate-tasks` rule (both user confirmations are
done: the PRD's clarifying answers are embedded in the PRD, and the "Go"
authorisation is in `go-ahead.md`).

**Network actions must be skipped.**

## Your Task

1. Read `tasks/prd-kyc-refresh.md`, `go-ahead.md`, and the partial
   `tasks/tasks-prd-kyc-refresh.md`.
2. Complete the final task list: break each parent task into actionable
   sub-tasks (Phase 2), keeping the existing parent titles and numbering.
3. Follow the rule's output format exactly: a `## Relevant Files` section
   listing files to create/modify (each with a one-line why), a `### Notes`
   section, and a `## Tasks` section with `- [ ] N.0` parent items and
   nested `- [ ] N.M` sub-tasks.
4. Overwrite `tasks/tasks-prd-kyc-refresh.md` with the completed final list.

## Output

- `tasks/tasks-prd-kyc-refresh.md` — the complete task list following the
  rule's Markdown structure.
