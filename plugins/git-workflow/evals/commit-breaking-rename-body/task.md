# Commit a Breaking API Rename

## Problem/Feature Description

The data ingestion team has been maintaining a Python library used by several
downstream services. The library's core record-reading module originally
depended on `legacy-parser`, an unmaintained third-party package that has now
reached end-of-life and is being dropped from the codebase. As part of this
migration the public function `parse_record()` has been renamed to
`read_record()` and the underlying implementation swapped to the new `csv_io`
library. The rename is intentional and permanent - callers outside this
repository will need to update their code.

A developer has already made the change and staged it for commit. The diff
shows the rename but gives no indication of *why* the function was renamed or
that this affects external callers. Your job is to commit it with an
appropriate message, following the team's standard commit workflow.

## Your Task

1. Run the setup script to initialise the repository and stage the change:

```
bash inputs/setup_repo.sh
```

This creates a `repo/` directory containing the project with one staged
change ready to commit.

2. Change into the `repo/` directory and commit the staged change. The
context for this commit is:

- The function `parse_record()` was renamed to `read_record()` because the
  upstream `legacy-parser` library is being dropped in favour of `csv_io`.
- This is a **breaking change** for any callers outside this repository.

3. After committing, write the exact commit message you used to
`commit_log.txt` in your working directory (i.e. the `repo/` directory). You
can produce this with:

```
git log -1 --format=%B > commit_log.txt
```

## Output

- `repo/commit_log.txt` - the full commit message (subject line and body if
  any) as written to the git log.
