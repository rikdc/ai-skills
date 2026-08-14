# AGENTS.md

## Repository purpose

This repository is a Claude Code marketplace and a shared collection of
development-agent workflows. The Claude-specific plugin manifests live under
`plugins/*/.claude-plugin/`; Codex-specific integration lives under `.codex/`.
Do not assume those packaging formats are interchangeable.

## Codex skills

The reusable plugin skills are exposed to Codex through symlinks in
`.codex/skills/`. Their sources of truth are the `plugins/*/skills/`
directories.

- Use a matching skill when a task clearly fits its stated workflow.
- Edit the source skill, not its `.codex/skills/` symlink.
- Keep skills provider-neutral unless a platform-specific instruction is
  essential.
- After adding, renaming, or removing a plugin skill, run
  `make sync-codex-skills` to update `.codex/skills/` and commit the result.
  `make test` fails if the symlinks drift from `plugins/*/skills/`.

## Commands and verification

Run relevant checks before handing off changes:

```bash
make test              # Hook tests, marketplace and Codex symlink consistency
make validate           # Plugin manifest, scripts, and dependencies
make lint               # ShellCheck/Markdown lint when installed
make sync-codex-skills  # Regenerate .codex/skills/ after plugin skill changes
```

`jq`, `grep`, `awk`, and `mktemp` are required for the security-hook tooling.

## Boundaries

- Keep Claude plugin manifests and hooks unchanged unless the task explicitly
  targets Claude Code.
- The shell scripts in `plugins/security-hooks/` are portable utilities, but
  their Claude hook registration is not a Codex configuration.
- Add Codex-specific hooks, MCP settings, or a `.codex-plugin` manifest only
  when that integration is explicitly being implemented.
