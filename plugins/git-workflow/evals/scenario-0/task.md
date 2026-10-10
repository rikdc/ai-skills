# Bug Fix Commit: Calculator Division Guard

## Problem/Feature Description

A colleague on the backend team has patched a long-standing bug in the shared `calculator.py` utility: the `divide` function would raise an unhandled `ZeroDivisionError` when the denominator was zero. The fix is to return `None` instead. The patch has been applied to the file on disk, but has not yet been committed.

The project lives in your current working directory and already has a `Makefile` — run `bash setup.sh` first to initialize the git repository and leave the fix in a ready-to-commit state. After that, your job is to commit the change by following the team's standard commit workflow.

To help the team trace what was done, write a `workflow.md` file in the working directory documenting the steps you took: any pre-commit checks you ran (including their output), the commit command used, and the verification step output.

## Output Specification

After you finish, the working directory should contain:

- A committed git history that includes the bug fix for `calculator.py`
- `workflow.md` — a plain-text log of the steps you followed: pre-commit check results, the commit message you used, and the output of any post-commit verification you ran
