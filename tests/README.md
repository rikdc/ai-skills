# Tests

Test suites for the hooks and marketplace configuration in this repository.

Run everything through the Makefile:

```bash
make test
```

That runs the three suites below in order. Each is a standalone script, so an
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

## Related Checks

`make test` covers the suites above. Two other checks run separately, and in
CI:

```bash
make lint          # ShellCheck, markdownlint, and claudelint
make validate      # Plugin manifest, hook scripts, and dependencies
```
