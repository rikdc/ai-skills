package check

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type repo struct {
	root   string
	plugin string
}

func write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func criteriaJSON(t *testing.T, mutate func([]map[string]any) []map[string]any) string {
	t.Helper()
	items := make([]map[string]any, 0, 8)
	for i := range 8 {
		items = append(items, map[string]any{
			"name":        "item-" + string(rune('a'+i)),
			"description": "description",
			"max_score":   12.5,
		})
	}
	if mutate != nil {
		items = mutate(items)
	}
	data, err := json.Marshal(map[string]any{
		"context":   "fixture context",
		"type":      "weighted_checklist",
		"checklist": items,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func validRepo(t *testing.T) repo {
	t.Helper()
	root := t.TempDir()
	r := repo{root: root, plugin: "alpha"}
	write(t, filepath.Join(root, "tessl.json"), `{"name":"ws/repo"}`, 0o644)
	r.writeManifests(t, `{"name":"alpha","description":"desc","version":"1.0.0"}`,
		`{"name":"ws/alpha","description":"desc","version":"1.0.0","private":true}`)
	r.writeScenario(t, `{"description":"demo","include":["./inputs"],"unit":"skills/alpha"}`,
		"# Task\n\nDo it.\n", criteriaJSON(t, nil),
		"#!/usr/bin/env bash\nset -euo pipefail\necho fixture.txt\n")
	r.writeSkill(t, `---
name: alpha
description: A sufficiently long description that carries activation triggers for the alpha skill.
allowed-tools: Read, Grep
---
# Alpha
`)
	return r
}

func (r repo) writeManifests(t *testing.T, claude, tessl string) {
	t.Helper()
	write(t, filepath.Join(r.root, "plugins", r.plugin, ".claude-plugin", "plugin.json"), claude, 0o644)
	write(t, filepath.Join(r.root, "plugins", r.plugin, ".tessl-plugin", "plugin.json"), tessl, 0o644)
}

func (r repo) writeScenario(t *testing.T, scenario, task, criteria, setup string) {
	t.Helper()
	dir := filepath.Join(r.root, "plugins", r.plugin, "evals", "demo")
	write(t, filepath.Join(dir, "scenario.json"), scenario, 0o644)
	write(t, filepath.Join(dir, "task.md"), task, 0o644)
	write(t, filepath.Join(dir, "criteria.json"), criteria, 0o644)
	write(t, filepath.Join(dir, "inputs", "setup.sh"), setup, 0o755)
	write(t, filepath.Join(dir, "inputs", "fixture.txt"), "fixture\n", 0o644)
}

func (r repo) writeSkill(t *testing.T, content string) {
	t.Helper()
	write(t, filepath.Join(r.root, "plugins", r.plugin, "skills", "alpha", "SKILL.md"), content, 0o644)
}

func runGate(t *testing.T, r repo) Result {
	t.Helper()
	result, err := Run(r.root)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func messages(result Result) string {
	var lines []string
	for _, finding := range result.Findings {
		lines = append(lines, finding.Message)
	}
	return strings.Join(lines, "\n")
}

func assertFinding(t *testing.T, result Result, want string) {
	t.Helper()
	if !strings.Contains(messages(result), want) {
		t.Fatalf("missing finding %q in:\n%s", want, messages(result))
	}
}

func assertNoFindings(t *testing.T, result Result) {
	t.Helper()
	if !result.OK() {
		t.Fatalf("unexpected findings:\n%s", messages(result))
	}
}

func TestCleanRepository(t *testing.T) {
	r := validRepo(t)
	assertNoFindings(t, runGate(t, r))
}

func TestPlacementFindings(t *testing.T) {
	r := validRepo(t)
	write(t, filepath.Join(r.root, "evals", "stray", "scenario.json"), "{}", 0o644)
	write(t, filepath.Join(r.root, "plugins", r.plugin, "skills", "alpha", "evals", "x", "scenario.json"), "{}", 0o644)
	result := runGate(t, r)
	assertFinding(t, result, "evals/ exists at the repository root")
	assertFinding(t, result, "plugins/alpha/skills/alpha/evals/: scenarios belong at the plugin root")
}

func TestScenarioFindings(t *testing.T) {
	r := validRepo(t)
	dir := filepath.Join(r.root, "plugins", r.plugin, "evals", "demo")
	os.Rename(filepath.Join(dir, "task.md"), filepath.Join(dir, "task.bak"))
	write(t, filepath.Join(dir, "task.md"), "", 0o644)
	write(t, filepath.Join(dir, "scenario.json"),
		`{"description":"","include":"./inputs","unit":"skills/alpha","units":["skills/nope"],"mode":"weird"}`, 0o644)
	result := runGate(t, r)
	for _, want := range []string{
		"/task.md: empty",
		"description is empty",
		"'include' must be an array",
		"use either 'unit' or 'units', not both",
		"unit 'skills/nope' does not exist in plugins/alpha/skills",
		"mode must be comparison or functional",
	} {
		assertFinding(t, result, want)
	}
}

func TestScenarioIncludeAndExecutionFindings(t *testing.T) {
	r := validRepo(t)
	dir := filepath.Join(r.root, "plugins", r.plugin, "evals", "demo")
	write(t, filepath.Join(dir, "scenario.json"),
		`{"description":"d","include":["./missing"],"unit":"skills/alpha","execution":{"backend":"hook"}}`, 0o644)
	result := runGate(t, r)
	assertFinding(t, result, "include path does not exist: ./missing")
	assertFinding(t, result, "execution backend 'hook' requires functional mode")
	assertFinding(t, result, "execution backend 'hook' requires a hook unit")

	write(t, filepath.Join(dir, "scenario.json"),
		`{"description":"d","include":[],"unit":"skills/alpha","execution":{"backend":"martian"}}`, 0o644)
	assertFinding(t, runGate(t, r), "execution backend must be agent or hook")
}

func TestFunctionalScenarioFindings(t *testing.T) {
	r := validRepo(t)
	dir := filepath.Join(r.root, "plugins", r.plugin, "evals", "demo")
	write(t, filepath.Join(dir, "scenario.json"),
		`{"description":"d","include":["./inputs"],"unit":"skills/alpha","mode":"functional"}`, 0o644)
	assertFinding(t, runGate(t, r), "functional mode requires a hook unit")
}

func TestScenarioCanonicalNameFinding(t *testing.T) {
	r := validRepo(t)
	// A tessl downloader-owned scenario-N directory is flagged even when
	// everything else about it is valid.
	dir := filepath.Join(r.root, "plugins", r.plugin, "evals", "scenario-12")
	write(t, filepath.Join(dir, "scenario.json"),
		`{"description":"demo","include":["./inputs"],"unit":"skills/alpha"}`, 0o644)
	write(t, filepath.Join(dir, "task.md"), "# Task\n\nDo it.\n", 0o644)
	write(t, filepath.Join(dir, "criteria.json"), criteriaJSON(t, nil), 0o644)
	write(t, filepath.Join(dir, "inputs", "setup.sh"),
		"#!/usr/bin/env bash\nset -euo pipefail\necho fixture.txt\n", 0o755)
	write(t, filepath.Join(dir, "inputs", "fixture.txt"), "fixture\n", 0o644)
	assertFinding(t, runGate(t, r), "canonical tessl name")
}

func TestDuplicateUnits(t *testing.T) {
	r := validRepo(t)
	dir := filepath.Join(r.root, "plugins", r.plugin, "evals", "demo")
	write(t, filepath.Join(dir, "scenario.json"),
		`{"description":"d","include":[],"units":["skills/alpha","skills/alpha"]}`, 0o644)
	assertNoFindings(t, runGate(t, r))
}

func TestRubricFindings(t *testing.T) {
	r := validRepo(t)
	dir := filepath.Join(r.root, "plugins", r.plugin, "evals", "demo")
	write(t, filepath.Join(dir, "criteria.json"),
		`{"context":"","type":"other","checklist":[
			{"name":"a","description":"d","max_score":0},
			{"name":"a","description":"d","max_score":10},
			{"name":"b","description":"","max_score":10}
		]}`, 0o644)
	result := runGate(t, r)
	for _, want := range []string{
		"empty context",
		"type must be weighted_checklist",
		"3 checklist items (expected 8-13)",
		"item needs a name, a description and a positive max_score: a",
		"item needs a name, a description and a positive max_score: b",
		"duplicate item names: a",
		"max_score sums to 20 (expected 100)",
	} {
		assertFinding(t, result, want)
	}

	write(t, filepath.Join(dir, "criteria.json"), "{not json", 0o644)
	assertFinding(t, runGate(t, r), "/criteria.json: invalid JSON")
}

func TestFixtureFindings(t *testing.T) {
	r := validRepo(t)
	setup := filepath.Join(r.root, "plugins", r.plugin, "evals", "demo", "inputs", "setup.sh")
	write(t, setup, "#!/bin/bash\necho $HOME\n", 0o644)
	if err := os.Chmod(setup, 0o644); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(filepath.Dir(setup), "extra.sh"), "#!/bin/bash\n", 0o755)
	write(t, filepath.Join(filepath.Dir(setup), "unused.txt"), "x", 0o644)
	result := runGate(t, r)
	for _, want := range []string{
		"setup.sh: not executable",
		"shebang must be #!/usr/bin/env bash",
		"missing set -euo pipefail",
		"writes outside the workspace ($HOME)",
		"inputs/extra.sh: only inputs/setup.sh may be a shell entry point",
		"inputs/unused.txt: nothing in setup.sh references it (dead fixture)",
	} {
		assertFinding(t, result, want)
	}
}

func TestFixtureMissingSetup(t *testing.T) {
	r := validRepo(t)
	if err := os.Remove(filepath.Join(r.root, "plugins", r.plugin, "evals", "demo", "inputs", "setup.sh")); err != nil {
		t.Fatal(err)
	}
	assertFinding(t, runGate(t, r), "/inputs: no setup.sh")
}

func TestFixtureGitInitFindings(t *testing.T) {
	r := validRepo(t)
	setup := filepath.Join(r.root, "plugins", r.plugin, "evals", "demo", "inputs", "setup.sh")
	write(t, setup, "#!/usr/bin/env bash\nset -euo pipefail\ngit init\n", 0o755)
	result := runGate(t, r)
	assertFinding(t, result, "setup.sh:3: git init must be quiet (git init -q)")
	assertFinding(t, result, "initialises the working directory without a .git/info/exclude")

	write(t, setup, "#!/usr/bin/env bash\nset -euo pipefail\ngit init -q\n", 0o755)
	assertFinding(t, runGate(t, r), "initialises the working directory without a .git/info/exclude")

	write(t, setup, "#!/usr/bin/env bash\nset -euo pipefail\ngit init -q\nprintf '%s\\n' ignored > .git/info/exclude\n", 0o755)
	result = runGate(t, r)
	if strings.Contains(messages(result), "git init") {
		t.Fatalf("quiet init flagged:\n%s", messages(result))
	}

	write(t, setup, "#!/usr/bin/env bash\nset -euo pipefail\ncd fixture\ngit init -q\n", 0o755)
	if strings.Contains(messages(runGate(t, r)), ".git/info/exclude") {
		t.Fatal("nested repo required an exclude file")
	}
}

func TestManifestFindings(t *testing.T) {
	r := validRepo(t)
	r.writeManifests(t, `{"name":"alpha","version":"1.0.0","description":"desc"}`,
		`{"name":"wrong/alpha","description":"other","version":"2.0.0","private":false}`)
	result := runGate(t, r)
	for _, want := range []string{
		"name is 'wrong/alpha' (expected 'ws/alpha')",
		"private must be true for repository plugins",
		"version '2.0.0' does not match the Claude manifest '1.0.0'",
		"description does not match the Claude manifest",
	} {
		assertFinding(t, result, want)
	}

	if err := os.Remove(filepath.Join(r.root, "plugins", r.plugin, ".claude-plugin", "plugin.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(r.root, "plugins", r.plugin, ".tessl-plugin", "plugin.json")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(r.root, "plugins", r.plugin, ".claude-plugin", "plugin.json"), "{bad", 0o644)
	write(t, filepath.Join(r.root, "plugins", r.plugin, ".tessl-plugin", "plugin.json"), "{bad", 0o644)
	result = runGate(t, r)
	assertFinding(t, result, "invalid JSON")
}

func TestEmptyEvalsDirectory(t *testing.T) {
	r := validRepo(t)
	pluginDir := filepath.Join(r.root, "plugins", r.plugin)
	if err := os.RemoveAll(filepath.Join(pluginDir, "evals")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pluginDir, "evals"), 0o755); err != nil {
		t.Fatal(err)
	}
	assertFinding(t, runGate(t, r), "plugins/alpha//evals: empty directory")
}

func TestSkillFindings(t *testing.T) {
	r := validRepo(t)
	r.writeSkill(t, "# no frontmatter\n")
	result := runGate(t, r)
	assertFinding(t, result, "no YAML frontmatter")
}

func TestSkillFrontmatterFindings(t *testing.T) {
	r := validRepo(t)
	r.writeSkill(t, `---
name: other
name: other
description: short
allowed-tools:
  - Bash
  - Bash(:*)
  - Read
  - "Bash(PATH=*)"
---
# Alpha
`)
	result := runGate(t, r)
	for _, want := range []string{
		"2 'name:' keys in frontmatter (expected 1)",
		"frontmatter name 'other' does not match the directory 'alpha'",
		"description is 5 characters",
		"allowed-tools contains 'Bash'",
		"allowed-tools contains 'Bash(:*)'",
		"allowed-tools contains 'Bash(PATH=*)'",
	} {
		assertFinding(t, result, want)
	}
}

func TestSkillUnclosedFrontmatter(t *testing.T) {
	r := validRepo(t)
	r.writeSkill(t, "---\nname: alpha\ndescription: "+strings.Repeat("x", 70)+"\n")
	assertFinding(t, runGate(t, r), "frontmatter is not closed")
}

func TestSkillLengthAndBlockDescription(t *testing.T) {
	r := validRepo(t)
	body := strings.Repeat("line\n", 500)
	r.writeSkill(t, "---\nname: alpha\ndescription: >\n  "+strings.Repeat("long description ", 5)+"\n---\n"+body)
	assertFinding(t, runGate(t, r), "lines (Agent Skills limit is 500)")
}

func TestSkillDeadPointerAndLivePointer(t *testing.T) {
	r := validRepo(t)
	write(t, filepath.Join(r.root, "plugins", r.plugin, "README.md"), "readme\n", 0o644)
	r.writeSkill(t, `---
name: alpha
description: A sufficiently long description that carries activation triggers for the alpha skill.
---
See `+"`plugins/alpha/README.md`"+` and `+"`plugins/alpha/MISSING.md`"+`.
`)
	result := runGate(t, r)
	assertFinding(t, result, "plugins/alpha/MISSING.md")
	if strings.Contains(messages(result), "plugins/alpha/README.md") {
		t.Fatalf("live pointer flagged:\n%s", messages(result))
	}
}

func TestWorkspaceMissing(t *testing.T) {
	r := validRepo(t)
	write(t, filepath.Join(r.root, "tessl.json"), `{}`, 0o644)
	assertFinding(t, runGate(t, r), "tessl.json: no workspace name")
}

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	command.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func TestChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	r := validRepo(t)
	git(t, r.root, "init", "-q")
	git(t, r.root, "add", ".")
	git(t, r.root, "commit", "-q", "-m", "initial")
	write(t, filepath.Join(r.root, "plugins", r.plugin, "skills", "alpha", "SKILL.md"), "# changed\n", 0o644)
	git(t, r.root, "add", ".")
	git(t, r.root, "commit", "-q", "-m", "change skill")
	write(t, filepath.Join(r.root, "plugins", r.plugin, "evals", "demo", "task.md"), "# changed\n", 0o644)
	git(t, r.root, "add", ".")
	git(t, r.root, "commit", "-q", "-m", "change scenario")

	changes, err := DetectChanges(r.root, "HEAD~2")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes.Skills) != 1 || changes.Skills[0] != "plugins/alpha/skills/alpha" {
		t.Fatalf("skills = %v", changes.Skills)
	}
	if len(changes.Scenarios) != 1 || changes.Scenarios[0] != "plugins/alpha/evals/demo" {
		t.Fatalf("scenarios = %v", changes.Scenarios)
	}

	if _, err := DetectChanges(r.root, "does-not-exist"); err == nil ||
		!strings.Contains(err.Error(), "cannot diff against 'does-not-exist' (fetch it first)") {
		t.Fatalf("bad ref err = %v", err)
	}
}

func TestChangesNeedsGit(t *testing.T) {
	root := t.TempDir()
	if _, err := DetectChanges(root, "HEAD"); err == nil || !strings.Contains(err.Error(), "needs a git checkout") {
		t.Fatalf("err = %v", err)
	}
}
