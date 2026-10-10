# Task Tool on Protected Branches, Master Denial, and the Emergency Bypass

## Background

Final contracts of `hooks/protect-main-branch.sh`:

- the `Task` tool has no target file; it is judged by the session directory
  in `.cwd` — a Task on a protected branch is denied
- protected branches are exactly `main`, `master`, `production`,
  `release`; a change in a repo checked out on any *other* branch name is
  allowed silently
- one documented escape hatch: when the variable
  `ALLOW_PROTECTED_BRANCH_EDIT` is set in the environment the hook itself
  runs in, it logs a bypass warning and allows the protected-tool call
  without a deny decision
- stdout deny JSON with exit code 0; allows are silent

The sandbox has two repos sharing the plugin hook:

- `repo-release/` — checked out on `main`
- `repo-hotfix/` — checked out on `master`

Three payloads are prepared:

- `payloads/task-main.json` — `Task` tool, cwd `repo-release`
- `payloads/write-master.json` — `Write` tool targeting a file in
  `repo-hotfix`
- `payloads/task-bypass.json` — identical `Task` payload as the first

## Your Task

Simulate the invocations and record behaviour. **Note the bypass
deliberately:** the hook reads its own environment, so the variable must be
on the same command line as the hook invocation
(`ALLOW_PROTECTED_BRANCH_EDIT=1 bash hooks/protect-main-branch.sh < ...`),
not an inline prefix on the payload's command.

1. Run the hook on `payloads/task-main.json`; record under
   `record/task-main/`: `exit-code`, `stdout.txt`, `stderr.txt`.
2. Run the hook on `payloads/write-master.json`; record under
   `record/write-master/` the same three artifacts.
3. Run the hook on `payloads/task-bypass.json` with
   `ALLOW_PROTECTED_BRANCH_EDIT=1` set for the hook process; record under
   `record/task-bypass/` the same three artifacts.
4. Copy the hook's log to `record/protect-main-branch.log`.
5. Write `record/decisions.txt`: one word per line — the verdict for
   `task-main`, the verdict for `write-master` (`deny`/`allow`), and the
   verdict for `task-bypass`, then a line stating which variable the hook
   honours to bypass protection and why it must reach the hook process
   itself.

## Output

- `record/task-main/{exit-code,stdout.txt,stderr.txt}`
- `record/write-master/{exit-code,stdout.txt,stderr.txt}`
- `record/task-bypass/{exit-code,stdout.txt,stderr.txt}`
- `record/protect-main-branch.log`
- `record/decisions.txt`
- `hooks/`, `payloads/`, both repos unchanged
