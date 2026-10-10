# eval-workflow

Tessl-native eval authoring for this repository's plugins. The
`generate-evals` skill migrates a plugin to the tessl plugin structure
(`.tessl-plugin/plugin.json` + plugin-root `evals/`) and authors offline
verifiable scenarios for its skills, commands, and hooks, then runs the local
verification ladder (`tessl plugin lint`, criteria sums, per-scenario smoke
tests, `make test`).

Worked example: `plugins/git-workflow/evals/`.

Usage (as an agent skill): invoke `generate-evals plugins/<name>`; see
`skills/generate-evals/SKILL.md`.
