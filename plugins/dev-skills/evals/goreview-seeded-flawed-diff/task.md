# Review the Staged CSV Export Change

## Background

You are the senior reviewer on the **orders service**, a Go service backing
the admin dashboard. A teammate has just staged a change that adds a CSV
order-export endpoint: `store/export.go` is new and sits on top of the
existing, clean `store/user_store.go`. The change is staged but **not yet
committed**, so it is visible with `git diff --staged`.

Run the setup script to prepare the repository:

```bash
bash setup.sh
```

(If the script landed in an `inputs/` directory instead of the working
directory root, run `bash inputs/setup.sh` — both work.)

The change claims to "add an admin CSV export for one user's orders, with
fast parallel label lookup".

**Environment notes:**

- The `go` toolchain is **not available** in this environment. Do not attempt
  to build, vet, or test. The review must be done by reading the code and the
  staged diff (`git diff --staged`).
- No network actions are needed or possible; skip anything that would touch
  the internet or a live database. The SQL is reviewed as text.

## Your Task

1. Run the setup script to materialise the repository.
2. Read the staged change: `git diff --staged` and the full contents of
   `store/export.go` (and `store/user_store.go` for context).
3. Review the change as a senior Go reviewer, following the team's review
   standard: correctness first, then security, then performance, then
   maintainability. Classify every finding with a severity of **Critical**,
   **Major**, or **Minor**, cite the location (file and function, ideally
   line), state the impact, and give an actionable recommendation with a
   concrete fix.
4. Write the review to `review.md` in the repository root, using the
   standard review structure: per-file summary, Critical Issues, Major
   Issues, Minor Issues, Positive Observations, and an Overall
   Recommendation with a verdict (APPROVED / APPROVED WITH COMMENTS /
   REQUEST CHANGES) and next steps.

## Output Specification

After you finish, the repository root must contain:

- `review.md` — the full structured review of the staged change
- the change itself left staged exactly as the setup script produced it
  (do not commit, unstage, or modify the fixture code)
