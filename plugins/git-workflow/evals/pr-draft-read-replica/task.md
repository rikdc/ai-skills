# Open a Draft PR for the Read Replica Branch

## Background

You are working on the metrics service repository. A branch named
`feature/replica-routing` has been pushed to the shared GitHub repository and
contains two commits ahead of `main`. The team reviews only draft PRs first,
and the team standard is a plain-text conventional-commit title with a short
body describing what the branch changes.

The repository lives in the working directory. Run the setup script to prepare
it:

```bash
bash setup.sh
```

This initialises the repository with `main` and the two branch commits, and
installs a `gh` shim at `~/bin/gh` (put `~/bin` first on your PATH so the shim
is used instead of any real `gh`). The shim records arguments to
`pr-record.json` in the repository root with the fields `title` and `body`, so
you can verify the PR was created exactly as intended.

Your job: create a draft pull request for `feature/replica-routing` against
`main`, following the team's PR conventions.

## Output Specification

After you finish, the repository should contain:

- `pr-record.json` — the recorded title and body the PR was created with
- a draft PR creation command run through the `gh` shim
