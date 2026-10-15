# Commit Pending Changes in the Acme Pipeline Repo

## Background

You have been handed a small Python project — the Acme Data Pipeline — that
has been sitting with several local modifications for a few days. The developer
who made these changes left no staged commits and no notes on what was changed
or why. Your job is to tidy this up by committing everything that is pending.

The project lives in the current working directory. Before you start, run the
fixture setup script to initialise the repository with its pending changes:

```bash
bash inputs/setup.sh  # or `bash setup.sh` if the fixture sits at the working-directory root
```

This will create a git repository in the current directory with an initial
commit already in place and three files that have been modified or added since
that commit — but nothing has been staged yet.

## What to Do

Review the pending changes and commit them. You should:

1. Inspect what has changed (use `git status` and `git diff` to understand each change).
2. Decide on the right commit strategy — the changes may or may not belong in a
   single commit.
3. Commit the changes with well-formed commit messages.
4. Write a file called `commit-plan.md` in the working directory **before**
   you start committing. It should record your assessment of the pending changes
   and the commit strategy you chose (including your rationale for splitting or
   not splitting them).

## Output

When you are done, the repository should have all pending changes committed
and a `commit-plan.md` file present in the working directory.
