# Run the Protected-Branch Hook Against Edit/Write Tool Calls

## Background

`hooks/protect-main-branch.sh` is the plugin's `PreToolUse` hook for the
`Edit`, `Write`, `Bash` and `Task` tools. Documented behaviour:

- a `PreToolUse` hook always returns exit code `0`; a deny is expressed on
  **stdout** as
  `{"hookSpecificOutput": {"permissionDecision": "deny", "permissionDecisionReason": ...}}`
- an allow produces no decision output at all
- Edit/Write calls are judged by the git work tree containing
  `tool_input.file_path`
- protected branches: `main`, `master`, `production`, `release`
- the deny reason explains the PR-based workflow and points at
  `git checkout -b` as the remedy
- all denials are logged with `PROTECTED BRANCH VIOLATION` to
  `$CLAUDE_PROJECT_DIR/.claude/protect-main-branch.log`

Your sandbox has two checkouts of the same service repository:

- `repo/` — checked out on `main`
- `repo-feature/` — a linked worktree checked out on `feature/agent-work`

Two `Write` tool calls are prepared as payloads — they are identical apart
from which checkout they target:

- `payloads/write-main.json` — targets `repo/src/config.py`
- `payloads/write-feature.json` — targets `repo-feature/src/config.py`

## Your Task

Simulate each invocation exactly as Claude Code would run the hook
(`bash hooks/protect-main-branch.sh < <payload>`), and record the hook's
real behaviour. No network use. The payloads carry `"cwd"` set to the
correct checkout; leave their file contents alone.

1. Run the hook on `payloads/write-main.json`; record under `record/main/`:
   `exit-code`, `stdout.txt`, `stderr.txt`.
2. Run the hook on `payloads/write-feature.json`; record under
   `record/feature/` the same three artifacts.
3. Copy the hook's log to `record/protect-main-branch.log`.
4. Write `record/decision-main.txt` containing one word: what Claude Code
   should do with the first tool call (`run` or `deny`).
5. Write `record/remedy.txt` with the exact remedy command the hook
   recommends for getting this change onto a branch safely (the `git`
   command from the deny text).

## Output

- `record/main/{exit-code,stdout.txt,stderr.txt}`
- `record/feature/{exit-code,stdout.txt,stderr.txt}`
- `record/protect-main-branch.log`
- `record/decision-main.txt`, `record/remedy.txt`
- `hooks/`, `payloads/` unchanged; neither checkout's files modified
