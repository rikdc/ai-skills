# Commit the flake.lock Update Properly

## Background

You maintain a small flake-based NixOS repository. It has two inputs:
`nixpkgs` (tracking `nixos-25.05`) and `home-manager` (pinned to a specific
commit of the `release-25.05` branch, because an upstream regression in
release-unstable broke your waybar config). Yesterday someone ran the update
and the change to `flake.lock` is staged but **not committed**, and the
committing session got interrupted, so nobody wrote a message.

Because your sandbox has no network and no Nix, the dry-run equivalent you
need has already been captured: the output of the update command is in
`notes/update-summary.txt`. Treat that as the exact text the lockfile tooling
would generate for this update.

Run the setup script to prepare the repository:

```bash
bash setup.sh
```

## Your Task

1. Review the staged `flake.lock` change (`git diff --cached --stat`,
   `git diff --cached flake.lock`) and the update summary in
   `notes/update-summary.txt` so your message names exactly which inputs
   moved.
2. Write the commit message to `commit-message.txt`.
3. Commit the staged change with your message (`git commit -F
   commit-message.txt`). Do not amend the staged content, do not commit
   anything else.
4. The team's rule: a `flake.lock` diff is bare hashes and epochs, so the commit
   message body must carry the human-readable record of which inputs moved and
   where; when an input is deliberately pinned or overridden, the *why* goes
   above the summary. Follow that rule exactly. Do not run any network
   command.

## Output

The repository must contain:

- `commit-message.txt` — the commit message you wrote
- a single commit on top of `main` containing only the staged `flake.lock`
  change, committed with `-F commit-message.txt`
