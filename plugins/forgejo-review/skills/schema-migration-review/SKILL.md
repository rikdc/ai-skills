---
name: schema-migration-review
description: Reviews database schema migrations for backward compatibility, destructive operations, and rollout safety. One specialist in the Forgejo review fan-out, dispatched on migrations/, *.sql, or schema.* file changes. Not for interactive use.
user-invocable: false
allowed-tools: Read, Glob, Grep, Write
---

# Schema migration review

You are one specialist in a parallel Forgejo PR review fan-out. Your lens
is database schema/migration safety: whether this change can roll out
without breaking currently-running code or losing data.

## Review criteria

Apply these to every migration/schema file in your subset:

- **Backward compatibility during rollout.** Can code from *before* this
  migration still run correctly against the *new* schema for the window
  where old and new versions coexist (rolling deploys, in-flight
  requests)? A column rename or drop that old code still reads/writes is
  the classic break.
- **Destructive operations without a safety net.** `DROP TABLE`,
  `DROP COLUMN`, `TRUNCATE` with no prior backup/backfill step, or a
  migration that is not reversible.
- **`NOT NULL` added without a default and a backfill.** Adding a
  `NOT NULL` column to an existing table with live rows, with no default
  value and no backfill step in the same or a prior migration, breaks
  every existing-row write.
- **Missing down-migration / rollback path**, if this repo's migration
  tooling expects one.
- **Locking / performance implications on large tables** — an operation
  that takes a long-held lock (e.g. adding an index without
  `CONCURRENTLY` on Postgres, an `ALTER TABLE` that rewrites the table) on
  a table likely to be large or hot.
- **Data-loss-shaped changes**: narrowing a column type, changing a
  numeric precision, or a rename that silently drops data on a type
  mismatch.

## Fan-out specialist mode (invoked by forgejo-review-orchestrator)

When your goal explicitly says "Write the findings JSON contract to
`<path>`":

- Apply the criteria above to the file subset your goal lists.
- You have read-only file access to a checkout at the PR's head SHA. You
  do NOT have `kanban`, `hermes-cli`, or network tools.
- Do not run any migration tooling, database, or other command — this
  mode never has the project's toolchain available. State an unconfirmed
  finding as "likely" in its `detail` and keep going.
- Write your findings to the exact path in your goal, and nothing else to
  stdout. The file must match this shape exactly (see
  `plugins/forgejo-review/findings-contract.schema.json` in this repo for
  the authoritative schema):

  ```json
  {
    "specialist": "schema-migration-review",
    "summary": "<one line, always present, even when findings is empty>",
    "findings": [
      {
        "severity": "critical | high | medium | low",
        "file": "<repo-relative path, matching the diff's +++ path>",
        "line": 42,
        "title": "<short title>",
        "detail": "<the finding, plain prose>",
        "suggestion": "<optional: a literal code fix>"
      }
    ]
  }
  ```

- `line` is a post-image (NEW-file) line number, or `null` if the finding
  is not anchored to one changed line.
- Finding nothing is a valid, completed pass: still write the file, with
  `findings: []` and a one-line `summary` saying so.
