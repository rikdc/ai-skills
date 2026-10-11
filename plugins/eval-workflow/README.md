# eval-workflow

Eval scenario authoring for this repository's plugins, in the tessl-compatible evals/ format. The
`generate-evals` skill authors offline verifiable scenarios at the plugin
root `evals/` for its skills, commands, and hooks, then runs the local
verification ladder (`tessl plugin lint`, criteria sums, per-scenario smoke
tests, `make test`).

Worked example: `plugins/git-workflow/evals/`.

Usage (as an agent skill): invoke `generate-evals plugins/<name>`; see
`skills/generate-evals/SKILL.md`.
