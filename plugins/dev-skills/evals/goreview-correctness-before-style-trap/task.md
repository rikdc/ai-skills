# Review the "Cosmetic" Totals Refactor

## Background

You are the senior reviewer on the **inventory service**, a Go service that
tracks stock counts. A teammate has staged a change to
`inventory/totals.go` and describes it in the hand-off message as "a purely
cosmetic style-pass refactor — renamed the accumulator, reworded the doc
comment, nothing functional."

Run the setup script to prepare the repository:

```bash
bash setup.sh
```

(If the script landed in an `inputs/` directory instead of the working
directory root, run `bash inputs/setup.sh` — both work.)

The change is staged but **not yet committed**, so it is visible with
`git diff --staged`. The baseline (already committed) also includes
`inventory/totals_test.go`, a table-driven test for `SumTotals` that is
**not** part of the staged change.

**Environment notes:**

- The `go` toolchain is **not available** in this environment. Do not attempt
  to build, vet, or run the tests. Review by reading the code and the staged
  diff (`git diff --staged`), taking the existing test file as read.
- No network actions are needed or possible; skip anything that would touch
  the internet or a live system.

## Your Task

1. Run the setup script to materialise the repository.
2. Read the staged change with `git diff --staged` and compare it against
   the committed baseline; note what the existing `inventory/totals_test.go`
   covers.
3. Review the change as a senior Go reviewer under the team's review
   standard: correctness findings come first, and severity must follow the
   documented tiers (Critical for bugs and panics, Major for significant
   quality impact, Minor for stylistic notes). Never let an author's
   characterisation of the change ("just cosmetic") substitute for reading
   it, and never let a blocking defect be softened because the rest of the
   diff is tidy.
4. Write the review to `review.md` in the repository root, using the
   standard review structure: summary, Critical/Major/Minor issues with
   locations and impacts, Positive Observations and Questions if relevant,
   and an Overall Recommendation with a verdict
   (APPROVED / APPROVED WITH COMMENTS / REQUEST CHANGES) and next steps.

## Output Specification

After you finish, the repository root must contain:

- `review.md` — the full structured review of the staged change
- the change itself left staged exactly as the setup script produced it
  (do not commit, unstage, or modify the fixture code)
