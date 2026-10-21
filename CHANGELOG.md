# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

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
