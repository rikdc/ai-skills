# Rewrite the Ingest Guide for Humans

## Background

You own documentation quality on the `ingest` service. The current
`docs/ingest-api.md` was written a long time ago and reviewers keep
bouncing code PRs that touch ingest because nobody can read the doc: it is
wall of unexplained jargon, it never mentions what happens when things go
wrong, and it links to nothing. There is also `docs/legacy-ingest.md`,
which still describes the nightly batch pipeline that was removed months
ago — two new hires followed it last sprint and lost an afternoon.

The team's documentation standard is the shared technical-documentation
principles: write for the audience, keep language clear (explain any
jargon at first use), be complete (document error cases and include
troubleshooting), keep docs close to the code they describe, and mark
obsolete docs clearly rather than letting them linger.

Run the setup script to prepare the repository in your working directory:

```bash
bash setup.sh
```

It initialises a repository with the ingest source (including its failure
modes), the jargon-heavy existing guide and the stale legacy doc.

The intended audience for the rewritten guide is new and existing
developers working with the ingest module — the same audience the original
targeted, so the format (a developer guide) stays; the *writing* is what
must change. No network access is needed and nothing outside this
repository may be modified.

## Your Task

1. Run `bash setup.sh` and read `src/ingest.py`, `docs/ingest-api.md`,
   `docs/legacy-ingest.md` and `README.md`.
2. Write a rewritten developer guide at `docs/ingest-guide.md` that:
   - covers every public function in `src/ingest.py` with its parameters
     and purpose,
   - documents the error cases the code actually has (duplicate batch
     rejection, and what a partial purge failure leaves behind) instead of
     pretending nothing fails,
   - defines each domain term at first use (for example *idempotent*,
     *upsert*, *backpressure*, *WAL*) — a reader should never meet an
     unexplained acronym,
   - includes at least one concrete code example or runnable command,
   - has a troubleshooting section for the realistic failure modes,
   - links to related docs in the repository.
3. Mark `docs/legacy-ingest.md` clearly as obsolete without deleting it:
   add a notice at the top saying it is deprecated and pointing readers to
   `docs/ingest-guide.md`.
4. Do not introduce new machinery: no builds, no CI, no new tools. The
   deliverables are the two documentation files.

## Output Specification

After you finish, the repository should contain:

- `docs/ingest-guide.md` — the rewritten, jargon-free developer guide.
- `docs/legacy-ingest.md` — still present, with a deprecation notice at
  the top pointing to the new guide.
