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
naming, rubric arithmetic, fixture hygiene, plugin manifests, and skill
frontmatter conventions. `--changes <ref>` lists the skills and scenarios a
ref touched.

## Running an eval

Running an eval needs a model, so it happens on a contributor's machine, never
in CI:

```bash
scripts/eval-run.sh --plugin git-workflow --scenario pr-draft-read-replica \
    --agent-cmd 'claude -p'
```

That stages the scenario in a throwaway sandbox with `HOME` redirected into it,
runs the agent once without the plugin's skills and once with them, records what
changed, scores the mechanical half of the rubric through the scenario's
`checks.sh`, and leaves a grading request for the criteria that need judgement.
Results land in the git-ignored `results/eval-run/`.

Usage (as agent skills): `generate-evals plugins/<name>` and
`review-evals <pr-number>`; see `skills/*/SKILL.md`.
