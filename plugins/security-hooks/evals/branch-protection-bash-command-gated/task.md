# Gate Bash Tool Calls on a Protected Branch

## Background

Same hook — `hooks/protect-main-branch.sh` — but now the `Bash` tool. Its
documented Bash policy:

- the hook judges **what the command actually does**, parsed into pipeline
  and list segments, in the directory the command runs in
- `git` subcommands that write to the branch or remote (`push`, `pull`,
  `fetch`, `merge`, `rebase` and friends) are denied on a protected branch;
  read-only git (`status`, `log`, `diff`, `show`) passes unaffected
- `git reset` that moves the branch pointer or destroys the working tree is
  denied; `git branch` deletion/force-rename is denied; `checkout`/`switch`
  that move HEAD off the protected branch are denied — **except**
  `git checkout -b` / `git switch -c`, which are deliberately allowed
  because they are the remedy the deny message itself recommends
- writes that bypass git by touching `.git/` are denied on a protected
  branch
- any other command (`curl`, `make`, `bash -c '...'` etc.) is not parsed for
  file writes: it passes, per the documented fail-open design
- deny still comes through stdout `hookSpecificOutput.permissionDecision`
  JSON with **exit code 0**; allows are silent; denials are logged with
  `PROTECTED BRANCH VIOLATION`

The sandbox repo `repo/` is checked out on `main`. Six `Bash` tool payloads
are prepared in `payloads/`.

## Your Task

Simulate each invocation (`bash hooks/protect-main-branch.sh < payloads/<id>.json`)
and record the hook's behaviour per case:

1. For each of `status`, `push`, `branch-delete`, `gitdir-tamper`,
   `create-branch`, `foreign-write` — run the hook and record under
   `record/cases/<id>/`: `exit-code`, `stdout.txt`, `stderr.txt`, plus one
   word `verdict` equal to what the tool loader sees (`block` when stdout
   carries a deny decision, `allow` otherwise).
2. Copy the hook's log to `record/protect-main-branch.log`.
3. Write `record/summary.txt`: one line per case id as
   `<id>:<block|allow>`, then a final line naming which two cases the hook
   deliberately allows despite running on `main`, and the documented reason
   (find it in the hook's own messages or comments).

## Output

- `record/cases/<id>/{exit-code,stdout.txt,stderr.txt,verdict}` for all six ids
- `record/protect-main-branch.log`
- `record/summary.txt`
- `hooks/`, `payloads/`, `repo/` unchanged (no payload was ever executed —
  the hook only decides)
