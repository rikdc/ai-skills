# Advise the Audit Team: Consumer-Defined Interface and the Flush Loop

## Background

You are the Go advisor for the ledger service (the fixture repository in the
working directory — run `bash setup.sh` to materialise it). The team is
integrating the `internal/audit` package with `internal/ledger`:

- `internal/ledger/ledger.go` holds the concrete `PostgresLedger` store with
  seven methods (`AppendEntry`, `AppendEntries`, `Balance`, `Entries`,
  `CloseAccounts`, `PurgeOldEntries`, `HealthCheck`).
- `internal/audit/audit.go` has a `Collector` that buffers audit events in a
  channel and needs a periodic flush loop (`FlushLoop`) that drains the queue
  and writes the buffered entries to the ledger every 5 seconds, shutting down
  cleanly when the service stops.
- `TEAM_NOTES.md` records the proposal from planning: define a `LedgerStore`
  interface **inside `internal/ledger`** listing all seven methods, so the
  producer package owns the abstraction and consumers get the whole thing for
  mocking.

This is an advisory task, not an implementation task. No code changes are
required.

## Your Task

1. Run `bash setup.sh` to build the fixture repository and read
   `internal/ledger/ledger.go`, `internal/audit/audit.go`, and
   `TEAM_NOTES.md`.
2. Evaluate the team's interface proposal against good Go design practice:
   where the interface should be defined, how wide it should be, and what the
   `Collector` constructor should accept and return.
3. Design the flush loop: how the goroutine is started, how it drains the
   queue every 5 seconds, and how it exits cleanly on shutdown.
4. Write your recommendation to a file named `design.md` in the repository
   root. Lead with code where a snippet makes the recommendation concrete,
   and state the trade-offs for any decision where an alternative exists.

## Output Specification

After you finish, the repository root must contain:

- `design.md` — the written advisory covering the interface decision and the
  flush-loop design, with code snippets for the non-obvious parts.

Notes on scope: no network access is available and none is required — do not
fetch modules or run external services. If a Go toolchain is present you may
read code with it, but the deliverable is the written design, not a passing
build; reason from reading the source if tools are unavailable, and say
plainly in `design.md` which validation commands you would run but did not.
