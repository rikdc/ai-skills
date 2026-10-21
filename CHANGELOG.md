# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `evalctl suite` now announces the shape of a run before it starts: one
  `suite plan` record names the selected scenarios, the agent and judge
  invocations this pass will spend, the jobs, and the per-run timeout, and a
  single job over several scenarios warns that they run one after another.
  `run started`/`run finished` and `scenario started`/`scenario finished` also
  carry a `progress="i/N"` position counted over the work the pass actually
  executes, so a resumed suite does not overstate its denominator
- `evalctl` records whether the with arm actually used the skill
  (`skill_use` in `result.json`, detected from the agent log) and the report
  marks scenarios whose skill was never invoked as `skill-not-exercised`,
  excluding them from the headline delta.
- `evalctl report` now states caveats up front: a Warnings block for unused
  skills, failed or retried attempts (count, kind and first error), a judge
  that is the same model as the agent, and n below 3. `evalctl suite` and
  `grade` print the same-model warning on stderr.
- Added the Go `evalctl` harness as the default `make eval` backend for
  Milestone A (command agent and command judge). `make evalctl-build`,
  `evalctl-test`, `evalctl-check`, `evalctl-selftest`, and `evalctl-parity`
  provide deterministic build, Go test, vet/gofmt, selftest, and shell-parity
  gates with no model or network; `make test` now runs the Go checks too
- Added `tests/test-evalctl-parity.sh`: compares shell and Go scenario
  selection, plan counts, the authoring gate, and stub-agent comparison and
  functional campaigns (20 checks)
- Added a `evalctl` GitHub workflow: Go test/vet/race/selftest/parity on Linux
  and Go test/process-lifecycle coverage on macOS
- Added `scripts/eval-usage.sh`, a read-only usage parser/hook that normalizes
  opencode JSONL and Claude result-object tokens/cost, supports a generic
  `--cmd` hook, and reports unknown usage as unknown rather than zero
- Added agent and judge cost/token capture throughout the eval harness:
  per-run `usage.json`/`judge-usage.json`, suite sums and coverage, resume
  backfill, report CLI/Markdown/JSON/HTML cost tables, and an efficiency note
  only when full cost and score deltas support it
- `make eval` now supports `USAGE_CMD` and `JUDGE_USAGE_CMD`, warns when a
  recognized CLI lacks structured output, and documents cost capture
- Added `scripts/eval-report.sh`: a read-only report renderer over a suite
  directory with a fixed 100-point denominator, pending/not-comparable states,
  per-criterion pairing, n-run mean/min/max, scenario and overall aggregates,
  regression detection, conservative verdicts, and CLI/JSON/Markdown/HTML
  output (self-contained, escaped, colour only on a TTY)
- Reports now split tokens into input, output, reasoning, cache-read and total,
  show agent/judge/wall timing with timed-out-run counts, and accept
  `--baseline DIR` to compare score, per-run cost and token deltas with an
  earlier suite; `eval-grade.sh` records `judge-result.json` timing
- Added `make eval` with `PLUGIN`, `SKILL`, `UNIT`, `RUNS`, `JOBS`, `AGENT`,
  `JUDGE`, `DRY` and `REPORT`; `DRY=1` prints the plan and report destination,
  and a real run captures its new suite directory and prints the CLI report
  plus the HTML path
- Every eval scenario now declares the `unit`/`units` it measures; the
  deterministic gate rejects missing or unknown units
- `eval-run.sh` and `eval-suite.sh` accept repeatable `--unit`, install only
  the selected skill/command in the with-arm, select scenarios by declared
  unit, and include units in the manifest, resume equality and `--plan`
- Added an eval suite harness (`scripts/eval-suite.sh`): discovers every
  `plugins/*/evals/*` scenario, plans agent invocation counts, runs in bounded
  parallel jobs, shards across machines, resumes unchanged scenarios, and
  writes per-plugin and per-scenario with/without reports
- Added an eval judge adapter (`scripts/eval-grade.sh`): grades the criteria a
  scenario's checks cannot score, validates the judgement against the rubric,
  and folds accepted judgements into the suite totals
- Added deterministic harness tests to `make test` (no model, no network),
  including report/verdict/HTML and `make eval` dry-run coverage
- Added forgejo-review plugin with the forgejo-advisory-review workflow skill (advisory COMMENT review on a Forgejo PR: clone at head, diff, review, one Reviews-API post with inline comments, 422 recovery, verify)
- Added changelog command with security controls
- Added code quality check command with parallel sub-task strategy
- Added clean command for removing redundant comments
- Added GitHub Super-Linter workflow
- Added MCP Security Scanner for preventing sensitive data leaks

### Changed

- `make eval` renders the report even when the suite fails: it captures the
  suite's exit code, runs the report step anyway, prints the HTML path, and
  exits with the suite's own code, so a campaign with one timed-out run no
  longer finishes without a report. A suite that dies before writing any
  artifact skips the report and keeps its own error
- `make eval` accepts `OUT=<dir>` to reuse a suite directory (the report goes
  to `<dir>/report`), `RESUME=1` to pass `--resume`, and `RESUME_POLICY` to
  choose the policy (default `retry-failed`). `RESUME=1` requires `OUT` and
  names the suite it is resuming before it starts, so a partial suite is
  resumed instead of re-run
- `summary.md` now separates the two reasons a criterion stays pending: a run
  whose attempt was never accepted gets a copy-pasteable
  `suite --resume --resume-policy retry-failed` command built from the
  recorded manifest, and only an accepted but unjudged run keeps the `grade`
  hint. The suite-level runner-failure error names the same resume flags
- A resumed suite may change its operational caps: `JOBS` and the per-run
  timeouts (`TIMEOUT`/`--timeout`/`--judge-timeout`) are no longer treated as
  configuration changes, so a run that keeps hitting its cap can be retried
  with a longer one instead of starting the suite over. The suite reports the
  difference (`resume timeouts differ from the recorded suite`) rather than
  refusing it; everything that defines the experiment (agent and judge command
  and identity, arm, `RUNS`, units, credential mode) is still compared exactly
- `nix-expert` secrets guidance now rejects an untracked plaintext env file,
  requires one declaration per secret, and lists the migration and rotation
  steps
- `make eval` now rejects an `AGENT`/`JUDGE` containing an empty quoted
  argument — the single-dollar `$(cat)` trap — naming the cause and the
  `$$(cat)` fix, and warns when an ambient `RUNS` is discarded instead of
  silently ignoring it; a real run also echoes the resolved runs per arm, jobs
  and per-run timeout
- `make eval` accepts `TIMEOUT` (a duration such as `15m`, or seconds) beside
  `RUNS`/`JOBS` and forwards it as `--timeout`. The legacy shell harness only
  takes whole seconds, so `EVALCTL_LEGACY=1` forwards an integer and warns that
  it is ignoring a duration instead of failing
- `evalctl suite` warns while it runs, adds a `Skill not exercised` section to
  `summary.md`, and fails under `--strict` when a with-arm run never invoked
  the scenario's skill
- `make eval` now routes to `evalctl` and builds `bin/evalctl` on demand; set
  `EVALCTL_LEGACY=1` to run the legacy shell harness. The shell scripts and
  jq programs remain until the documented deletion criteria are met, and the
  `evalctl-a-parity` tag is the rollback checkpoint
- Every `plugins/*/evals/*/scenario.json` now maps its scenario to the
  skill/command/hook unit it measures; `generate-evals` requires the field
  for new scenarios
- `eval-run.sh` now installs either the selected units or (without `--unit`)
  the plugin's whole skill/command set in the with-arm
- `eval-run.sh` now installs a plugin's namespaced commands in the with-arm
  and always writes the evidence bundle; `--keep` only controls whether the
  sandbox directory survives
- Eval sandboxes copy the agent CLI's login state in for the run (and remove
  it afterwards), so sandboxed `HOME` no longer breaks authenticated agents;
  `--auth-path` adds any other CLI's credential file, and `--no-auth` keeps a
  run fully hermetic
- Migrated the `git-workflow` plugin from commands to the skill format
  (`skills/<name>/SKILL.md`)
- `evalctl suite` and `evalctl run` report progress on stderr while they work:
  one Info line per `scenario started`, `run started`, `run finished`,
  `judge started`, `judge finished`/`judge failed`, and `scenario finished`
  with status and duration. Progress never touches stdout, `-q` silences it,
  and `-v`/`--debug` add skipped-run events
- Tightened `git-workflow` commit and PR message rules: no emoji, shorter bodies
- Improved project linting and code quality standards
- Enhanced security scanning capabilities

### Fixed

- `evalctl` now exits 2 for every positional-argument error (for example
  `evalctl report a b`), not only the ones whose cobra message happened to
  start with "unknown command", and no longer matches error text to choose an
  exit code.
- `evalctl` no longer fails every command when the working directory is gone;
  the missing directory surfaces as a missing repository root only for
  commands that need one, so `evalctl version` still works.
- An atomic write whose directory fsync fails after the rename now returns an
  error wrapping `atomicfile.ErrNotDurable`, so callers can tell "content
  replaced but not yet durable" from "not written".
- A usage record's `known` flag is now always derived from its `state` when
  serialised or updated, so the two can no longer disagree.
- `evalctl grade` now refreshes `summary.md`, `suite.json` counts,
  `results.jsonl` and the grading index after grading, so a suite graded after
  the run no longer reports 0% and every run pending.
- Revised the `nix-expert` eval rubrics to reward the skill's own rules (the
  five-rung testing ladder, the remote `test` step, the generated update
  output in lock commits, agenix/sops path consumption) instead of general
  good practice.
- Fixed eval tooling edge cases: a report now treats a declared run with no
  directory as missing, the suite keeps the fixed 100-point denominator for a
  partially scored rubric, fractional judgements keep their exact score and
  rows, mechanical coverage uses the checks' own maxima, multiple `--unit`
  selections reach the runner as
  separate arguments, uncreatable suite/report outputs fail before or during
  run, a narrowed `--resume` rewrites the manifest, and an interrupted run
  removes copied credentials and its sandbox
- Hardened the eval harness against the shell review findings: an interrupted
  suite now stops in-flight agent jobs and their descendants, an uncopyable
  credential source fails the run instead of silently running unauthenticated,
  an unresolvable `--changed` ref is reported by name, the usage hook's stderr
  reaches the caller's log, an evidence `tar` failure fails the run, the
  no-`timeout` watchdog reaps its sleeper, and the largest report/suite jq
  programs live in `scripts/jq/`
- Fixed protect-main-branch denying writes in linked worktrees: the branch is
  now read where the write lands (Edit/Write target path, Bash `cd` and
  `git -C`), not in the hook's own working directory
- Fixed markdown linting issues (line length violations)
- Fixed general linter issues across project files
- Removed superfluous comments from codebase
- `--pass-env` now overrides the credential deny list: an explicitly requested
  variable such as `OPENCODE_API_KEY` reaches setup, checks, the agent, and the
  judge instead of being silently dropped. Credential-shaped names that were
  not requested are still stripped, and the duplicated deny list in the command
  adapter and the workspace is now one shared helper
- The command judge now receives the same CLI credentials and `--pass-env`
  values as the agent in both `suite` and `grade`. It previously ran in its own
  empty home, so with `--auth-cli` the agent was authenticated but every judge
  attempt failed and every criterion stayed `pending` with no verdict possible;
  `grade` also gained `--auth-cli`, `--keychain`, `--auth-path`, and
  `--pass-env`, and the pending-criteria hint now names `--auth-cli` when the
  suite ran with credentials
- `make eval` now forwards credentials to the Go backend: `AUTH_CLI=1` (the
  default, matching the shell harness) copies the agent CLI login state,
  `AUTH_CLI=0` disables copying, and `AUTH_PATH`/`KEYCHAIN` map to
  `--auth-path`/`--keychain`. `bin/evalctl` keeps its credential-off default,
  so a real run without credentials no longer fails as an unexplained agent
  error
- Failed runs now explain themselves: the command adapter appends the agent's
  last output line to the error, `summary.md` gains a `Failures` section, and
  the suite-level error names the first failing run instead of only counting
  runner failures

### Security

- Implemented MCP Security Scanner to prevent sensitive data from being sent to external services
