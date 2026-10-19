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

That stages the scenario in a throwaway sandbox with `HOME` redirected into it,
runs the agent once without the plugin's unit and once with it, records what
changed, scores the mechanical half of the rubric through the scenario's
`checks.sh`, and leaves a grading request for the criteria that need judgement.
The with-arm installs the plugin's skills and namespaced commands; the evidence
bundle (`snapshot.tar`, logs, before/after state) is always written.
Results land in the git-ignored `results/eval-run/`.

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
installs nothing because hook scenarios invoke the hook themselves. An unknown
unit, or a unit the plugin does not ship, fails before any run. Without
`--unit`, the with-arm keeps installing every skill and command in the plugin.

The sandbox `HOME` starts empty so fixtures and checks cannot touch your home
directory. Immediately before the agent runs, login state for the common CLIs
(`~/.claude.json`, `~/.claude/.credentials.json`, `~/.codex/auth.json`,
`~/.config/gh/hosts.yml`, `~/.local/share/opencode/auth.json`, and their
settings) is copied in, and on macOS the login Keychains directory is linked
into the sandbox because the CLI looks it up under `$HOME/Library`. That
symlink grants the sandboxed agent full access to your keychains, so it is a
deliberate tradeoff for authenticated runs; use `--no-auth` for a hermetic
run. The copied files and link are deleted again as soon as the agent exits
(or on interrupt), and `home/` is excluded from the evidence bundle. Pass
`--auth-path PATH` (repeatable) for any other credential or config file under
your home; a suite forwards every path. Pass `--no-auth` when the agent
authenticates from the environment (for example an API key).

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

Headless agents also need permission to use tools. The sandbox is the
boundary, so pass the CLI's non-interactive flag - for Claude Code,
`--dangerously-skip-permissions`, or `--permission-mode acceptEdits` when the
scenario only needs file edits; for opencode, `--auto`.

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
runs, timeout, skill directory and auth mode. The fingerprint covers the
scenario and the plugin's skills, commands, hooks and manifest, so editing a
unit reruns that plugin's scenarios.

A mechanical score is not a verdict. By default the suite exits non-zero only
when a scenario fails to run; `--strict` additionally fails when criteria are
still pending or a scenario scores no better with its unit than without it.

## `make eval` and the report

`make eval` wraps the suite and the report renderer. `SKILL=<name>` is
shorthand for `--unit skills/<name>`; `UNIT=` names any skill, command or
hook. `PLUGIN`, `RUNS`, `JOBS`, `AGENT`, `JUDGE`, `DRY` and `REPORT` map to
the suite options and the report format. `AGENT` is required for a real run
and `JUDGE` defaults to it.

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
[--out DIR]` is read-only. It never reruns an eval or calls a model. Every
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

The suite and report remain deliberately large Bash scripts with embedded jq;
extracting named `.jq` programs is a maintenance follow-up, not a behavioral
change. Both use `set -u`/`pipefail` to match the rest of the harness.

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
`cost_known_runs`/`cost_total_runs`; `summary.md` gains a cost table. The
report's CLI/Markdown/HTML show `partial` coverage and withhold a per-point
efficiency claim unless both score and cost deltas are known for every run.
Cost never changes the verdict.

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

Usage (as agent skills): `generate-evals plugins/<name>` and
`review-evals <pr-number>`; see `skills/*/SKILL.md`.
