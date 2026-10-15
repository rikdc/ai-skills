# Open a Draft PR for the Read Replica Branch

## Background

You are working on the metrics service repository. A branch named
`feature/replica-routing` has been pushed to the shared GitHub repository and
contains two commits ahead of `main`. The team reviews only draft PRs first,
and the team standard is a plain-text conventional-commit title with a short
body describing what the branch changes.

The repository lives in the working directory. Run the fixture setup script to
prepare it:

```bash
bash inputs/setup.sh  # or `bash setup.sh` if the fixture sits at the working-directory root
```

This initialises the repository with `main` and the two branch commits, and
installs a `gh` shim at `./bin/gh` (prepend `./bin` to your PATH so the shim is
used instead of any real `gh`). The shim records arguments to
`pr-record.json` in the repository root with the fields `title`, `body`,
`draft`, `base` and `head` - `head` is taken from `--head`, or, exactly as
real `gh` does, from the branch you are on when you run it.

Your job: create a draft pull request for `feature/replica-routing` against
`main`, following the team's PR conventions.

## Output Specification

After you finish, the repository should contain:

- `pr-record.json` — the recorded invocation (`title`, `body`, `draft`, `base`, `head`)
- a draft PR creation command run through the `gh` shim
