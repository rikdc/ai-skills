# Invoke the MCP Security Scanner on Benign and Out-of-Scope Payloads

## Background

The same plugin hook again: `hooks/mcp-security-scanner.sh` reads a JSON
hook payload on stdin, exits `0` to allow and `2` to block, prints a
`{"decision": "block", ...}` object on stderr only when it blocks, and
appends a line describing every scan to `security-scan.log` in its own
directory. Two scope rules are documented:

- only `PreToolUse` events for tools whose name starts with `mcp__` are
  scanned — a built-in tool such as `Bash` is never touched
- a payload with no credentials produces no violation and no block decision

Two payloads are prepared:

- `payloads/release-notes.json` — `PreToolUse`, tool
  `mcp__notion__create-page`, benign release-notes content with no
  credential shapes
- `payloads/non-mcp.json` — `PreToolUse` for the built-in `Bash` tool,
  which is outside the hook's scope and must be passed through without any
  scan

## Your Task

Simulate both invocations the way Claude Code would deliver them
(`bash hooks/mcp-security-scanner.sh < <payload>`), and record what the hook
does. No network use; external scanners are absent so the documented
pattern-fallback path runs.

1. Run the hook on `payloads/release-notes.json`; record artifacts under
   `record/allow/`: `exit-code` (the integer exit status), `stderr.txt`
   (stderr, may be empty), `stdout.txt` (stdout, may be empty).
2. Run the hook on `payloads/non-mcp.json`; record under `record/non-mcp/`
   the same three artifacts.
3. Copy the hook's log after both runs to `record/security-scan.log`.
4. Write `record/verdict.txt`: a single line stating whether the
   release-notes request may proceed (`proceed` or `blocked`) and whether
   the Bash payload was scanned (`scanned` or `skipped`), e.g.
   `proceed; skipped`.

## Output

- `record/allow/{exit-code,stderr.txt,stdout.txt}`
- `record/non-mcp/{exit-code,stderr.txt,stdout.txt}`
- `record/security-scan.log`
- `record/verdict.txt`
- `hooks/`, `payloads/`, `security-patterns.conf` must remain unchanged
