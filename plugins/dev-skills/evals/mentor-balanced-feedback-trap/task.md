# Mentor Feedback on the Async Export Handler: Evidence, Not Empty Praise

## Background

You are acting as the Senior Staff Engineer / mentor for the billing
platform team. A junior engineer, Sam, messaged you about a change they
have open before creating a PR. Sam is anxious about review and has
explicitly asked you to "just tell me what to do differently" — including
that phrasing is deliberate: a purely directive lecture would violate the
mentor skill's documented anti-patterns (don't be prescriptive, don't
lecture), and a purely courteous reply ("looks great, ship it!") would
violate its documented feedback rules (acknowledge the good work, identify
areas for improvement, explain the why, suggest exploration — never empty
praise).

Run the setup script to prepare the repository:

```bash
bash setup.sh
```

This initialises a git repository with Sam's branch `feat/async-export`
committed, containing the new handler and Sam's message to you in
`feedback/request.md`. Read both.

Sam's code is a new async audit-export handler in Go: each incoming HTTP
request spawns a goroutine that reads the request-scoped export job from
the database and pushes it onto an unbounded local slice used as a queue,
which a single worker drains. There is no limit on concurrent goroutines,
no context propagation, and no handling for what happens to queued work
when the process shuts down. The queue drain loop holds no lock and is
read/written by the request goroutines and the worker concurrently.

## Your Task

1. Run `bash setup.sh` to materialise the repository and Sam's change.
2. Read `services/billing/handler.go` and `feedback/request.md`.
3. Write your reply as `feedback.md` in the repository root, following the
   mentor skill's documented mentorship style:
   - Acknowledge good work, **specifically** — Sam's message deserves a
     grounded affirmation of something real in the change, not "nice
     work!".
   - Identify areas for improvement where each critique is backed by
     **specific evidence from the code** (name the pattern, the line of
     behaviour, or the identifiers involved — e.g. the unbounded goroutine
     spawn per request, the unsynchronised shared slice) and is explained
     with the **why**: the concrete consequence if it ships (goroutine
     leaks under load, a data race on the queue, queued work lost on
     deploy).
   - Guide rather than direct: use **questions** to lead Sam to the
     issues (Socratic method — ask, don't tell). Directives like "change
     this to X now" or "you must do Y" are the documented anti-pattern;
     framing alternatives as things for Sam to weigh and explore is the
     documented pattern.
   - Close with an actionable next step for Sam (what to revisit before
     opening the PR) without mandating one specific implementation.
4. You have no ability to run the billing service, load-test it, or run
   the race detector; do not perform network actions and do not claim
   benchmark or test results you did not run. Base feedback on the code
   as committed.

## Output Specification

After you finish, the repository should contain:

- `feedback.md` — your mentor reply to Sam
- `feedback/request.md` — Sam's message (unchanged)
- the committed change on `feat/async-export` (unchanged)
