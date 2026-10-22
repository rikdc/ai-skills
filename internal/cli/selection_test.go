package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rikdc/ai-skills/internal/platform/errs"
)

func cliWrite(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func cliCriteria() string {
	items := make([]map[string]any, 0, 8)
	for i := range 8 {
		items = append(items, map[string]any{
			"name":        "item-" + string(rune('a'+i)),
			"description": "description",
			"max_score":   12.5,
		})
	}
	data, _ := json.Marshal(map[string]any{
		"context":   "context",
		"type":      "weighted_checklist",
		"checklist": items,
	})
	return string(data)
}

func cliRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if _, err := exec.LookPath("git"); err == nil {
		command := exec.Command("git", "-C", root, "init", "-q")
		if err := command.Run(); err != nil {
			t.Fatal(err)
		}
	}
	cliWrite(t, filepath.Join(root, "tessl.json"), `{"name":"ws/repo"}`, 0o644)
	for _, plugin := range []string{"alpha", "beta"} {
		cliWrite(t, filepath.Join(root, "plugins", plugin, ".claude-plugin", "plugin.json"),
			`{"name":"`+plugin+`","description":"desc","version":"1.0.0"}`, 0o644)
		cliWrite(t, filepath.Join(root, "plugins", plugin, ".tessl-plugin", "plugin.json"),
			`{"name":"ws/`+plugin+`","description":"desc","version":"1.0.0","private":true}`, 0o644)
	}
	add := func(plugin, name, scenario string) {
		dir := filepath.Join(root, "plugins", plugin, "evals", name)
		cliWrite(t, filepath.Join(dir, "scenario.json"), scenario, 0o644)
		cliWrite(t, filepath.Join(dir, "task.md"), "# Task\n", 0o644)
		cliWrite(t, filepath.Join(dir, "criteria.json"), cliCriteria(), 0o644)
		cliWrite(t, filepath.Join(dir, "inputs", "setup.sh"),
			"#!/usr/bin/env bash\nset -euo pipefail\necho fixture.txt\n", 0o755)
		cliWrite(t, filepath.Join(dir, "inputs", "fixture.txt"), "x\n", 0o644)
	}
	add("alpha", "compare", `{"description":"d","include":["./inputs"],"unit":"skills/alpha"}`)
	add("beta", "functional", `{"description":"d","include":["./inputs"],"unit":"hooks/h","mode":"functional"}`)
	add("beta", "direct", `{"description":"d","include":["./inputs"],"unit":"hooks/h","mode":"functional","execution":{"backend":"hook"}}`)

	cliWrite(t, filepath.Join(root, "plugins", "alpha", "skills", "alpha", "SKILL.md"), `---
name: alpha
description: A sufficiently long description that carries activation triggers for the alpha skill.
allowed-tools: Read
---
# Alpha
`, 0o644)
	cliWrite(t, filepath.Join(root, "plugins", "beta", "hooks", "h.sh"), "#!/usr/bin/env bash\n", 0o755)
	return root
}

func executeWithRoot(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func TestListTextAndJSON(t *testing.T) {
	root := cliRoot(t)
	stdout, stderr, err := executeWithRoot(t, "--root", root, "list")
	if err != nil {
		t.Fatalf("list: %v (stderr %s)", err, stderr)
	}
	want := "alpha/compare\nbeta/direct\nbeta/functional\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}

	stdout, _, err = executeWithRoot(t, "--root", root, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []listEntry
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, err)
	}
	if len(entries) != 3 || entries[0].Plugin != "alpha" || entries[0].Scenario != "compare" {
		t.Fatalf("entries = %+v", entries)
	}
	if len(entries[0].Units) != 1 || entries[0].Units[0] != "skills/alpha" {
		t.Fatalf("units = %v", entries[0].Units)
	}
}

func TestListSelectionFlags(t *testing.T) {
	root := cliRoot(t)
	stdout, _, err := executeWithRoot(t, "--root", root, "list", "--plugin", "beta", "--unit", "hooks/h")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "beta/direct\nbeta/functional\n" {
		t.Fatalf("stdout = %q", stdout)
	}

	stdout, _, err = executeWithRoot(t, "--root", root, "list", "--scenario", "^alpha/")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "alpha/compare\n" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestListShardPartition(t *testing.T) {
	root := cliRoot(t)
	var joined []string
	for _, shard := range []string{"1/3", "2/3", "3/3"} {
		stdout, _, err := executeWithRoot(t, "--root", root, "list", "--shard", shard)
		if err != nil {
			t.Fatal(err)
		}
		joined = append(joined, strings.Split(strings.TrimSpace(stdout), "\n")...)
	}
	if strings.Join(joined, ",") != "alpha/compare,beta/direct,beta/functional" {
		t.Fatalf("joined = %v", joined)
	}
}

func TestListBadInputsExitTwo(t *testing.T) {
	root := cliRoot(t)
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"list", "--shard", "4/3"}, "between 1 and 3"},
		{[]string{"list", "--unit", "skills/nope"}, "unknown --unit"},
		{[]string{"list", "--scenario", "("}, "invalid --scenario regex"},
		{[]string{"list", "--changed", "does-not-exist"}, "cannot diff against"},
	} {
		_, _, err := executeWithRoot(t, append([]string{"--root", root}, test.args...)...)
		if err == nil || errs.Code(err) != 2 || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("%v: err = %v (code %d)", test.args, err, errs.Code(err))
		}
	}
}

func TestPlanText(t *testing.T) {
	root := cliRoot(t)
	stdout, _, err := executeWithRoot(t, "--root", root, "plan", "--runs", "3", "--jobs", "2")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Eval suite plan",
		"alpha/compare",
		"installs: all units (skills/alpha)",
		"beta/direct",
		"unsupported: direct-hook execution is milestone C",
		"Scenarios: 2",
		"Arms: both (2)  ·  runs per arm: 3",
		"Functional scenarios: 1 (with arm only, no delta)",
		"Agent invocations: 9",
		"Unsupported (milestone C): 1",
		"Jobs: 2  ·  per-run timeout: 900s",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("plan missing %q:\n%s", want, stdout)
		}
	}
}

func TestPlanJSON(t *testing.T) {
	root := cliRoot(t)
	stdout, _, err := executeWithRoot(t, "--root", root, "plan", "--format", "json", "--runs", "2", "--judge-cmd", "true")
	if err != nil {
		t.Fatal(err)
	}
	var document planDocument
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, err)
	}
	if document.Counts.AgentInvocations != 6 || document.Counts.JudgeInvocations != 6 {
		t.Fatalf("counts = %+v", document.Counts)
	}
	if len(document.Scenarios) != 2 || len(document.Unsupported) != 1 {
		t.Fatalf("document = %+v", document)
	}
}

func TestPlanBadFormatAndArm(t *testing.T) {
	root := cliRoot(t)
	_, _, err := executeWithRoot(t, "--root", root, "plan", "--format", "yaml")
	if errs.Code(err) != 2 {
		t.Fatalf("format err = %v", err)
	}
	_, _, err = executeWithRoot(t, "--root", root, "plan", "--arm", "sideways")
	if errs.Code(err) != 2 || !strings.Contains(err.Error(), "--arm must be both, with or without") {
		t.Fatalf("arm err = %v", err)
	}
}

func TestCheckCleanAndJSON(t *testing.T) {
	root := cliRoot(t)
	stdout, stderr, err := executeWithRoot(t, "--root", root, "check")
	if err != nil {
		t.Fatalf("check: %v (stderr %s stdout %s)", err, stderr, stdout)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q", stdout)
	}
	stdout, _, err = executeWithRoot(t, "--root", root, "check", "--json")
	if err != nil {
		t.Fatalf("check --json: %v", err)
	}
	if stdout != "" {
		t.Fatalf("JSON stdout = %q", stdout)
	}
}

func TestCheckFindingExitOne(t *testing.T) {
	root := cliRoot(t)
	setup := filepath.Join(root, "plugins", "alpha", "evals", "compare", "inputs", "setup.sh")
	cliWrite(t, setup, "#!/bin/bash\n", 0o755)
	stdout, _, err := executeWithRoot(t, "--root", root, "check")
	if err == nil || errs.Code(err) != 1 {
		t.Fatalf("err = %v (code %d)", err, errs.Code(err))
	}
	if !strings.Contains(stdout, "shebang must be #!/usr/bin/env bash") {
		t.Fatalf("stdout = %q", stdout)
	}

	stdout, _, err = executeWithRoot(t, "--root", root, "check", "--json")
	if err == nil || errs.Code(err) != 1 {
		t.Fatalf("err = %v", err)
	}
	var finding map[string]any
	line := strings.Split(strings.TrimSpace(stdout), "\n")[0]
	if err := json.Unmarshal([]byte(line), &finding); err != nil {
		t.Fatalf("invalid finding JSON %q: %v", line, err)
	}
	if finding["message"] == "" || finding["class"] == "" || finding["rule"] == "" {
		t.Fatalf("finding = %+v", finding)
	}
}
