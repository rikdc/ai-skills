# Review the Staged User-Summary Change

## Background

You are the senior reviewer on the **orders service**, a Go service backing
the admin dashboard. A teammate has staged a small, well-groomed change:
`store/summary.go` adds `SummarizeUsers` (an aggregation query with a
bounded result set) and a `Describe` helper, and `store/summary_test.go`
adds a table-driven test for the helper. Both files are new and staged but
**not yet committed**, so the change is visible with `git diff --staged`.

Run the setup script to prepare the repository:

```bash
bash setup.sh
```

(If the script landed in an `inputs/` directory instead of the working
directory root, run `bash inputs/setup.sh` — both work.)

**Environment notes:**

- The `go` toolchain is **not available** in this environment. Do not attempt
  to build, vet, or test. Review by reading the code and the staged diff
  (`git diff --staged`).
- No network actions are needed or possible; skip anything that would touch
  the internet or a live database.

## Your Task

1. Run the setup script to materialise the repository.
2. Read the staged change: `git diff --staged` and the full contents of
   `store/summary.go` and `store/summary_test.go` (plus
   `store/user_store.go` for context).
3. Review the change as a senior Go reviewer under the team's review
   standard. Severity tiers exist for a reason: **Critical** is for bugs,
   security vulnerabilities, incidents, and data loss; **Major** is for
   significant quality/performance/maintainability impact; **Minor** is for
   stylistic and suggestion-level notes that do not block a merge. A change
   that has no blocking findings gets an approving verdict — do not
   manufacture problems to look thorough.
4. Write the review to `review.md` in the repository root, using the
   standard review structure: summary, issues by severity tier (omit empty
   tiers or state explicitly that a tier is empty), Positive Observations,
   Questions, and an Overall Recommendation with a verdict
   (APPROVED / APPROVED WITH COMMENTS / REQUEST CHANGES) and next steps.

## Output Specification

After you finish, the repository root must contain:

- `review.md` — the full structured review of the staged change
- the change itself left staged exactly as the setup script produced it
  (do not commit, unstage, or modify the fixture code)
