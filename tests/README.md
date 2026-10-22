# Tests

Test coverage for the hooks, plugins, and the `evalctl` Go harness in this
repository.

Run everything through the Makefile:

```bash
make test
```

`make test` runs the Go tests and `go vet`, the deterministic `evalctl` gate,
and the shell suites below, in order. Each shell suite is a standalone script,
so it can also be run directly.

## Go test packages

`go test ./...` exercises the `evalctl` harness:

- `cmd/evalctl`: process entry, signal context, and exit-code mapping.
- `internal/cli`: Cobra commands and flag wiring.
- `internal/scenario`, `internal/check`: discovery, selection, plan, and the
  deterministic authoring gate.
- `internal/runner`, `internal/runner/attempts`: workspace staging, attempts,
  `current.json`, projections, and crash recovery.
- `internal/suite`, `internal/judge`, `internal/report`, `internal/reportio`:
  orchestration, command judging, report analysis and rendering.
- `internal/provider/command`, `internal/executor`, `internal/usage`,
  `internal/platform/*`: the command backend, process lifecycle, and usage
  normalization.
- `internal/selftest`: the synthetic end-to-end fixture.

```bash
go test ./...
go test -race ./...   # Linux
make evalctl-check    # gofmt, go vet, and the deterministic evalctl gate
```

## Shell suites

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

### `test-codex-skills.sh`

Checks that `.codex/skills/*` mirrors `plugins/*/skills/*` exactly, so Codex
never sees a stale or missing skill. It wraps `scripts/sync-codex-skills.sh
--check`.

```bash
./tests/test-codex-skills.sh
```

## Deterministic eval authoring gate

`bin/evalctl check` runs the deterministic checks over the eval scenarios and
skills, with no model and no network, so it can run in CI on every pull
request. `make test` builds `bin/evalctl` and runs it:

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
bin/evalctl check
bin/evalctl check --root /path/to/copy      # check a copy of the repo
bin/evalctl check --changes origin/main     # list the skills/scenarios a ref touched
```

## Related Checks

`make test` covers the Go tests plus the shell suites above. These checks run
separately, and in CI:

```bash
make check         # evalctl-check + the deterministic evalctl selftest
make lint          # ShellCheck, markdownlint, and claudelint
make validate      # Plugin manifest, hook scripts, and dependencies
```

## Checks That Need a Model

Reviewing a scenario or skill with judgement, and running an eval, need a model,
so they run on a contributor's machine rather than in CI:

```bash
bin/evalctl selftest                                    # harness plumbing, no model
bin/evalctl run --plugin <plugin> --scenario <scenario> --agent-cmd 'claude -p'
bin/evalctl suite --agent-cmd 'claude -p' --jobs 4
```

See `plugins/eval-workflow/skills/review-evals/SKILL.md` for how the pending
criteria of a run get graded, and `plugins/eval-workflow/README.md` for the
suite's selection, resume and reporting options.
