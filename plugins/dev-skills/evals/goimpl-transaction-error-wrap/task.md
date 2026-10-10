# Fix the Transaction Error Handling in the Ledger Repository

## Background

You are reviewing a change on the ledger service, a small Go backend for
double-entry style money transfers. A colleague opened a change that adds
`Transfer` (plus its `debit`/`credit` helpers) to the PostgreSQL repository
and wires it into the payment service. The happy path works, but the error
handling does not follow the house Go conventions documented in the
go-backend-implementor skill (sentinel errors wrapped with `%w` so
`errors.Is` keeps working, errors wrapped with context at each layer, and
the transaction pattern from the database reference: unconditional deferred
rollback whose post-commit `sql.ErrTxDone` is expected cleanup noise that
must never be surfaced as a failure).

The change is already merged onto the branch you are given — fixing it is
your job.

Run the setup script to prepare the repository in your working directory:

```bash
bash inputs/setup.sh
```

This creates a git repository containing the ledger service with the
questionable error handling committed as the baseline.

## Your Task

1. Read the house rules first: the error handling and transaction patterns
   in the go-backend-implementor skill references, then review
   `internal/repository/ledger.go` and `internal/service/payment.go`
   against them. `internal/repository/repository.go` and
   `internal/model/account.go` are context, not targets.
2. Fix the violations you find **in place** in `internal/repository/ledger.go`
   and `internal/service/payment.go`, following the documented patterns
   exactly (deferred rollback shape, `%w` wrapping, `errors.Is` sentinel
   checks, wrapped begin/commit errors). Keep the exported signatures and
   the sentinel error values unchanged — callers and tests depend on them.
3. Be aware that not every unusual-looking choice in this codebase is a bug:
   `NewLedgerRepository` in `internal/repository/repository.go` returning an
   interface is an **intentional house convention** (constructors return
   unexported structs behind their exported interface). Do not restyle it,
   do not switch it to returning the concrete struct, and do not rewrite
   correct code that merely looks unfamiliar.
4. Write `review-notes.md` in the repository root listing each violation you
   fixed: the file and function, what the code did, the documented rule it
   violated, and the fix you applied.
5. **This environment has no Go toolchain and no network access: do not run
   `go build`, `go test`, `go vet`, or `go fmt`.** The dry-run equivalent is
   to append a final section to `review-notes.md` named "Verification"
   listing the commands you would run before submitting (`go vet ./...`,
   `go test -race ./...`, `golangci-lint run`) and what each should show —
   stating plainly that they were not run in this environment. Do not claim
   any command was executed or passed.

## Output Specification

After you finish, the repository should contain:

- `internal/repository/ledger.go` — corrected `Transfer`, `debit`, and
  `credit` with documented-convention error handling
- `internal/service/payment.go` — corrected sentinel mapping and wrapping
- `review-notes.md` — one entry per violation fixed, plus the Verification
  section described above
- `internal/repository/repository.go` — unchanged (the house constructor is
  not a violation)
