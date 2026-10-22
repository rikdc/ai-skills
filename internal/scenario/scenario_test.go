package scenario

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type fixture struct {
	root string
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func validCriteria() string {
	items := make([]map[string]any, 0, 8)
	for i := range 8 {
		items = append(items, map[string]any{
			"name":        "item-" + string(rune('a'+i)),
			"description": "criterion description",
			"max_score":   12.5,
		})
	}
	data, _ := json.Marshal(map[string]any{
		"context":   "fixture context",
		"type":      "weighted_checklist",
		"checklist": items,
	})
	return string(data)
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	f := fixture{root: root}
	writeFile(t, filepath.Join(root, "tessl.json"), `{"name":"ws/repo"}`, 0o644)
	for _, plugin := range []string{"alpha", "beta"} {
		writeFile(t, filepath.Join(root, "plugins", plugin, ".claude-plugin", "plugin.json"),
			`{"name":"`+plugin+`","version":"1.0.0","description":"d"}`, 0o644)
	}
	writeFile(t, filepath.Join(root, "plugins", "alpha", "skills", "a", "SKILL.md"), "# a\n", 0o644)
	writeFile(t, filepath.Join(root, "plugins", "alpha", "commands", "c.md"), "command\n", 0o644)
	writeFile(t, filepath.Join(root, "plugins", "alpha", "hooks", "h.sh"), "hook\n", 0o755)
	writeFile(t, filepath.Join(root, "plugins", "beta", "hooks", "h.sh"), "hook\n", 0o755)

	f.addScenario(t, "alpha", "compare", `{"description":"compare","include":["./inputs"],"unit":"skills/a"}`, "comparison")
	f.addScenario(t, "beta", "functional", `{"description":"functional","include":["./inputs"],"unit":"hooks/h","mode":"functional"}`, "functional")
	f.addScenario(t, "beta", "direct", `{"description":"direct","include":["./inputs"],"unit":"hooks/h","mode":"functional","execution":{"backend":"hook","cases":"hook-cases.json"}}`, "functional")

	// Incomplete: no task/rubric/setup.
	if err := os.MkdirAll(filepath.Join(root, "plugins", "alpha", "evals", "incomplete"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "plugins", "alpha", "evals", "incomplete", "scenario.json"),
		`{"description":"incomplete","include":[],"unit":"skills/a"}`, 0o644)
	return f
}

func (f fixture) addScenario(t *testing.T, plugin, name, spec, mode string) {
	t.Helper()
	dir := filepath.Join(f.root, "plugins", plugin, "evals", name)
	writeFile(t, filepath.Join(dir, "scenario.json"), spec, 0o644)
	writeFile(t, filepath.Join(dir, "task.md"), "# Task\n\nDo the thing.\n", 0o644)
	writeFile(t, filepath.Join(dir, "criteria.json"), validCriteria(), 0o644)
	writeFile(t, filepath.Join(dir, "inputs", "setup.sh"),
		"#!/usr/bin/env bash\nset -euo pipefail\ntouch fixture.txt\n", 0o755)
	writeFile(t, filepath.Join(dir, "inputs", "fixture.txt"), "fixture\n", 0o644)
	_ = mode
}

func TestDiscoverSorted(t *testing.T) {
	f := newFixture(t)
	entries, err := Discover(f.root)
	if err != nil {
		t.Fatal(err)
	}
	var pairs []string
	for _, entry := range entries {
		pairs = append(pairs, entry.Pair())
	}
	want := []string{
		"alpha/compare",
		"alpha/incomplete",
		"beta/direct",
		"beta/functional",
	}
	if strings.Join(pairs, ",") != strings.Join(want, ",") {
		t.Fatalf("pairs = %v, want %v", pairs, want)
	}
}

func TestSelectSkipsIncompleteWithWarning(t *testing.T) {
	f := newFixture(t)
	selected, warnings, err := Select(f.root, Selection{})
	if err != nil {
		t.Fatal(err)
	}
	var pairs []string
	for _, item := range selected {
		pairs = append(pairs, item.Pair())
	}
	want := []string{"alpha/compare", "beta/direct", "beta/functional"}
	if strings.Join(pairs, ",") != strings.Join(want, ",") {
		t.Fatalf("pairs = %v, want %v", pairs, want)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "skipping incomplete scenario: alpha/incomplete") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestSelectUnitAndPluginFilters(t *testing.T) {
	f := newFixture(t)
	selected, _, err := Select(f.root, Selection{Plugins: []string{"beta"}, Units: []string{"hooks/h"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 {
		t.Fatalf("selected = %d", len(selected))
	}
	for _, item := range selected {
		if item.Scenario.Plugin != "beta" {
			t.Fatalf("plugin = %s", item.Scenario.Plugin)
		}
		if strings.Join(item.Units, ",") != "hooks/h" {
			t.Fatalf("units = %v", item.Units)
		}
	}
}

func TestSelectScenarioRegex(t *testing.T) {
	f := newFixture(t)
	selected, _, err := Select(f.root, Selection{Pattern: `^(alpha|beta)/compare|beta/direct$`})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 {
		t.Fatalf("selected = %+v", selected)
	}
	selected, _, err = Select(f.root, Selection{Pattern: `(alpha/compare|beta/direct)`})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 {
		t.Fatalf("selected = %d", len(selected))
	}
}

func TestShardPartition(t *testing.T) {
	f := newFixture(t)
	all, _, err := Select(f.root, Selection{})
	if err != nil {
		t.Fatal(err)
	}
	var joined []string
	for index := 1; index <= 3; index++ {
		selected, _, err := Select(f.root, Selection{Shard: shardValue(index, 3)})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range selected {
			joined = append(joined, item.Pair())
		}
	}
	if len(joined) != len(all) {
		t.Fatalf("joined %d, want %d (%v)", len(joined), len(all), joined)
	}
	seen := map[string]bool{}
	for _, pair := range joined {
		if seen[pair] {
			t.Fatalf("duplicate pair %s", pair)
		}
		seen[pair] = true
	}
}

func shardValue(index, count int) string {
	return string(rune('0'+index)) + "/" + string(rune('0'+count))
}

func TestSelectionErrors(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		name string
		opts Selection
		want string
	}{
		{"bad shard shape", Selection{Shard: "x"}, "--shard must look like 1/3"},
		{"shard zero", Selection{Shard: "0/3"}, "--shard index must be between 1 and 3"},
		{"shard over", Selection{Shard: "4/3"}, "--shard index must be between 1 and 3"},
		{"zero denominator", Selection{Shard: "1/0"}, "--shard denominator must be at least 1"},
		{"bad unit syntax", Selection{Units: []string{"skills"}}, "invalid --unit 'skills'"},
		{"unknown unit", Selection{Units: []string{"skills/nope"}}, "unknown --unit 'skills/nope'"},
		{"bad regex", Selection{Pattern: "("}, "invalid --scenario regex"},
		{"no match", Selection{Pattern: "^nothing$"}, "no scenarios selected"},
		{"no unit match", Selection{Units: []string{"skills/a"}, Plugins: []string{"beta"}}, "unknown --unit 'skills/a'"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := Select(f.root, test.opts)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err = %v, want %q", err, test.want)
			}
			if !IsUsageError(err) {
				t.Fatalf("not a usage error: %v", err)
			}
		})
	}
}

func TestPlanCountsAndUnsupported(t *testing.T) {
	f := newFixture(t)
	plan, _, err := BuildPlan(f.root, PlanOptions{
		Runs:           3,
		Arm:            "both",
		Jobs:           2,
		TimeoutSeconds: 900,
	})
	if err != nil {
		t.Fatal(err)
	}
	// alpha/compare comparison 2*3, beta/functional functional 1*3; beta/direct
	// is unsupported and excluded.
	if plan.Counts.Scenarios != 2 || plan.Counts.ComparisonScenarios != 1 || plan.Counts.FunctionalScenarios != 1 {
		t.Fatalf("counts = %+v", plan.Counts)
	}
	if plan.Counts.AgentInvocations != 9 {
		t.Fatalf("agent invocations = %d, want 9", plan.Counts.AgentInvocations)
	}
	if plan.Counts.Unsupported != 1 || len(plan.Unsupported) != 1 || plan.Unsupported[0].Pair() != "beta/direct" {
		t.Fatalf("unsupported = %+v", plan.Unsupported)
	}
}

func TestPlanArmCounts(t *testing.T) {
	f := newFixture(t)
	plan, _, err := BuildPlan(f.root, PlanOptions{Runs: 1, Arm: "with", Jobs: 1, TimeoutSeconds: 30})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Counts.AgentInvocations != 2 {
		t.Fatalf("agent invocations = %d, want 2", plan.Counts.AgentInvocations)
	}
	if _, _, err := BuildPlan(f.root, PlanOptions{Runs: 1, Arm: "sideways", Jobs: 1, TimeoutSeconds: 30}); err == nil ||
		!strings.Contains(err.Error(), "--arm must be both, with or without") {
		t.Fatalf("arm error = %v", err)
	}
	if _, _, err := BuildPlan(f.root, PlanOptions{Runs: 0, Arm: "both", Jobs: 1, TimeoutSeconds: 30}); err == nil ||
		!strings.Contains(err.Error(), "--runs must be a positive integer") {
		t.Fatalf("runs error = %v", err)
	}
}

func TestFingerprintDeterministicAndInvalidated(t *testing.T) {
	f := newFixture(t)
	first, err := Fingerprint(f.root, "alpha", "compare")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Fingerprint(f.root, "alpha", "compare")
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first) != 64 {
		t.Fatalf("fingerprints = %q, %q", first, second)
	}

	// Ignored artifacts do not change the fingerprint.
	dir := filepath.Join(f.root, "plugins", "alpha", "evals", "compare")
	writeFile(t, filepath.Join(dir, "result.json"), "{}", 0o644)
	writeFile(t, filepath.Join(dir, "run.log"), "log", 0o644)
	unchanged, _ := Fingerprint(f.root, "alpha", "compare")
	if unchanged != first {
		t.Fatalf("ignored artifacts changed the fingerprint")
	}

	mutations := []struct {
		name   string
		mutate func()
	}{
		{"scenario file", func() {
			writeFile(t, filepath.Join(dir, "task.md"), "changed task\n", 0o644)
		}},
		{"skill", func() {
			writeFile(t, filepath.Join(f.root, "plugins", "alpha", "skills", "a", "SKILL.md"), "changed\n", 0o644)
		}},
		{"manifest", func() {
			writeFile(t, filepath.Join(f.root, "plugins", "alpha", ".claude-plugin", "plugin.json"), `{"name":"changed"}`, 0o644)
		}},
		{"security patterns", func() {
			writeFile(t, filepath.Join(f.root, "plugins", "alpha", "security-patterns.conf"), "pattern\n", 0o644)
		}},
		{"executable bit", func() {
			if err := os.Chmod(filepath.Join(dir, "inputs", "setup.sh"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			before, err := Fingerprint(f.root, "alpha", "compare")
			if err != nil {
				t.Fatal(err)
			}
			mutation.mutate()
			after, err := Fingerprint(f.root, "alpha", "compare")
			if err != nil {
				t.Fatal(err)
			}
			if before == after {
				t.Fatalf("%s did not invalidate the fingerprint", mutation.name)
			}
		})
	}
}

func TestFingerprintSymlink(t *testing.T) {
	f := newFixture(t)
	dir := filepath.Join(f.root, "plugins", "alpha", "evals", "compare")
	if err := os.Symlink("task.md", filepath.Join(dir, "linked-task")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	first, err := Fingerprint(f.root, "alpha", "compare")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "linked-task")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("inputs/setup.sh", filepath.Join(dir, "linked-task")); err != nil {
		t.Fatal(err)
	}
	second, err := Fingerprint(f.root, "alpha", "compare")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("symlink target change did not invalidate the fingerprint")
	}
}

func TestLoadValidation(t *testing.T) {
	f := newFixture(t)
	loaded, err := Load(f.root, "alpha", "compare")
	if err != nil {
		t.Fatalf("valid scenario rejected: %v", err)
	}
	if loaded.Mode() != "comparison" || loaded.Execution() != "agent" {
		t.Fatalf("mode/execution = %s/%s", loaded.Mode(), loaded.Execution())
	}

	dir := filepath.Join(f.root, "plugins", "alpha", "evals", "compare")
	writeFile(t, filepath.Join(dir, "scenario.json"),
		`{"description":"x","include":["./missing"],"unit":"skills/a","units":["skills/a"],"mode":"bogus"}`, 0o644)
	_, err = Load(f.root, "alpha", "compare")
	if err == nil {
		t.Fatal("invalid scenario accepted")
	}
	for _, want := range []string{"include path does not exist", "use either 'unit' or 'units', not both", "invalid mode"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
}

func TestLoadFunctionalRequiresHook(t *testing.T) {
	f := newFixture(t)
	dir := filepath.Join(f.root, "plugins", "alpha", "evals", "compare")
	writeFile(t, filepath.Join(dir, "scenario.json"),
		`{"description":"x","include":["./inputs"],"unit":"skills/a","mode":"functional"}`, 0o644)
	_, err := Load(f.root, "alpha", "compare")
	if err == nil || !strings.Contains(err.Error(), "requires a hook unit") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadHookBackendRequiresFunctional(t *testing.T) {
	f := newFixture(t)
	dir := filepath.Join(f.root, "plugins", "alpha", "evals", "compare")
	writeFile(t, filepath.Join(dir, "scenario.json"),
		`{"description":"x","include":["./inputs"],"unit":"skills/a","execution":{"backend":"hook"}}`, 0o644)
	_, err := Load(f.root, "alpha", "compare")
	if err == nil || !strings.Contains(err.Error(), `requires functional mode`) {
		t.Fatalf("err = %v", err)
	}
}

func runGitTest(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	command.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func sortedPairs(selected []Selected) []string {
	pairs := make([]string, 0, len(selected))
	for _, item := range selected {
		pairs = append(pairs, item.Pair())
	}
	sort.Strings(pairs)
	return pairs
}

func TestChangedSelection(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	f := newFixture(t)
	runGitTest(t, f.root, "init", "-q")
	runGitTest(t, f.root, "add", ".")
	runGitTest(t, f.root, "commit", "-q", "-m", "initial")

	// Working-tree change to a skill selects the whole plugin.
	writeFile(t, filepath.Join(f.root, "plugins", "alpha", "skills", "a", "SKILL.md"), "changed\n", 0o644)
	selected, _, err := Select(f.root, Selection{Changed: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	pairs := sortedPairs(selected)
	if strings.Join(pairs, ",") != "alpha/compare" {
		t.Fatalf("pairs = %v", pairs)
	}

	// An untracked scenario is selected individually.
	writeFile(t, filepath.Join(f.root, "plugins", "beta", "evals", "new", "scenario.json"),
		`{"description":"new","include":[],"unit":"hooks/h"}`, 0o644)
	writeFile(t, filepath.Join(f.root, "plugins", "beta", "evals", "new", "task.md"), "task\n", 0o644)
	writeFile(t, filepath.Join(f.root, "plugins", "beta", "evals", "new", "criteria.json"), validCriteria(), 0o644)
	writeFile(t, filepath.Join(f.root, "plugins", "beta", "evals", "new", "inputs", "setup.sh"),
		"#!/usr/bin/env bash\nset -euo pipefail\n", 0o755)
	selected, _, err = Select(f.root, Selection{Changed: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	pairs = sortedPairs(selected)
	if strings.Join(pairs, ",") != "alpha/compare,beta/new" {
		t.Fatalf("pairs = %v", pairs)
	}

	if _, _, err := Select(f.root, Selection{Changed: "does-not-exist"}); err == nil ||
		!strings.Contains(err.Error(), "cannot diff against 'does-not-exist' (fetch it first)") {
		t.Fatalf("bad ref error = %v", err)
	}
}

func TestChangedNeedsGit(t *testing.T) {
	root := t.TempDir()
	if _, _, err := Select(root, Selection{Changed: "HEAD"}); err == nil || !strings.Contains(err.Error(), "needs a git checkout") {
		t.Fatalf("err = %v", err)
	}
}
