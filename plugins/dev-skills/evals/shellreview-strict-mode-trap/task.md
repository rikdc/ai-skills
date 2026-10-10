# Review the Nightly Snapshot Script

## Background

The `sysops` repository carries a small wrapper script, `snapshot.sh`,
which the daily cron already runs: it writes a tarball snapshot of
`/data/live` into `/srv/snapshots` and prunes snapshots older than the
retention window. When the script was written the author bolted on the
"unofficial Bash strict mode" header (`set -euo pipefail` plus a narrowed
`IFS`), which the team's checklist reviewer at the time treated as the
whole error-handling story. After a cron run last week silently logged
"snapshot complete" while `/data/live` had been missing for several hours,
the team suspects the strict header is hiding failures rather than catching
them — and that some of the promised error handling never existed.

You are asked to review the script. The team reviews with the
`shell-script-reviewer` flow: a named dialect, static analysis first, then
manual review across Security, Correctness, Portability, and Style, with
every finding grounded in a named authority (the Google Shell Style Guide,
a specific ShellCheck code, BashPitfalls, or the security checklist).

## Setup

Run the setup script in the scenario working directory:

```bash
bash inputs/setup.sh
```

This writes `snapshot.sh` (the script under review), commits it as a
baseline, cleans any previous `review-report.md`, and installs a ShellCheck
shim at `bin/shellcheck` that replays the genuine findings a real ShellCheck
run produced for this file.

No network actions are needed; any network calls are skipped. The sandbox
has no real `shellcheck` or `shfmt` binaries — the shim only speaks the
`--format=json1` wrapper protocol for `snapshot.sh`, and
`scripts/run_shfmt_check.sh` will report shfmt as unavailable, which counts
as *reduced coverage* you should note rather than a blocker.

## Your Task

1. Put the ShellCheck shim first on your PATH:
   `export PATH="$PWD/bin:$PATH"`.
2. Follow the static-analysis-first step: run the skill's wrappers
   (resolve them relative to the skill's own directory), e.g.
   `scripts/run_shellcheck.sh snapshot.sh bash` and
   `scripts/run_shfmt_check.sh snapshot.sh`. Treat the ShellCheck codes it
   reports as ground truth — explain and prioritise them; do not re-derive
   rules ShellCheck already flagged.
3. Identify the dialect and apply the manual review categories in the
   documented priority order — Security first, then Correctness, then
   Portability, then Style. Bear in mind the skill's documented
   error-handling stance (in `references/bash-pitfalls.md`): `-e`/`-u`
   presence is a finding to flag, not a safety blanket to rely on or a
   gap to fill — `pipefail` and the `IFS` narrowing are the parts worth
   keeping — and an unchecked critical command is a Correctness finding
   regardless of `-e`, so error handling is scored via explicit
   per-command checks, not via the strict-mode header.
4. Produce the findings as a written review report. Use the skill's output
   skeleton: findings grouped by severity with security/critical issues
   first, each finding carrying Location (`file:line`), Severity
   (Critical/High/Medium/Low), Category (Security/Correctness/Portability/
   Style), a one-sentence risk specific to this script, and a concrete
   Current/Fix code snippet; end with a summary count table by severity and
   by category.
5. Review only — do not apply any fixes or modify `snapshot.sh`. Always
   show the proposed diff for approval; the report is the deliverable.

## Output Specification

After you finish, the scenario working directory should contain:

- `review-report.md` — the full review, laid out per the output skeleton
- `snapshot.sh` — unchanged from the baseline commit; proposed fixes live
  inside the report as snippets, not on disk
