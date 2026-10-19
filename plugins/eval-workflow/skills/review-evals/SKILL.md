---
name: review-evals
description: Reviews eval scenarios and skills with judgement, covering what a deterministic gate cannot - whether a scenario discriminates, whether a rubric is fair to the unit it tests, whether a recording shim is faithful, and whether a skill's description and body guide an agent. Also grades the criteria an eval run left pending. Use when asked to review a pull request that touches a skill or an eval scenario, to judge an eval run, or to check that a set of evals actually proves what it claims.
user-invocable: true
argument-hint: "[pr-number | --changes <git-ref> | --result <path> | --suite <path>]"
allowed-tools: Read, Write, Edit, Glob, Grep, Bash(git:*), Bash(gh:*), Bash(jq:*), Bash(tar:*), Bash(tests/test-evals.sh:*), Bash(scripts/eval-run.sh:*), Bash(scripts/eval-suite.sh:*), Bash(scripts/eval-grade.sh:*), Bash(make:*)
---
# Review Evals - judgement over evals and skills

Reviews the parts of a skill or eval that a script cannot decide. The
deterministic gate (`tests/test-evals.sh`, run by `make test` and in CI)
already covers layout, rubric arithmetic, fixture hygiene, manifests and
frontmatter conventions. Never spend judgement on what that gate decides:
run it first and read its findings.

Nothing here runs in CI. A review needs a model, so it runs on a
contributor's machine, and it only ever reads refs you trust.

## Scope the review

- An eval run left pending criteria: `--result <dir>` (see "Grade a run").
- A finished suite: `--suite <dir>`; start from `grading/index.tsv` and
  `results.jsonl`, then grade what is pending.
- A pull request: `gh pr diff <number>` plus
  `tests/test-evals.sh --changes <base-ref>` to list the skills and scenarios
  it touches.
- A working branch: `tests/test-evals.sh --changes origin/main`.

## Judge a scenario

Read `task.md`, `criteria.json`, `inputs/setup.sh`, the scenario's optional
`checks.sh`, and the unit (`SKILL.md` or command) the scenario claims to
exercise. Then ask, in this order:

1. **Does it discriminate?** Name the run that scores well without the unit's
   guidance. If none exists - the task states the answer, or a criterion holds
   for any outcome (`git log -1`, an artifact the task hands over) - the
   scenario proves nothing. This is the failure that matters most.
2. **Can a correct run pass every criterion?** Look for a rubric item the
   task's own end state contradicts: a clean working tree when the task asks
   for an untracked artifact, a heading the final step moves, a file the
   fixture never creates.
3. **Is every criterion documented by the unit?** Grade only behaviour the
   unit states. If the behaviour is worth testing but undocumented, put it in
   the task instead of the rubric.
4. **Does the task leak the process?** If the task spells out the unit's own
   procedure step by step, an agent without the unit passes. Keep intent in
   the task; leave the process to the unit.
5. **Is the shim faithful?** A recording shim must parse like the real tool -
   long and short flags, `--flag=value`, `-` for stdin, values that begin with
   `-` - and record what the tool would infer, not only what it receives.
6. **Is the fixture honest?** Deterministic, prompt-free, executable, no
   network, writes only inside its workspace, and no checked-in file the script
   overwrites.

## Judge a skill

- **Description**: would an agent load this at the right moment? Look for the
  capability, the trigger terms a user would actually say, and a "Use when"
  clause. Absence of triggers is a finding.
- **Body**: does it guide rather than narrate? Concrete steps, no restatement
  of the description, no dead references, no procedure the tooling already
  encodes.
- **Boundaries**: permissions scoped to what the work needs; nothing that
  grants more than it appears to.

## Grade a run

An eval run stages a scenario, runs an agent with and without the unit, and
scores whatever a script can score. What remains is in
`results/eval-run/<plugin>/<scenario>/<arm>/run-<n>/grading-request.json`:

1. Read `judgement_pending` and `evidence`; unpack `snapshot.tar` to see the
   state the run produced, and read `before.txt` for the starting state.
2. Score each pending criterion against the rubric's own description, giving
   partial credit where the description implies it.
3. Write `judgement.json` next to the request, same shape as `checks.sh`
   output: `[{"name", "score", "max", "note"}]`.
4. Report the headline the run exists to produce: the with-arm total versus
   the without-arm total. A large positive delta is the unit earning its
   place; near-zero means the agent already did it, or the scenario cannot
   tell. Never report the with-arm score alone.

A suite run keeps the same files under `results/eval-suite/<id>/runs/` and adds
`grading/index.tsv`. Grade a whole suite in one pass with
`scripts/eval-grade.sh --suite <dir> --judge-cmd '<your agent CLI>'`; the
adapter rejects a judgement that does not cover exactly the pending criteria,
and re-running the suite with `--resume` folds accepted judgements into its
percentages. When reviewing the numbers, still read a sample of the evidence:
a judge that scored without looking is indistinguishable from a good one in
the totals.

## Report

Findings are specific: file, line, what breaks, and the smallest fix. Rank
them - a scenario that cannot discriminate outranks a wording nit. Then:

- Locally: print the review, or write it beside the results.
- On a pull request: `gh pr review <number> --comment --body-file <file>` -
  never approve on a judgement call, and never let a review of a fork's code
  run anything.

## Non-goals

- Do not re-check what `tests/test-evals.sh` decides; run it and move on.
- Do not grade a scenario against behaviour its unit does not document.
- Do not run inside CI, and do not put model credentials in repository
  secrets.
