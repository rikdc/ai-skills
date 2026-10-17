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
scripts/eval-run.sh --plugin <plugin> --scenario <scenario> --agent-cmd 'claude -p'
```

See `plugins/eval-workflow/skills/review-evals/SKILL.md` for how the pending
criteria of a run get graded.
