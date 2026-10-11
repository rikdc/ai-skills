---
name: generate-evals
description: Generates and verifies eval scenarios for a repository plugin — covering its skills, commands, and hooks — using the tessl-compatible evals/ layout (task.md, criteria.json, scenario.json) so they run under `tessl eval run` and grade offline with any judge. Migrates the plugin to tessl structure when needed (adds .tessl-plugin/plugin.json, colocates evals/), authors self-gradable scenarios, runs the offline verification ladder, and commits. Use when the user asks to build evals for a plugin, generate eval scenarios, or verify that a skill actually works.
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
- `--dry-run`: print the migration plan and the scenario list; write nothing.

## Workflow

### 1. Inventory the plugin
1. Read `.claude-plugin/plugin.json`, the README, and the directory tree.
2. Classify the content: `skills/<name>/SKILL.md`, `commands/*.md`,
   `hooks/*.sh` (or scripts exercised as hooks), or a mix.
3. Note existing `evals/` content and any **nested** `skills/*/.tessl-plugin/`
   directories that need migration.
4. For hooks, also read the relevant `tests/test-*.sh` to understand the
   contract - but never let a scenario duplicate unit-level test coverage.

### 2. Migrate to tessl structure (only when needed)
1. Add `.tessl-plugin/plugin.json` mirroring the Claude manifest's
   `version`/`description`. Use `workspace/plugin-name` format for `name`
   (the workspace is the first segment of the root `tessl.json` name), and
   `"private": true`.
2. Point the manifest at what the plugin ships: `"skills": "./skills/"` (or
   an array of skill dirs) when skills exist, `"commands": "./commands/"`
   when commands exist. For hooks-only plugins declare at least one content
   key - `tessl plugin lint` rejects a manifest with none of skills,
   commands, rules, `hooks`, or `nativeHooks`. These Claude-native hooks
   translate to the tessl `hooks` (cross-agent) schema: event names
   (`PreToolUse`, matcher, command array) with
   `"${TESSL_PLUGIN_DIR}"/hooks/<script>.sh` as the command; verify with
   lint.
3. Remove any **nested** per-skill `skills/*/.tessl-plugin/` manifests - the
   parent manifest supersedes them. Mention the removal in the commit body.
4. `evals/` belongs at the plugin root, never under `skills/`.
5. HARD BOUNDARY: never modify `.claude-plugin/**`, hook scripts, or hook
   registration. Migration adds files only.

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

### 4. Make it gradeable offline
Scenarios run in a sandbox without network or credentials.
1. `inputs/setup.sh` is POSIX bash: `#!/usr/bin/env bash`, `set -euo
   pipefail`, `git init -q` plus `git config user.email/user.name`, heredoc
   fixtures, and an echo of the final state. Deterministic and prompt-free.
2. Build recording shims for external CLIs the agent would call (a `gh`
   shim that writes `title`, `body`, `draft`, `base` to `pr-record.json`),
   so the graded call is observable. See the git-workflow PR scenario.
3. Require durable evidence artifacts in task.md (`workflow.md`,
   `commit_log.txt`, plan files) when criteria need to reference the steps
   the agent took, not just the end state.
4. State plainly in task.md that network actions must be skipped, and give
   the equivalent dry-run instruction instead.

### 5. Verification ladder (run ALL before claiming done)
1. Every `criteria.json`: `jq -e .` passes and
   `jq '[.checklist[].max_score] | add'` returns exactly 100.
2. `tessl plugin lint <plugin>` reports valid (`tessl` lives at
   `~/.local/bin`).
3. Smoke-test every scenario: copy `inputs/` to a fresh
   `/tmp/eval-smoke-<scenario>/` directory, `bash inputs/setup.sh`, assert
   exit 0 and that the fixture state matches the task (e.g. the change is
   staged, the shim is executable). Remove the temp dirs afterwards.
4. For one representative scenario per unit type, play the intended
   solution path end-to-end yourself (produce the artifact the criteria
   grade) to prove the rubric is answerable, then clean up.
5. `make test && make validate` at the checkout root. Run `make
   sync-codex-skills` first if SKILL.md files were added or renamed.
6. Optional and only when explicitly asked: `tessl eval run <plugin>` -
   server-side, needs `tessl login`.

### 6. Commit
Conventional commits, scoped to the plugin, no other files touched, do not
push:
1. If migration happened: `chore(<plugin>): adopt tessl plugin structure`.
2. `test(<plugin>): add eval scenarios for <units>` - one commit either way
   for the scenarios themselves.

### 7. Batching across plugins (orchestrator notes)
1. One worktree per plugin, branched off the branch that carries this skill:
   `git worktree add .worktrees/eval-<plugin> -b evals/<plugin> skill/eval-workflow`.
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
