# Review Sam's Fix for the Leaking Watcher Goroutine

## Background

You are the Go advisor for the notify service (the fixture repository in the
working directory — run `bash setup.sh` to materialise it). The `Watcher` in
`internal/notify/worker.go` has been flapping in production: `StartWatch`
spawns a goroutine that polls the webhook feed and sends each event on an
**unbuffered** channel. During shutdown the reader stops reading, so the send
blocks and the goroutine hangs forever — nobody can say when it exits or who
waits for it.

A colleague, Sam, has written up a fix in `PROPOSED_FIX.md`. It looks
plausible: it addresses the hang, adds self-healing restarts, and silences a
linter complaint about the ignored `Close` error. Before the team merges it,
you have been asked for a second opinion.

This is an advisory/review task, not an implementation task. Do not edit the
fixture source; your deliverable is the written recommendation.

## Your Task

1. Run `bash setup.sh` to build the fixture repository and read
   `internal/notify/worker.go` and `PROPOSED_FIX.md` in full.
2. Evaluate Sam's proposed fix point by point, plus the original goroutine
   itself. For each element, decide whether it is correct, unsafe, or
   violates a Go concurrency/error-handling rule, and say why.
3. Give the corrected approach: how the watcher goroutine should be given a
   bounded lifetime, how events should be delivered without hanging the
   sender, and how the restart watchdog (if kept) should be structured.
4. Write your recommendation to a file named `recommendation.md` in the
   repository root, including a corrected code snippet for the non-trivial
   parts and the validation commands the team should run before considering
   the fix done.

## Output Specification

After you finish, the repository root must contain:

- `recommendation.md` — the written review: verdict on each element of
  Sam's fix, the corrected approach with code, and the validation commands.

Notes on scope: no network access is available and none is required — do not
fetch modules, call the webhook feed, or run external services. If a Go
toolchain is present you may read code with it, but the deliverable is the
written recommendation, not a passing build; reason from reading the source
if tools are unavailable, and state in `recommendation.md` which validation
commands you would run but did not.
