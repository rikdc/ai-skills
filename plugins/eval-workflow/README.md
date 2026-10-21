# eval-workflow

Eval tooling for this repository's plugins, in the tessl-compatible `evals/`
format.

**`generate-evals`** authors offline verifiable scenarios at the plugin root
`evals/` for a plugin's skills, commands, and hooks, then runs the local
verification ladder (`tessl plugin lint`, criteria sums, per-scenario smoke
tests, `make test`).

**`review-evals`** judges what a script cannot: whether a scenario
discriminates, whether a rubric is fair to the unit it tests, whether a
recording shim is faithful, whether a description will activate - and grades
the criteria an eval run left pending.

Worked example: `plugins/git-workflow/evals/`.

## Deterministic gate

`tests/test-evals.sh`, run by `make test` and in CI: scenario layout and
naming, unit metadata (every declared skill/command/hook must exist), rubric
arithmetic, fixture hygiene, plugin manifests, and skill frontmatter
conventions. `--changes <ref>` lists the skills and scenarios a ref touched.

## Running one eval

Running an eval needs a model, so it happens on a contributor's machine, never
in CI:

```bash
scripts/eval-run.sh --plugin git-workflow --scenario pr-draft-read-replica \
    --agent-cmd 'claude -p'
```

That copies the scenario into a temporary workspace with `HOME` redirected
into it, runs the agent once without the plugin's unit and once with it,
records what changed, scores the mechanical half of the rubric through the
scenario's `checks.sh`, and leaves a grading request for the criteria that
need judgement. The with-arm installs the plugin's skills and namespaced
commands; the evidence bundle (`snapshot.tar`, logs, before/after state) is
always written. Results land in the git-ignored `results/eval-run/`.

The workspace is a working directory, not a security boundary. The agent runs
as your user with your user's privileges: it can read and write absolute host
paths, and it can reach the network unless the command you pass prevents it.
Use a container or VM when a scenario must actually be isolated.

Every scenario declares the unit it measures in `scenario.json`:

```json
{"description": "...", "include": ["./inputs"], "unit": "skills/pr"}
```

Use `"units": ["commands/triage", "skills/triage-reviews"]` when a scenario
genuinely exercises several units. Pass `--unit` (repeatable) to install only
the selected units in the with-arm:

```bash
scripts/eval-run.sh --plugin git-workflow --scenario pr-draft-read-replica \
    --agent-cmd 'claude -p' --unit skills/pr
scripts/eval-run.sh --plugin code-quality --scenario review-empty-changeset \
    --agent-cmd 'claude -p' --unit commands/review
```

`skills/<name>` copies that skill directory, `commands/<name>` copies only
that command file into the namespaced commands directory, and `hooks/<name>`
stages the plugin's current `hooks/` directory and patterns file for the
fixture to copy; hook scenarios invoke the hook themselves. A scenario with
`"mode": "functional"` (all current hook scenarios) runs only the with arm and
reports an absolute score: it exercises the current plugin source directly and
does not claim a with/without contribution. A comparison-mode hook scenario
uses the current source in the with arm and a disabled no-op control in the
without arm. An unknown unit, or a unit the plugin does not ship, fails before
any run. Without `--unit`, the with-arm keeps installing every skill and
command in the plugin.

The workspace `HOME` starts empty, so tools that write to `$HOME` land in the
workspace rather than your home directory. That redirection is not filesystem
isolation: absolute paths still reach the host. Immediately before the agent
runs, login state for the common CLIs
(`~/.claude.json`, `~/.claude/.credentials.json`, `~/.codex/auth.json`,
`~/.config/gh/hosts.yml`, `~/.local/share/opencode/auth.json`, and their
settings) is copied in, and on macOS the login Keychains directory is linked
into the workspace because the CLI looks it up under `$HOME/Library`. That
symlink grants the agent full access to your keychains, so it is a deliberate
tradeoff for authenticated runs. `--no-auth` skips the credential copying and
the link; it does not isolate the process. The copied files and link are
deleted again as soon as the agent exits (or on interrupt), and `home/` is
excluded from the evidence bundle. Pass `--auth-path PATH` (repeatable) for
any other credential or config file under your home; a suite forwards every
path. Pass `--no-auth` when the agent authenticates from the environment (for
example an API key).

Any agent CLI works if it reads the task on **stdin**, does its work in the
**current directory**, and runs **non-interactively**. For a CLI that takes the
prompt as an argument, wrap it:

```bash
scripts/eval-suite.sh --agent-cmd 'opencode run --auto "$(cat)"'
```

The with-arm writes the plugin's skills to `--skill-dir` and its commands to
the sibling `commands/<plugin>/`. The default `.claude/skills` is also
discovered by opencode; use `--skill-dir .opencode/skills` for opencode's
native layout, which puts commands under `.opencode/commands/`.

Headless agents also need permission to use tools. The runner does not sandbox
the agent, so these flags reduce prompting, not risk: pass Claude Code's
`--dangerously-skip-permissions`, or `--permission-mode acceptEdits` when the
scenario only needs file edits; for opencode, `--auto`. Only use them in an
environment you are willing to let the agent modify.

## Running a suite

A suite is the same primitive across many scenarios. Nothing is hosted and no
eval credits are involved: the only cost is whatever `--agent-cmd` charges.

```bash
# What would run, and how many agent invocations it costs
scripts/eval-suite.sh --agent-cmd 'claude -p' --plan

# Everything, four scenarios at a time
scripts/eval-suite.sh --agent-cmd 'claude -p' --jobs 4

# Only the plugins and scenarios a ref touched (plus the working tree)
scripts/eval-suite.sh --agent-cmd 'claude -p' --changed origin/main

# One plugin, with a model grading every run it finishes
scripts/eval-suite.sh --agent-cmd 'claude -p' --plugin git-workflow \
    --judge-cmd 'claude -p --permission-mode acceptEdits'

# One unit, across every scenario that declares it
scripts/eval-suite.sh --agent-cmd 'claude -p' --unit skills/pr
scripts/eval-suite.sh --agent-cmd 'claude -p' --plugin code-quality \
    --unit commands/review

# A deterministic slice, for fanning out across machines
scripts/eval-suite.sh --agent-cmd 'claude -p' --shard 1/3
```

Selection is every `plugins/*/evals/*/` scenario, narrowed by `--plugin`
(repeatable), `--unit` (repeatable), `--scenario` (a regex over
`plugin/scenario`), `--changed REF`, and `--shard I/N`. `--unit` keeps only
scenarios whose declared `unit`/`units` include the request and forwards the
intersection to every `eval-run.sh`, so the with-arm installs exactly the unit
being measured. A scenario without unit metadata is rejected when `--unit` is
used. Execution is `--arm`, `--runs`, `--jobs`, and `--timeout`; staging and
scoring still happen in `eval-run.sh`.

`--jobs` parallelizes scenarios, not the arms or runs inside one scenario. A
three-scenario, three-run, both-arm suite is 18 agent invocations plus up to 18
judge invocations; with the default `--jobs 1` those model calls run one after
another, so an hour-long suite is expected rather than a hang. `--plan` prints
the invocation counts and suggests `--jobs N` when scenarios would run
sequentially. Each run is also capped by `--timeout` (agent) and
`--judge-timeout` (judge).

Each suite writes one directory under `results/eval-suite/`:

| Path | Contents |
| --- | --- |
| `manifest.json` | agent, arm, units, runs and a fingerprint per scenario |
| `results.jsonl` | one aggregate row per scenario, per-run detail included |
| `suite.json` | counts plus the scenario rows, for scripts |
| `summary.md` | per-plugin and per-scenario with/without table |
| `criteria/` | the rubric each scenario ran with, so reports stay readable |
| `grading/index.tsv` | every run whose criteria are still pending |
| `runs/`, `logs/` | the evidence `eval-run.sh` produced, per-run `usage.json`/`judge-usage.json`, and per-scenario logs |

`--resume --out DIR` skips a scenario only when its result files exist **and**
its fingerprint still matches. It also requires the same units, agent, arm,
runs, timeout, skill directory, auth mode and judge command. The fingerprint
covers the scenario and the plugin's skills, commands, hooks and manifest, so
editing a unit reruns that plugin's scenarios. A different judge is refused:
pass `--regrade` to replace the existing judgements with the new judge's.
Each accepted judgement records its judge command and grader version, so stale
grades cannot be attributed to the current judge.

A mechanical score is not a verdict. By default the suite exits non-zero only
when a scenario fails to run; `--strict` additionally fails when criteria are
still pending or a scenario scores no better with its unit than without it.

## evalctl (Milestone A)

`make eval` now runs `evalctl`, the Go harness under `cmd/evalctl` and
`internal/`. The shell harness is still installed and is the documented
fallback, so both paths read the same `plugins/*/evals/` scenarios and both
remain reportable. Nothing was deleted in the cutover.

Milestone A is the **command backend**: `--agent-cmd` and `--judge-cmd` are
the only real backends. It does not call a model API and does not execute
hooks directly. Milestone B (native OpenAI-compatible agent/judge) and
Milestone C (direct hook execution) are not available yet; flags such as
`--backend openai` and direct-hook scenarios return clear errors instead of
silently falling back.

### Build, test, and run

```bash
make evalctl-build       # bin/evalctl
make evalctl-test        # go test ./...
make evalctl-check       # gofmt check, go vet ./..., evalctl check
make evalctl-selftest    # deterministic end-to-end run + every fixture setup
make check               # evalctl-check + evalctl-selftest

make evalctl-parity      # shell-vs-Go selection, plan, gate, and stub campaigns
```

`evalctl selftest --quick` skips repository fixture coverage when you want a
fast local smoke test. `make evalctl-selftest SELFTEST_ARGS=--quick` does the
same through Make. None of these targets require a model, network, or
credentials.

```bash
bin/evalctl version
bin/evalctl list --plugin git-workflow
bin/evalctl plan --plugin git-workflow --runs 3
bin/evalctl check
bin/evalctl suite --agent-cmd 'claude -p' --plugin git-workflow --runs 3
bin/evalctl report results/eval-suite/<stamp>-<sha> --format all
```

The command adapter passes the prompt on stdin and runs in the temporary
workspace, exactly like `eval-run.sh`. `bin/evalctl suite` keeps credentials
off by default: `--auth-cli` copies the known CLI login state, `--keychain`
links the macOS Keychains directory, and `--auth-path` adds one path.

`make eval` opts in for you, because the shell harness it replaces copies CLI
login state by default (`scripts/eval-suite.sh` `AUTH=1`) and most agent CLIs
cannot run without it. Set `AUTH_CLI=0` to run without credentials,
`AUTH_CLI=1` (the default) to copy them, `AUTH_PATH=<relative path>` to add
files, and `KEYCHAIN=1` to link the macOS Keychains directory (`KEYCHAIN=1`
requires the evalctl backend). If a credential file cannot be read, the
suite-level error names it instead of only counting failed runs. The legacy
shell path keeps its own default (`--no-auth` disables it).

`suite` and `run` also report progress on stderr while they work — one line per
event (`scenario started`, `run started`, `run finished`, `judge started`,
`judge finished`/`judge failed`, `scenario finished`) with status and duration,
so a multi-minute suite is not a blank terminal. Progress never touches stdout,
`-q` silences it, and `-v`/`--debug` add skipped-run events.

### `make eval` cutover and `EVALCTL_LEGACY=1`

`make eval` keeps its existing variables and maps them to the Go suite and
report:

| Make variable | evalctl flag |
| --- | --- |
| `PLUGIN` | `--plugin` |
| `SKILL=<name>` / `UNIT=<unit>` | `--unit skills/<name>` / `--unit <unit>` |
| `RUNS` / `JOBS` | `--runs` / `--jobs` |
| `AGENT` / `JUDGE` | `--agent-cmd` / `--judge-cmd` |
| `USAGE_CMD` / `JUDGE_USAGE_CMD` | `--usage-cmd` / `--judge-usage-cmd` |
| `AUTH_CLI=1` (default) / `AUTH_CLI=0` | `--auth-cli` / no credential flag |
| `AUTH_PATH=<relative path>...` | `--auth-path` (repeatable) |
| `KEYCHAIN=1` | `--keychain` (evalctl backend only) |
| `DRY=1` | `suite --plan` (no agent, no writes) |
| `REPORT` | `report --format` |

The legacy shell path stays available at any time:

```bash
EVALCTL_LEGACY=1 make eval PLUGIN=git-workflow RUNS=1 DRY=1
EVALCTL_LEGACY=1 make eval PLUGIN=git-workflow SKILL=pr AGENT='claude -p'
```

A real run writes to `results/eval-suite/<timestamp>-<sha>-<pid>/` and prints
the report plus the HTML path, the same contract as before. Go suites use v3
attempt directories under `runs/<plugin>/<scenario>/<arm>/run-N/attempts/`;
`evalctl report` and `eval-report.sh` both read the accepted artifacts.

### Rollback checkpoint

Tag `evalctl-a-parity` is the pre-cutover checkpoint (create it on the commit
before the cutover). The documented rollback is:

```bash
EVALCTL_LEGACY=1 make eval ...     # keep working immediately
```

or revert the cutover commit, not the attempt/report work. There is no
destructive migration: old shell suite directories remain reportable, and a
v3 resume uses a new `--out` directory. Deletion criteria for the shell
orchestration are: all Milestone A gates pass for two consecutive parity
campaigns, Go tests cover the shell assertions, one release has shipped both
paths, and the rollback tag has been exercised once.

### Parity gate

`tests/test-evalctl-parity.sh` (run by `make evalctl-parity` and CI) compares
the shell and Go harnesses with no model or network:

- selection for every scenario and for `--plugin`, `--unit`, `--scenario`,
  and `--shard 1/4` / `3/4`;
- full and filtered plan output, including invocation counts;
- `tests/test-evals.sh` vs `evalctl check`, including `--changes HEAD`;
- a comparison scenario (`git-workflow/pr-draft-read-replica`) and a
  functional hook scenario (`security-hooks/scanner-clean-payload-allowed`)
  run with agent `true`, comparing status, arm/run results, mechanical
  scores, pending counts, suite counts, criteria, and the grading index.

Recorded result on the Milestone A cutover: **20 checks passed, 0 failed**
across the 65 repository scenarios. Evidence-path differences are excluded by
design: the shell writes a run directory and v3 writes an attempt evidence
path. Formatting and version fields are not compared.

### Evaluating the Go code

- `cmd/evalctl`: process entry, signal context, exit-code mapping.
- `internal/cli`: Cobra commands and flag wiring.
- `internal/scenario`, `internal/check`: discovery, selection, plan, and the
  deterministic authoring gate.
- `internal/runner`, `internal/runner/attempts`: workspace staging, attempts,
  `current.json`, projections, crash recovery.
- `internal/suite`, `internal/judge`, `internal/report`, `internal/reportio`:
  orchestration, command judging, report analysis and rendering.
- `internal/provider/command`, `internal/executor`, `internal/usage`,
  `internal/platform/execx`: the command backend, process lifecycle, and
  usage normalization.
- `internal/selftest`: the synthetic end-to-end fixture.

Run `go test ./...`, `go test -race ./...` (Linux), `go vet ./...`, and
`gofmt -l cmd internal` to check it. `internal/report/parity_test.go`,
`internal/report/v3_parity_test.go`, and `tests/test-evalctl-parity.sh` are
the shell-parity oracles.

## `make eval` and the report

`make eval` wraps the suite and the report renderer. It uses `evalctl` unless
`EVALCTL_LEGACY=1` is set. `SKILL=<name>` is shorthand for
`--unit skills/<name>`; `UNIT=` names any skill, command or hook. `PLUGIN`,
`RUNS`, `JOBS`, `AGENT`, `JUDGE`, `DRY` and `REPORT` map to the suite options
and the report format. `AGENT` is required for a real run and `JUDGE` defaults
to it.

```bash
# Plan only: no agent, no network
make eval PLUGIN=git-workflow RUNS=1 DRY=1

# A three-run per-unit experiment, judge defaulting to the agent
make eval PLUGIN=git-workflow SKILL=pr RUNS=3 JOBS=2 AGENT='claude -p'

# A command unit, with the agent command that needs stdin substitution
make eval PLUGIN=git-workflow UNIT=commands/review AGENT='opencode run --auto "$$(cat)"'
```

Make consumes `$(...)` itself. An agent command that needs the shell to expand
`$(cat)` must therefore use `"$$(cat)"` in the value passed to Make, as in the
last example. The target captures the suite directory it creates (it never
picks the newest directory in `results/`), renders the report, and prints the
CLI report plus the HTML path.

`scripts/eval-report.sh SUITE_DIR [--format cli|json|md|html|all] [--no-color]
[--out DIR] [--baseline DIR]` is read-only. It never reruns an eval or calls a
model. `--baseline` compares the suite with an earlier one: score, per-run
cost, and input/output token deltas for the whole suite and for each scenario
present in both. The comparison is in `report.json` under `comparison`, and in
the CLI, Markdown and HTML reports. Every
score uses the rubric's fixed `max_score` sum: a run with only 52 of 100
points judged is `52/100 + 48 pending`, never renormalised to 100%. Criteria
are paired by name across arms, and a criterion contributes a delta only when
both arms scored it for the same run.

The verdict is deliberately conservative:

- `incomplete` while a runner failed or any criterion is still pending
- `indicative` when there is one run per arm
- `inconclusive` when the delta sits inside the overlapping per-arm ranges
- `helps` for a positive delta with no regressions
- `hurts` for a negative delta
- `no measurable effect` when the delta is approximately zero (often a task
  leak or a non-discriminating scenario)

Overall with/without scores and the overall delta are means over scenarios;
each scenario's numbers are means over its runs. Per-arm min/max are kept in
the JSON (and are what the overlap check uses).

JSON is machine-readable and stable; Markdown is PR-comment friendly; HTML is
one self-contained file with inline CSS, escaped scenario/criterion text and
relative evidence links. A report also treats a declared run whose directory is
absent as a failed run, and the suite keeps the rubric's fixed denominator even
when only part of a checklist is scored.

The report's `timing` block separates wall clock, agent time and judge time,
and counts timed-out runs. New runs record `judge-result.json` beside
`result.json`, so the report can show where a slow suite spent its time.

### Caveats the report states up front

`evalctl report` prints a **Warnings** block directly under the verdict (and a
`warnings` array in `report.json`) for conditions that weaken a score:

- **Skill never used.** For the with arm, `evalctl` reads the accepted agent
  log and records `skill_use` in each run's `result.json` (`invoked`,
  `not-invoked` or `unknown`, plus the answer per installed skill). A skill
  counts as used when a `skill` tool call names it, or a tool call touches a
  path under `<skill-dir>/<name>/`. Recognised logs are opencode JSON and
  Claude `stream-json`; a missing, pruned or unrecognised log is `unknown`,
  never `not-invoked`. A scenario whose with-arm runs all proved
  `not-invoked` gets status `skill-not-exercised` and is left out of the
  headline With, Without, Delta and criterion tallies (`totals.skill_not_exercised`
  counts them); its own rows stay visible. A scenario that used the skill in
  only some runs is flagged `skill-partially-exercised` but kept. The report
  adds a **Skill use** table with one row per with-arm run.
- **Failed or retried attempts.** `attempt_failures` lists, per attempt kind
  (agent, hook, judge), how many attempts failed or were interrupted, how many
  there were in total, and the first error string. The accepted run results
  hide these because a later attempt supersedes them, so `failures: 0` can sit
  beside a dozen failed judge calls.
- **Same-model judge.** When the judge command names the same model as the
  agent (`--model`/`-m`, or identical commands), scores may be lenient. The
  report warns, and `evalctl suite` and `evalctl grade` print the same warning
  on stderr. It never blocks a run.
- **Low n.** With fewer than 3 runs per arm the report warns that variance
  cannot be estimated reliably. The verdict reason says so for n=1 (`one run
  per arm; variance cannot be estimated`) and n=2.

`evalctl grade` rewrites `summary.md`, `suite.json` (including `counts`),
`results.jsonl` and `grading/index.tsv` once it finishes, so a suite graded
after the fact reports its judged runs, scores and attempt totals instead of
the pre-grade state. The recorded duration and planned/executed counts are
kept.

The largest jq programs live in `scripts/jq/` and load with `jq -f`; the shell
scripts keep selection, staging and rendering. Both use `set -u`/`pipefail` to
match the rest of the harness.

## Cost and tokens

Cost capture is opt-in through the agent's structured output, or through a
usage hook:

```bash
# opencode: one JSONL step_finish event per step
scripts/eval-run.sh --plugin git-workflow --scenario pr-draft-read-replica \
    --agent-cmd 'opencode run --auto --format json "$(cat)"'

# Claude: one result object with total_cost_usd and usage
scripts/eval-run.sh --plugin git-workflow --scenario pr-draft-read-replica \
    --agent-cmd 'claude -p --output-format json'
```

`scripts/eval-usage.sh` parses `agent.log` and `judge.log` into
`usage.json`/`judge-usage.json`. It understands opencode's JSONL
`step_finish` events and Claude's result object; anything else is unknown.
Unknown is reported as `known:false`, never as zero. A CLI whose schema is not
built in (for example Codex) can supply a hook:

```bash
scripts/eval-suite.sh --agent-cmd 'codex exec -' \
    --usage-cmd './scripts/my-usage-hook.sh' \
    --judge-cmd 'claude -p --output-format json' \
    --judge-usage-cmd './scripts/my-judge-usage-hook.sh'
```

The hook runs with `USAGE_FILE`, `AGENT_LOG`, `RUN_DIR`, `SANDBOX`, `ARM`,
`RUN` and `AGENT_EXIT` set, and may write `USAGE_FILE` or print a normalized
JSON object. A failing or invalid hook is ignored and the built-in parser is
used.

Suites sum cost and tokens only over runs that reported them. `results.jsonl`
and `suite.json` carry `usage`/`judge_usage`, per-arm totals and coverage, and
`cost_known_runs`/`cost_total_runs`; `summary.md` gains a cost table. Report
token rows are split into input, output, reasoning, cache-read and total;
`report.json` carries the same breakdown, plus per-run means and with/without
deltas. Missing components are `null`, never zero. Report expenditure totals
include functional runs; with/without cost deltas, efficiency and baseline
comparisons use only scenarios with both arms. The report's
CLI/Markdown/HTML show `partial` coverage and withhold a per-point efficiency
claim unless both score and cost deltas are known for every run. Cost never
changes the verdict.

Compare a fresh run with a baseline to see what changed:

```bash
scripts/eval-report.sh results/eval-suite/<new> \
    --baseline results/eval-suite/<old> --format all
```

The comparison uses per-run means, so suites with different `--runs` counts
remain comparable. Deltas are calculated only over scenarios present in both
suites with identical rubric criterion names and maxima; cost and token deltas
use the same matched set. Every excluded scenario and its reason appears under
`comparison.excluded` (and in each rendered report). When no scenario is
compatible, the comparison reports `not comparable` and withholds every delta
instead of subtracting different workloads.

## Grading pending criteria

`scripts/eval-grade.sh` is the judgement half of a run. It builds a prompt from
`grading-request.json`, runs your judge command with the evidence directory as
its working directory, and accepts `judgement.json` only when it covers exactly
the pending criteria with the rubric's own maxima.

```bash
scripts/eval-grade.sh --suite results/eval-suite/<id> \
    --judge-cmd 'claude -p --permission-mode acceptEdits'
```

Grading can run later against any suite; re-running the suite with `--resume`
folds accepted judgements into the percentages without rerunning the agent.
`judge-result.json` records the judge command and grader version next to each
accepted judgement. Reusing a grade with a different judge, or with a changed
grader, is refused; pass `--regrade` to replace it.

The Go harness offers the same half:

```bash
bin/evalctl grade --suite results/eval-suite/<id> \
    --judge-cmd 'opencode run --auto --format json "$(cat)"' --auth-cli
```

`grade` accepts the same credential flags as `suite` (`--auth-cli`,
`--keychain`, `--auth-path`, `--pass-env`), because the judge is a CLI running
in its own home: a judge that needs login state must be given it, exactly like
the agent. When a suite ran with credentials, its summary prints the matching
hint (`evalctl grade --suite <suite> --auth-cli`).

Usage (as agent skills): `generate-evals plugins/<name>` and
`review-evals <pr-number>`; see `skills/*/SKILL.md`.
