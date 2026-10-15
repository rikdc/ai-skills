# Bug Fix Commit: Calculator Division Guard

## Problem/Feature Description

A colleague on the backend team has patched a long-standing bug in the shared `calculator.py` utility: the `divide` function would raise an unhandled `ZeroDivisionError` when the denominator was zero. The fix is to return `None` instead. The patch has been applied to the file on disk, but has not yet been committed.

The project lives in your current working directory and already has a `Makefile` — run the fixture setup script (`bash inputs/setup.sh`, or `bash setup.sh` if it sits at the working-directory root) first to initialize the git repository and leave the fix in a ready-to-commit state. After that, your job is to commit the change by following the team's standard commit workflow.

To help the team trace what was done, write a `workflow.md` file in the working directory documenting the steps you took: any pre-commit checks you ran (including their output) and the commit command used.

## Output Specification

After you finish, the working directory should contain:

- A committed git history that includes the bug fix for `calculator.py`
- `workflow.md` — a plain-text log of the steps you followed: pre-commit check results and the commit message you used
