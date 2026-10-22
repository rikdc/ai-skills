package report_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/provider/command"
	"github.com/rikdc/ai-skills/internal/report"
	"github.com/rikdc/ai-skills/internal/reportio"
	"github.com/rikdc/ai-skills/internal/scenario"
	"github.com/rikdc/ai-skills/internal/suite"
)

const v3Criteria = `{
  "context": "v3 report integration",
  "type": "weighted_checklist",
  "checklist": [
    {"name": "first", "description": "first criterion", "max_score": 10},
    {"name": "second", "description": "second criterion", "max_score": 10}
  ]
}`

// TestGoProducedV3SuiteReportsFromAttempts runs the suite end to end with a
// stub agent and judge, then reports from current.json and the immutable
// attempt tree.
func TestGoProducedV3SuiteReportsFromAttempts(t *testing.T) {
	root, out := runStubV3Suite(t)

	// The suite must have produced the v3 storage contract.
	for _, arm := range []string{"with", "without"} {
		runDir := filepath.Join(out, "runs", "demo", "v3", arm, "run-1")
		for _, name := range []string{"current.json", "result.json", "grading-request.json"} {
			if _, err := os.Stat(filepath.Join(runDir, name)); err != nil {
				t.Fatalf("missing %s for %s: %v", name, arm, err)
			}
		}
		if _, err := os.Stat(filepath.Join(runDir, "attempts", "agent-000001", "finished.json")); err != nil {
			t.Fatalf("missing accepted agent attempt for %s: %v", arm, err)
		}
	}

	repoRoot := root
	loaded, err := reportio.Load(out, reportio.Options{RepoRoot: repoRoot})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ManifestVersion != model.ManifestVersion {
		t.Fatalf("manifest version = %d", loaded.ManifestVersion)
	}
	built := report.Build(loaded)
	if built.Verdict.Status != "indicative" {
		t.Fatalf("verdict = %q, want indicative", built.Verdict.Status)
	}
	sc := built.Scenarios[0]
	if len(sc.Runs) != 2 || sc.Status != "ok" {
		t.Fatalf("scenario = %+v", sc)
	}
	for _, run := range sc.Runs {
		if run.Status != string(model.StatusOK) || !run.Complete {
			t.Fatalf("run = %+v", run)
		}
		if run.Judged == nil || len(run.Judged.Items) != 2 {
			t.Fatalf("judged items = %+v", run.Judged)
		}
		if run.Score == nil || *run.Score != 20 {
			t.Fatalf("score = %v", run.Score)
		}
		if run.PendingPoints != 0 || run.PendingCount != 0 {
			t.Fatalf("pending = %v/%d", run.PendingPoints, run.PendingCount)
		}
	}
	if sc.Arms.With.PercentMean == nil || *sc.Arms.With.PercentMean != 100 {
		t.Fatalf("with mean = %v", sc.Arms.With.PercentMean)
	}
	if built.Totals.PendingPoints != 0 || built.Totals.Failures != 0 {
		t.Fatalf("totals = %+v", built.Totals)
	}

	e := built.Expenditure
	if e.TotalAttempts != 4 || e.AgentAttempts != 2 || e.JudgeAttempts != 2 {
		t.Fatalf("expenditure = %+v", e)
	}
	if e.Complete {
		t.Fatal("stub invocations have unknown usage; expenditure must not claim completeness")
	}
	if e.UnknownAttempts == 0 {
		t.Fatalf("unknown attempts = %d", e.UnknownAttempts)
	}
	if built.Cost.With.Combined.Cost.Total != nil {
		t.Fatalf("unknown usage leaked into accepted cost: %v", built.Cost.With.Combined.Cost.Total)
	}

	// current.json wins over stale projections: remove both derived files and
	// reload. The immutable attempts still describe the accepted result.
	for _, arm := range []string{"with", "without"} {
		runDir := filepath.Join(out, "runs", "demo", "v3", arm, "run-1")
		for _, name := range []string{"result.json", "grading-request.json"} {
			if err := os.Remove(filepath.Join(runDir, name)); err != nil {
				t.Fatal(err)
			}
		}
	}
	reloaded, err := reportio.Load(out, reportio.Options{RepoRoot: repoRoot})
	if err != nil {
		t.Fatal(err)
	}
	rebuilt := report.Build(reloaded)
	if rebuilt.Scenarios[0].Status != "ok" || rebuilt.Verdict.Status != "indicative" {
		t.Fatalf("rebuilt report = %+v", rebuilt.Scenarios[0])
	}
	for _, run := range rebuilt.Scenarios[0].Runs {
		if run.AgentExit == nil || *run.AgentExit != 0 {
			t.Fatalf("agent exit lost without projection: %v", run.AgentExit)
		}
		if run.Judged == nil || len(run.Judged.Items) != 2 {
			t.Fatalf("judgement lost without projection: %+v", run.Judged)
		}
	}
}

// runStubV3Suite writes a minimal repository, runs one comparison scenario
// with a stub command agent and judge, and returns the repo root and suite
// directory.
func runStubV3Suite(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "tessl.json"), `{"name":"ws/repo"}`)
	plugin := filepath.Join(root, "plugins", "demo")
	writeTestFile(t, filepath.Join(plugin, ".claude-plugin", "plugin.json"),
		`{"name":"demo","version":"1.0.0","description":"d"}`)
	writeTestFile(t, filepath.Join(plugin, "skills", "good", "SKILL.md"),
		"---\nname: good\n---\nbody\n")
	scenarioDir := filepath.Join(plugin, "evals", "v3")
	writeTestFile(t, filepath.Join(scenarioDir, "scenario.json"),
		`{"description":"v3","include":["./inputs"],"unit":"skills/good"}`)
	writeTestFile(t, filepath.Join(scenarioDir, "task.md"), "# Task\n\nDo the thing.\n")
	writeTestFile(t, filepath.Join(scenarioDir, "criteria.json"), v3Criteria)
	writeTestFile(t, filepath.Join(scenarioDir, "inputs", "setup.sh"),
		"#!/usr/bin/env bash\nset -euo pipefail\ntouch fixture.txt\n")

	agentCalls := filepath.Join(root, "agent-calls")
	agentScript := filepath.Join(root, "agent.sh")
	writeTestFile(t, agentScript, fmt.Sprintf(
		"#!/usr/bin/env bash\nset -euo pipefail\nprintf 'call\\n' >> %q\n", agentCalls))
	judgeScript := filepath.Join(root, "judge.sh")
	writeTestFile(t, judgeScript, `#!/usr/bin/env bash
set -euo pipefail
cat > judgement.json <<'JSON'
[{"name":"first","score":10,"max":10},{"name":"second","score":10,"max":10}]
JSON
`)

	agentExec, err := command.New(command.Config{Command: agentScript})
	if err != nil {
		t.Fatal(err)
	}
	judgeExec, err := command.New(command.Config{Command: judgeScript, Timeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "suite")
	runner, err := suite.New(suite.Config{
		Root:      root,
		Selection: scenario.Selection{},
		Arm:       "both",
		Runs:      1,
		Jobs:      1,
		Out:       out,
		SkillDir:  ".claude/skills",
		Timeout:   30 * time.Second,
		AgentExec: agentExec,
		JudgeExec: judgeExec,
		JudgeCmd:  judgeScript,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Run(context.Background()); err != nil {
		t.Fatalf("suite run failed: %v", err)
	}
	return root, out
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	mode := os.FileMode(0o644)
	if filepath.Ext(path) == ".sh" {
		mode = 0o755
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}
