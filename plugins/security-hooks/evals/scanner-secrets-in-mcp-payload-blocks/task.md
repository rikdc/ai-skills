# Invoke the MCP Security Scanner on a Payload Containing Secrets

## Background

You are security-reviewing the payments-service repository ahead of a demo.
The security-hooks plugin ships a Claude Code `PreToolUse` hook,
`hooks/mcp-security-scanner.sh`, which scans every MCP tool request for
sensitive data before it is sent to an external service. Its documented
behaviour:

- runs as `hooks/mcp-security-scanner.sh`, reading one JSON hook payload from
  stdin
- exit code `0` = allow, exit code `2` = block the MCP request
- when it blocks, it prints a `{"decision": "block", "reason": ...}` JSON
  object on **stderr**
- when it allows, it produces no decision output
- every scan is logged to `security-scan.log` in the directory the pattern
  file lives in; violations are logged as lines containing
  `SECURITY VIOLATION` plus the matched pattern names

A colleague reviewed a bug report and is about to paste its contents into an
MCP tool call to the GitHub integration. The exact hook payload they would
send is already prepared at `payloads/bug-report.json` — it embeds an AWS
access key ID and a hard-coded API key in the comment body.

## Your Task

Simulate the Claude Code hook invocation yourself, then record the
hook's behaviour. No network use is required or allowed; the fixtures are
deterministic; the sandbox has no external secret scanners installed, so the
documented pattern-fallback path is what runs.

1. Run the hook the way Claude Code would:
   `bash hooks/mcp-security-scanner.sh < payloads/bug-report.json`.
2. Record the run into `record/` (create the directory as needed):
   - `record/exit-code` — the hook's exit code (one integer)
   - `record/stderr.json` — the hook's stderr, exactly (may be empty)
   - `record/stdout.txt` — the hook's stdout, exactly (may be empty)
3. Copy the hook's log for this run to `record/security-scan.log`.
4. Write `record/decision.txt` containing exactly one word: the own-word
   summary of what the hook did — `blocked` or `allowed`.
5. In `record/matched-patterns.txt` list the pattern names from the config
   that the log says matched (one per line).
6. State in one file, `record/handle-next.txt`, what the agent should do
   with this request: either `send` or `redact-and-resubmit`.

## Output

- `record/exit-code`, `record/stderr.json`, `record/stdout.txt`,
  `record/decision.txt`, `record/matched-patterns.txt`,
  `record/handle-next.txt`, `record/security-scan.log`
- `hooks/`, `payloads/`, `security-patterns.conf` must remain unchanged
