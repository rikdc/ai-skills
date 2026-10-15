---
name: generate-evals
description: Generates and verifies eval scenarios for a repository plugin — covering its skills, commands, and hooks — using the tessl-compatible evals/ layout (task.md, criteria.json, scenario.json) so they run under `tessl eval run` and grade offline with any judge. Authors self-gradable scenarios, runs the offline verification ladder, and commits. This skill never restructures plugins. Use when the user asks to build evals for a plugin, generate eval scenarios, or verify that a skill actually works.
user-invocable: true
argument-hint: "<plugin-path> [--count N] [--dry-run]"
allowed-tools: Read, Write, Edit, Glob, Grep, Bash(git *), Bash(make *), Bash(jq *), Bash(tessl plugin lint:*), Bash(mkdir *), Bash(mv *), Bash(chmod *), Bash(cp -R *), Bash(bash inputs/*), Bash(bash /tmp/eval-*), Bash(rm -rf /tmp/eval-*)
---
# Generate Evals - eval scenarios for repository plugins

Builds and verifies eval scenarios that prove a plugin's skills,
commands, or hooks change agent behaviour. Scenarios live **inside the
plugin directory** at `evals/`, so `tessl eval run plugins/<name>` can run
them against the plugin. `plugins/git-workflow/evals/` in this repository is
the canonical worked example - read one of its scenarios before writing new
ones.

## Inputs

- `<plugin-path>`: the plugin directory relative to the repository root
  (for example `plugins/dev-skills`).
- `--count N`: target scenarios per unit. Default is 3; fewer is acceptable
  for small units. One "unit" is a skill, a command, or a hook script.
- `--dry-run`: print the scenario list and any coverage gaps; write nothing.

## Workflow

### 1. Inventory the plugin
1. Read `.claude-plugin/plugin.json`, the README, and the directory tree.
2. Classify the content: `skills/<name>/SKILL.md`, `commands/*.md`,
   `hooks/*.sh` (or scripts exercised as hooks), or a mix.
3. Note existing `evals/` content so new scenario names do not collide with
   it.
4. For hooks, also read the relevant `tests/test-*.sh` to understand the
   contract - but never let a scenario duplicate unit-level test coverage.

### 2. Standing structure and boundaries
1. Scenarios live at the plugin root: `plugins/<plugin>/evals/<scenario>/`,
   beside `skills/` - never inside a skill directory and never at the
   repository root. Do not create, delete, or restructure
   `.tessl-plugin/plugin.json`; if a plugin is missing its tessl manifest,
   stop and ask - do not invent one.
2. HARD BOUNDARY: never modify `.claude-plugin/**`, hook scripts, or hook
   registration. Scenarios add files only.

### 3. Design scenarios per behaviour group
1. Default three scenarios per unit: the happy path; an edge or trap case
   (boundary, refusal path, error handling); a formatting or anti-pattern
   guard. Two is the floor for tiny units.
2. Every scenario must name an exact documented behaviour from the unit -
   never invent features the unit does not claim.
3. Scenario directory names: lowercase-hyphen, unit-prefixed when the plugin
   has several units (`commit-failing-checks`, `pr-draft-read-replica`).
   NEVER name hand-written scenarios `scenario-N`: tessl merges/overwrites
   canonical `scenario-N` directories on re-download. Keep existing
   tessl-generated `scenario-N` dirs untouched.
4. Each scenario directory contains:
   - `task.md`: Problem/Feature Description written in realistic narrative
     (a colleague, a project, a change pending review), then **Your Task**
     as numbered steps, then **Output** listing the exact artifacts.
   - `scenario.json`: `{"description": "...", "include": ["./inputs"]}` -
     always with an `inputs/` directory, even a minimal one.
   - `criteria.json`: `{"context", "type": "weighted_checklist",
     "checklist": [...]}` with 8-13 items whose `max_score` sums to exactly
     100. Every item must be gradeable from observable state: file presence
     or content, git state, exit codes, or recorded output.
5. Make every criterion check something the fixture can actually produce.
   Read the fixture and the rubric together before committing: if the
   criteria demand a style or a state the fixture contradicts, or a check
   that any outcome satisfies (e.g. `git log -1`, which always prints
   something), the scenario cannot discriminate. Prefer countable checks
   (`git rev-list --count HEAD`, recorded fields) over "looks right" ones.
6. Check the rubric against the end state the task itself asks for. A
   criterion that a correct run cannot satisfy is broken: requiring a clean
   working tree when the task tells the agent to leave an artifact
   untracked, or requiring a heading the task's own final step moves away
   from. Also check that the task does not demand behaviour the unit's
   SKILL.md never documents (e.g. choosing a release version when the skill
   only releases a version it is given) - if the behaviour is not
   documented, hand it to the agent in the task instead of grading it.

### 4. Make it gradeable offline
Scenarios run in a sandbox without network or credentials.
1. `inputs/setup.sh` is POSIX bash: `#!/usr/bin/env bash`, `set -euo
   pipefail`, `git init -q` plus `git config user.email/user.name`, heredoc
   fixtures, and an echo of the final state. Deterministic, prompt-free and
   executable (`chmod +x`). Keep the name `inputs/setup.sh` across scenarios
   and state the invocation tolerantly in task.md - `bash inputs/setup.sh`,
   or `bash setup.sh` if a harness flattens the fixture into the working
   directory - so the instruction survives either layout. Every checked-in
   fixture file is either written by the script or read by it, never both: a
   copy the script overwrites is dead weight that drifts silently.
2. Build recording shims for external CLIs the agent would call (a `gh`
   shim that writes `title`, `body`, `draft`, `base` and `head` to
   `pr-record.json`), so the graded call is observable. See the git-workflow
   PR scenario. A shim must parse its CLI the way the real tool does: long
   and short flags, `--flag=value`, and `-` for stdin, taking the token after
   a value-taking flag as that value even when it starts with `-`. Record
   every field the rubric grades, including values the tool infers rather
   than receives as an argument (the branch `gh pr create` derives from the
   working tree) and flags whose effects the real tool computes (`--fill`
   taking title and body from the branch's commits) - otherwise a correct
   invocation can be graded wrong.
3. Install shims inside the fixture (for example `./bin`) and keep them out
   of the fixture's `git status` with `.git/info/exclude`. Never write to
   `$HOME` or anywhere outside the scenario workspace: authors run every
   setup script locally, so a `$HOME/bin` install silently replaces the
   contributor's real tools, and a temp directory elsewhere does not undo it.
4. Require durable evidence artifacts in task.md (`workflow.md`,
   `commit_log.txt`, plan files) when criteria need to reference the steps
   the agent took, not just the end state.
5. State plainly in task.md that network actions must be skipped, and give
   the equivalent dry-run instruction instead.
6. The fixture's own files must not contaminate the graded repository state.
   A setup that runs `git init` in the working tree leaves `inputs/` (and,
   when a harness flattens the fixture, the setup script itself) untracked;
   exclude them or the "clean working tree" criteria fail on a correct
   solution.

### 5. Verification ladder (run ALL before claiming done)
1. Every `criteria.json`: `jq -e .` passes and
   `jq '[.checklist[].max_score] | add'` returns exactly 100.
2. `tessl plugin lint <plugin>` reports valid (`tessl` lives at
   `~/.local/bin`). If it fails for reasons unrelated to your scenarios,
   stop and report - do not modify manifests.
3. Smoke-test every scenario: copy `inputs/` to a fresh
   `/tmp/eval-smoke-<scenario>/` directory, `bash inputs/setup.sh`, assert
   exit 0 and that the fixture state matches the task (e.g. the change is
   staged, the shim is executable). Remove the temp dirs afterwards.
4. Exercise the graded path, not just the setup: run each recording shim the
   way a plausible agent would call it (both flag styles, both a correct and
   an incorrect invocation) and confirm the recorded fields distinguish them.
   Run the setup in both layouts (`inputs/` preserved and flattened) when the
   harness's behaviour is unknown.
5. For one representative scenario per unit type, play the intended
   solution path end-to-end yourself (produce the artifact the criteria
   grade) to prove the rubric is answerable, then clean up.
6. `make test && make validate` at the checkout root. Run `make
   sync-codex-skills` first if SKILL.md files were added or renamed.
7. Optional and only when explicitly asked: `tessl eval run <plugin>` -
   server-side, needs `tessl login`.

### 6. Commit
Conventional commits, scoped to the plugin, no other files touched, do not
push: `test(<plugin>): add eval scenarios for <units>`. Manifests and other
plugin files stay untouched.

### 7. Batching across plugins (orchestrator notes)
1. One worktree per plugin, branched off the commit that carries this skill
   (the current HEAD): `git worktree add .worktrees/eval-<plugin> -b
   evals/<plugin> HEAD`.
2. One sub-agent per plugin. Its brief must be self-contained: worktree
   path, plugin path, content-type hints, this skill's workflow verbatim,
   the verification ladder, and the commit rules. Sub-agents may decompose a
   many-skill plugin internally, but they own integration, verification and
   the commits.
3. Merge order into the integration branch: the skill branch first, then
   each `evals/<plugin>` branch (they are stacked), then `make test &&
   make validate` on the merged result.

## Non-goals
- Never grade behaviour the unit does not document; if a unit is too thin to
  evaluate, say so instead of padding.
- No placeholder directories: a scenario without runnable inputs or an
  answerable rubric fails the ladder.
- Authoring is local; tessl server-side generation and downloading are out
  of scope.
