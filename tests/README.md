# Tests

Test suites for the hooks and marketplace configuration in this repository.

Run everything through the Makefile:

```bash
make test
```

That runs the suites below in order. Each is a standalone script, so an
individual suite can also be run directly.

## Test Files

### `test-scanner.sh`

Functional tests for the MCP security scanner hook. Covers detection of
sensitive data in MCP request payloads, and confirms non-MCP tool calls pass
through untouched.

```bash
./tests/test-scanner.sh
```

### `test-protect-main-branch.sh`

Tests for the protected branch hook. The suite is organised around the
distinction the hook has to get right — blocking writes to a protected branch
without blocking harmless commands:

- Read-only and non-git Bash on `main` is allowed
- Read-only git commands and lookalike strings are allowed
- Genuine writes to a protected branch are blocked
- Writes hidden behind wrappers, pipelines, and `.git/` paths are blocked
- Feature branches are unaffected regardless of command
- Creating a feature branch is allowed, since it is the suggested remedy

```bash
./tests/test-protect-main-branch.sh
```

### `test-marketplace.sh`

Consistency checks between each plugin and the marketplace manifest. Every
plugin's own `.claude-plugin/plugin.json` is treated as the source of truth;
`marketplace.json` and the docs are derivative, so drift is always reported
against the plugin manifest.

This suite deliberately does not use `set -e`, so a single run reports every
failure rather than stopping at the first.

```bash
./tests/test-marketplace.sh
```

### `test-evals.sh`

Deterministic checks over the eval scenarios and skills, with no model and no
network, so it can run in CI on every pull request:

- Scenario layout and naming (descriptive names, never `scenario-N`)
- Rubric arithmetic — 8-13 checklist items whose `max_score` sums to 100
- Fixture hygiene — one executable `inputs/setup.sh` that keeps its writes
  inside the workspace and leaves the graded repository state clean
- Plugin manifests — the tessl manifest matches the Claude manifest and the
  workspace in `tessl.json`
- Skill frontmatter — spec limits, duplicate keys, and dead path references

Checks that need judgement — does a scenario discriminate, is a rubric fair to
the unit it tests, is a recording shim faithful to the real CLI — are
deliberately out of scope. Those need an LLM and run on a contributor's
machine, not in public CI.

```bash
./tests/test-evals.sh
./tests/test-evals.sh --root /path/to/copy     # check a copy of the repo
./tests/test-evals.sh --changes origin/main    # list the skills/scenarios a ref touched
```

### `test-eval-usage.sh`

Deterministic parser tests for `scripts/eval-usage.sh`, with local fixture
logs and hook scripts only:

- opencode JSONL `step_finish` summation and Claude result JSON
- unknown and partial token-only usage, with invalid numbers ignored
- hooks that write `USAGE_FILE` or print JSON
- failing/invalid hook fallback to the built-in parser, with the hook's own
  stderr preserved for the caller's `usage.log`

```bash
./tests/test-eval-usage.sh
```

### `test-eval-suite.sh`

Deterministic tests for the local eval harness — no model, no network, so it
runs in CI:

- the runner's and suite's own selftests
- discovery matches `plugins/*/evals/*/` and ignores worktrees
- `--plan` invocation counts, the `--jobs` hint, and shards that partition
  without overlap
- `--unit` selection: skills install only the named skill, commands only the
  named command, hooks are accepted without an install, and unknown units
  fail clearly
- manifest/config unit recording and `--resume` equality, including manifest
  rewriting for a narrowed resume and multiple units forwarded separately
- an uncreatable `--out` fails before any job starts
- a bad `--changed` ref fails by name instead of being masked
- an uncopyable credential source fails the run, hints at `--no-auth`/
  `--auth-path`, and still removes the sandbox
- TERM stops the suite with status 143 and kills the in-flight agent and its
  descendants
- a failed evidence tar fails the run instead of grading without a snapshot
- parallel execution, `--resume`, and aggregation, with a partial rubric kept
  against the fixed 100-point denominator
- usage capture: suite cost sums, partial coverage, unknown never summed as
  zero, judge timing, judge-usage backfill, and `--resume` usage backfill
- the judge adapter rejects invalid judgements and folds valid ones into
  per-arm percentages
- `--strict` fails a suite that does not discriminate

```bash
./tests/test-eval-suite.sh
```

### `test-eval-report.sh`

Deterministic tests for the report renderer and `make eval`, with synthetic
suite directories and no model or network:

- the fixed 100-point denominator: `52/100` with unjudged criteria stays
  pending rather than being renormalised
- per-criterion pairing by name and mean deltas across runs
- declared runs with missing directories stay incomplete (never a false
  `helps`), and `results.jsonl` runner failures reach the verdict
- regression detection and every verdict state (`incomplete`, `indicative`,
  `inconclusive`, `helps`, `hurts`, `no measurable effect`)
- unwritable `--out` directories fail instead of printing success
- cost/usage in CLI, Markdown, JSON and HTML, including unknown/partial states
  and the efficiency claim only when the data supports it
- input/output/reasoning/cache token splits, agent/judge/wall timing, and
  `--baseline` comparisons of score, per-run cost and token deltas
- JSON, Markdown and self-contained HTML generation, including escaping of
  scenario and criterion text
- `make eval` dry-run selection (`SKILL=`, `UNIT=`), the required-`AGENT`
  guard, and rejection of `SKILL` with `UNIT`

```bash
./tests/test-eval-report.sh
```

## Related Checks

`make test` covers the suites above. Two other checks run separately, and in
CI:

```bash
make lint          # ShellCheck, markdownlint, and claudelint
make validate      # Plugin manifest, hook scripts, and dependencies
```

## Checks That Need a Model

Reviewing a scenario or skill with judgement, and running an eval, need a model,
so they run on a contributor's machine rather than in CI:

```bash
scripts/eval-run.sh --selftest                 # runner plumbing, no model
scripts/eval-suite.sh --selftest               # suite plumbing, no model
scripts/eval-run.sh --plugin <plugin> --scenario <scenario> --agent-cmd 'claude -p'
scripts/eval-suite.sh --agent-cmd 'claude -p' --jobs 4
```

See `plugins/eval-workflow/skills/review-evals/SKILL.md` for how the pending
criteria of a run get graded, and `plugins/eval-workflow/README.md` for the
suite's selection, resume and reporting options.
