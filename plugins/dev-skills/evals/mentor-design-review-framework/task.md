# Senior Review of the Scheduled Audit Log Export Design

## Background

You are the Senior Staff Engineer on the payments platform team. A
mid-level engineer on your team has just finished a design document for the
nightly audit-log export the compliance team has been asking for, and has
asked you for a senior-level design review before the team's architecture
review board meets on Thursday.

The repository in the working directory contains the design document at
`docs/design-audit-export.md`. Run the setup script to prepare the
repository:

```bash
bash setup.sh
```

This initialises a git repository with the design document committed on
`main`, so you can review it in place.

The design under review proposes: an in-process scheduler in the existing
billing service runs a nightly job; the job issues a `SELECT ... BETWEEN`
query against the `audit_events` table in the same PostgreSQL database the
service already uses, converts the rows to CSV in memory, uploads the file
to an S3 bucket with a per-run job ID in the key, and posts a success
message to a Slack webhook. The document mentions the existing index on
`created_at` and an idempotency guard keyed on the run ID, but says nothing
about what happens when the upload fails mid-run or when the table grows.

Your job: perform the design review **as the mentor skill prescribes** —
using the skill's System Design Review framework, with Socratic questions
and non-prescriptive guidance — and write the review to a file.

## Your Task

1. Run `bash setup.sh` to materialise the repository with the design
   document.
2. Read `docs/design-audit-export.md` carefully.
3. Produce the review as `design-review.md` in the repository root,
   following the mentor skill's System Design Review framework:
   a. A **Context** section capturing the business goal, the scale
      requirements the design implies, and the constraints it assumes.
   b. An architecture analysis with a **What I Like** section citing
      *specific* strengths of the design (not generic praise), and a
      **Potential Concerns** section where each concern states the
      **Issue**, the **Impact** if unaddressed, **Considerations** with at
      least two options and their trade-offs, and a **Recommendation** with
      reasoning.
   c. A **Questions to Explore** section: open questions the author should
      answer, phrased as questions (Socratic, ask-don't-tell) rather than
      instructions.
   d. A **Looking Ahead** section covering short-term, medium-term, and
      long-term horizons.
   e. A final **My Recommendation** with specific next steps and the
      reasoning behind them.
4. Keep the mentor tone throughout: honest about concerns, humble about
   certainty, non-prescriptive — concerns are offered as options to weigh,
   never as orders ("do it this way"), and never dismissive ("that won't
   work").

There is no network in this sandbox. Do not attempt to run the billing
service, query any real database, or contact Slack or S3; the review is
performed purely from the design document. Note this explicitly if your
review depends on information the document does not contain.

## Output Specification

After you finish, the repository should contain:

- `design-review.md` — the completed senior design review following the
  System Design Review framework
- the unchanged `docs/design-audit-export.md` committed on `main`
