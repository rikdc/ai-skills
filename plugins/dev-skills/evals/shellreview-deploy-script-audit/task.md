# Review the Release Deploy Script

## Background

You have joined the platform team that owns `svc-ops`, a small tooling
repository for the checkout service. The script `deploy.sh` drives staging
deploys: it walks a directory of pending release bundles, copies artifacts
into per-release directories under the web root, then pings the deploy
webhook. Over the last two weeks an on-call rotation has logged three
incidents — partially emptied release directories and one staging
environment that never received the webhook ping — and the team has asked a
reviewer to audit the script before anyone runs it in production.

The team reviews shell scripts with the `shell-script-reviewer` flow: a
named dialect, static analysis first, then manual review across Security,
Correctness, Portability, and Style, with every finding grounded in a named
authority (the Google Shell Style Guide, a specific ShellCheck code,
BashPitfalls, or the security checklist) rather than taste.

## Setup

Run the setup script in the scenario working directory:

```bash
bash inputs/setup.sh
```

This writes `deploy.sh` (the script under review), commits it as a baseline,
cleans any previous `review-report.md`, and installs a ShellCheck shim at
`bin/shellcheck` that replays the genuine findings a real ShellCheck run
produced for this file.

No network actions are needed; any network calls are skipped. The sandbox
has no real `shellcheck` or `shfmt` binaries — the shim only speaks the
`--format=json1` wrapper protocol for `deploy.sh`, and
`scripts/run_shfmt_check.sh` will report shfmt as unavailable, which counts
as *reduced coverage* you should note rather than a blocker.

## Your Task

1. Put the ShellCheck shim first on your PATH:
   `export PATH="$PWD/bin:$PATH"`.
2. Follow the static-analysis-first step: run the skill's wrappers
   (resolve them relative to the skill's own directory), e.g.
   `scripts/run_shellcheck.sh deploy.sh bash` and
   `scripts/run_shfmt_check.sh deploy.sh`. Treat the ShellCheck codes it
   reports as ground truth — explain and prioritise them; do not re-derive
   rules ShellCheck already flagged.
3. Identify the dialect and apply the manual review categories in the
   documented priority order — Security first, then Correctness, then
   Portability, then Style — reading the applicable reference files
   (`references/security-checklist.md` unconditionally, plus the Bash
   references for this dialect).
4. Produce the findings as a written review report. Use the skill's output
   skeleton: findings grouped by severity with security/critical issues
   first, each finding carrying Location (`file:line`), Severity
   (Critical/High/Medium/Low), Category (Security/Correctness/Portability/
   Style), a one-sentence risk specific to this script, and a concrete
   Current/Fix code snippet; end with a summary count table by severity and
   by category.
5. Review only — do not apply any fixes or modify `deploy.sh`. Always show
   the proposed diff for approval; the report is the deliverable. If a
   construct's safety depends on a value ShellCheck cannot see, mark it
   "needs runtime verification" per the escalation rule instead of
   claiming certainty.

## Output Specification

After you finish, the scenario working directory should contain:

- `review-report.md` — the full review, laid out per the output skeleton
- `deploy.sh` — unchanged from the baseline commit; proposed fixes live
  inside the report as snippets, not on disk
