package suite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/platform/errs"
	"github.com/rikdc/ai-skills/internal/provider/command"
	"github.com/rikdc/ai-skills/internal/runner/attempts"
	"github.com/rikdc/ai-skills/internal/scenario"
)

const criteriaJSON = `{
  "context": "suite fixture",
  "type": "weighted_checklist",
  "checklist": [
    {"name": "first", "description": "first criterion", "max_score": 10},
    {"name": "second", "description": "second criterion", "max_score": 10}
  ]
}`

type fixture struct {
	root        string
	agentCalls  string
	judgeCalls  string
	agentScript string
	judgeScript string
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	fx := &fixture{
		root:        root,
		agentCalls:  filepath.Join(root, "agent-calls"),
		judgeCalls:  filepath.Join(root, "judge-calls"),
		agentScript: filepath.Join(root, "agent.sh"),
		judgeScript: filepath.Join(root, "judge.sh"),
	}
	writeFile(t, filepath.Join(root, "tessl.json"), `{"name":"ws/repo"}`, 0o644)
	plugin := filepath.Join(root, "plugins", "demo")
	writeFile(t, filepath.Join(plugin, ".claude-plugin", "plugin.json"),
		`{"name":"demo","version":"1.0.0","description":"d"}`, 0o644)
	writeFile(t, filepath.Join(plugin, "skills", "good", "SKILL.md"), "---\nname: good\n---\nbody\n", 0o644)
	writeFile(t, filepath.Join(plugin, "hooks", "hook.sh"), "#!/usr/bin/env bash\necho hook\n", 0o755)

	fx.addScenario(t, "demo", "compare", `{"description":"compare","include":["./inputs"],"unit":"skills/good"}`, "comparison")
	fx.addScenario(t, "demo", "functional", `{"description":"functional","include":["./inputs"],"unit":"hooks/hook","mode":"functional"}`, "functional")

	writeFile(t, fx.agentScript, fmt.Sprintf(
		"#!/usr/bin/env bash\nset -euo pipefail\nprintf 'call\\n' >> %q\n", fx.agentCalls), 0o755)
	writeFile(t, fx.judgeScript, fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
printf 'call\n' >> %q
cat > judgement.json <<'JSON'
[{"name":"first","score":10,"max":10},{"name":"second","score":10,"max":10}]
JSON
`, fx.judgeCalls), 0o755)
	return fx
}

func (fx *fixture) addScenario(t *testing.T, plugin, name, spec, mode string) {
	t.Helper()
	dir := filepath.Join(fx.root, "plugins", plugin, "evals", name)
	writeFile(t, filepath.Join(dir, "scenario.json"), spec, 0o644)
	writeFile(t, filepath.Join(dir, "task.md"), "# Task\n\nDo the thing.\n", 0o644)
	writeFile(t, filepath.Join(dir, "criteria.json"), criteriaJSON, 0o644)
	writeFile(t, filepath.Join(dir, "inputs", "setup.sh"),
		"#!/usr/bin/env bash\nset -euo pipefail\ntouch fixture.txt\n", 0o755)
	_ = mode
}

func (fx *fixture) suiteConfig(t *testing.T, out string) Config {
	t.Helper()
	exec, err := command.New(command.Config{Command: fx.agentScript})
	if err != nil {
		t.Fatal(err)
	}
	return Config{
		Root:      fx.root,
		Selection: scenario.Selection{},
		Arm:       "both",
		Runs:      1,
		Jobs:      1,
		Out:       out,
		SkillDir:  ".claude/skills",
		Timeout:   30 * time.Second,
		AgentExec: exec,
	}
}

func (fx *fixture) judgeExecutor(t *testing.T, script string) *command.Executor {
	t.Helper()
	exec, err := command.New(command.Config{Command: script, Timeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return exec
}

func (fx *fixture) callCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(strings.Fields(string(data)))
}

func runSuite(t *testing.T, cfg Config) (*Runner, *Result, error) {
	t.Helper()
	runner, err := New(cfg)
	if err != nil {
		return nil, nil, err
	}
	result, err := runner.Run(context.Background())
	return runner, result, err
}

func readSuiteJSON(t *testing.T, out string) suiteDocument {
	t.Helper()
	var doc suiteDocument
	data, err := os.ReadFile(filepath.Join(out, "suite.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func readResults(t *testing.T, out string) []ScenarioRow {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(out, "results.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []ScenarioRow
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var row ScenarioRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
	return rows
}

func TestSuiteRunsAggregatesAndWritesArtifacts(t *testing.T) {
	fx := newFixture(t)
	out := filepath.Join(t.TempDir(), "suite")
	_, result, err := runSuite(t, fx.suiteConfig(t, out))
	if err != nil {
		t.Fatalf("suite failed: %v", err)
	}
	if result.Executed != 2 || result.Skipped != 0 {
		t.Fatalf("executed/skipped = %d/%d", result.Executed, result.Skipped)
	}
	for _, name := range []string{"manifest.json", "plan.tsv", "results.jsonl", "suite.json", "summary.md"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	rows := readResults(t, out)
	if len(rows) != 2 {
		t.Fatalf("results rows = %d", len(rows))
	}
	var compare, functional ScenarioRow
	for _, row := range rows {
		if row.Scenario == "compare" {
			compare = row
		} else {
			functional = row
		}
	}
	if compare.Mode != "comparison" || len(compare.Runs) != 2 {
		t.Fatalf("comparison row = %+v", compare)
	}
	if functional.Mode != "functional" || len(functional.Runs) != 1 || functional.Runs[0].Arm != "with" {
		t.Fatalf("functional row = %+v", functional)
	}
	if compare.PendingCriteria != 4 || functional.PendingCriteria != 2 {
		t.Fatalf("pending counts = %d/%d", compare.PendingCriteria, functional.PendingCriteria)
	}
	if compare.Denominator != 20 {
		t.Fatalf("denominator = %v", compare.Denominator)
	}
	doc := readSuiteJSON(t, out)
	if doc.Counts["planned"] != 2 || doc.Counts["executed"] != 2 || doc.Counts["agent_invocations"] != 3 {
		t.Fatalf("counts = %+v", doc.Counts)
	}
	if doc.Counts["total_attempts"] != 3 || doc.Counts["pending_runs"] != 3 {
		t.Fatalf("attempt/pending counts = %+v", doc.Counts)
	}
	if doc.Counts["usage_unknown_attempts"] != 3 {
		t.Fatalf("usage counts = %+v", doc.Counts)
	}
	summary, err := os.ReadFile(filepath.Join(out, "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(summary), "## By plugin") || !strings.Contains(string(summary), "## By scenario") {
		t.Fatalf("summary missing sections:\n%s", summary)
	}
	if !strings.Contains(string(summary), "demo") {
		t.Fatalf("summary missing plugin: %s", summary)
	}
}

func TestPlanSelectionCountsAndShards(t *testing.T) {
	fx := newFixture(t)
	plan, _, err := BuildPlan(fx.root, PlanOptions{
		Runs: 2, Arm: "both", Jobs: 1, TimeoutSeconds: 900,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Counts.Scenarios != 2 || plan.Counts.ComparisonScenarios != 1 || plan.Counts.FunctionalScenarios != 1 {
		t.Fatalf("counts = %+v", plan.Counts)
	}
	if plan.Counts.AgentInvocations != 2*2+1*2 {
		t.Fatalf("agent invocations = %d", plan.Counts.AgentInvocations)
	}
	first, _, err := BuildPlan(fx.root, PlanOptions{Selection: scenario.Selection{Shard: "1/2"}, Runs: 1, Arm: "both", Jobs: 1, TimeoutSeconds: 900})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := BuildPlan(fx.root, PlanOptions{Selection: scenario.Selection{Shard: "2/2"}, Runs: 1, Arm: "both", Jobs: 1, TimeoutSeconds: 900})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Selected)+len(second.Selected) != 2 {
		t.Fatalf("shard sizes = %d + %d", len(first.Selected), len(second.Selected))
	}
	if first.Selected[0].Pair() == second.Selected[0].Pair() {
		t.Fatalf("shards overlap")
	}
	// Plan selection must match the shared list selection exactly.
	selected, _, err := scenario.Select(fx.root, scenario.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != len(plan.Selected) {
		t.Fatalf("plan/list sizes = %d/%d", len(plan.Selected), len(selected))
	}
	for i := range selected {
		if selected[i].Pair() != plan.Selected[i].Pair() {
			t.Fatalf("plan/list pair %d = %s/%s", i, plan.Selected[i].Pair(), selected[i].Pair())
		}
	}
}

func TestResumeSkipsReusableRuns(t *testing.T) {
	fx := newFixture(t)
	out := filepath.Join(t.TempDir(), "suite")
	if _, _, err := runSuite(t, fx.suiteConfig(t, out)); err != nil {
		t.Fatal(err)
	}
	before := fx.callCount(t, fx.agentCalls)

	cfg := fx.suiteConfig(t, out)
	cfg.Resume = true
	result, err := func() (*Result, error) {
		runner, err := New(cfg)
		if err != nil {
			return nil, err
		}
		return runner.Run(context.Background())
	}()
	if err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if result.Skipped != 2 || result.Executed != 0 {
		t.Fatalf("resume executed/skipped = %d/%d", result.Executed, result.Skipped)
	}
	if got := fx.callCount(t, fx.agentCalls); got != before {
		t.Fatalf("agent calls changed on skip: %d → %d", before, got)
	}
}

func TestResumeRebuildsMissingProjections(t *testing.T) {
	fx := newFixture(t)
	out := filepath.Join(t.TempDir(), "suite")
	if _, _, err := runSuite(t, fx.suiteConfig(t, out)); err != nil {
		t.Fatal(err)
	}
	before := fx.callCount(t, fx.agentCalls)
	runDir := filepath.Join(out, "runs", "demo", "compare", "with", "run-1")
	for _, name := range []string{"result.json", "grading-request.json"} {
		if err := os.Remove(filepath.Join(runDir, name)); err != nil {
			t.Fatal(err)
		}
	}
	cfg := fx.suiteConfig(t, out)
	cfg.Resume = true
	if _, _, err := runSuite(t, cfg); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if got := fx.callCount(t, fx.agentCalls); got != before {
		t.Fatalf("projection rebuild reran the agent: %d → %d", before, got)
	}
	for _, name := range []string{"result.json", "grading-request.json"} {
		if _, err := os.Stat(filepath.Join(runDir, name)); err != nil {
			t.Fatalf("%s was not rebuilt: %v", name, err)
		}
	}
}

func TestResumeAdoptsFinishedUnacceptedAttempt(t *testing.T) {
	fx := newFixture(t)
	out := filepath.Join(t.TempDir(), "suite")
	if _, _, err := runSuite(t, fx.suiteConfig(t, out)); err != nil {
		t.Fatal(err)
	}
	before := fx.callCount(t, fx.agentCalls)
	// Simulate a crash after finished.json but before pointer replacement.
	compareDir := filepath.Join(out, "runs", "demo", "compare", "with", "run-1")
	if err := os.Remove(filepath.Join(compareDir, "current.json")); err != nil {
		t.Fatal(err)
	}

	cfg := fx.suiteConfig(t, out)
	cfg.Resume = true
	if _, _, err := runSuite(t, cfg); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if got := fx.callCount(t, fx.agentCalls); got != before {
		t.Fatalf("adoption reran the agent: %d → %d", before, got)
	}
	if _, err := os.Stat(filepath.Join(compareDir, "current.json")); err != nil {
		t.Fatalf("adoption did not restore the pointer: %v", err)
	}
}

func TestResumeRetriesFailedRuns(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, fx.agentScript, fmt.Sprintf(
		"#!/usr/bin/env bash\nprintf 'call\\n' >> %q\nexit 1\n", fx.agentCalls), 0o755)
	out := filepath.Join(t.TempDir(), "suite")
	if _, _, err := runSuite(t, fx.suiteConfig(t, out)); err == nil {
		t.Fatal("expected the failing suite to report runner failures")
	}
	before := fx.callCount(t, fx.agentCalls)
	writeFile(t, fx.agentScript, fmt.Sprintf(
		"#!/usr/bin/env bash\nprintf 'call\\n' >> %q\n", fx.agentCalls), 0o755)

	cfg := fx.suiteConfig(t, out)
	cfg.Resume = true
	cfg.ResumePolicy = PolicyRetryFailed
	if _, _, err := runSuite(t, cfg); err != nil {
		t.Fatalf("retry resume failed: %v", err)
	}
	if got := fx.callCount(t, fx.agentCalls); got != before*2 {
		t.Fatalf("failed runs were not retried: %d → %d", before, got)
	}
	for _, row := range readResults(t, out) {
		if row.Status != "ok" {
			t.Fatalf("row still failed: %+v", row)
		}
	}
	doc := readSuiteJSON(t, out)
	if doc.Counts["failed_attempts"] == 0 || doc.Counts["retried_attempts"] != 3 {
		t.Fatalf("counts = %+v", doc.Counts)
	}
}

func TestResumeKeepFailuresRetainsInterruptedAndFailed(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, fx.agentScript, fmt.Sprintf(
		"#!/usr/bin/env bash\nprintf 'call\\n' >> %q\nexit 1\n", fx.agentCalls), 0o755)
	out := filepath.Join(t.TempDir(), "suite")
	if _, _, err := runSuite(t, fx.suiteConfig(t, out)); err == nil {
		t.Fatal("expected the failing suite to report runner failures")
	}
	runDir := filepath.Join(out, "runs", "demo", "compare", "with", "run-1")
	fp, err := scenario.Fingerprint(fx.root, "demo", "compare")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := attempts.New(runDir).Allocate(model.StartedAttempt{
		Kind:        model.KindAgent,
		RunID:       "demo/compare/with/run-1",
		Fingerprint: fp,
	}); err != nil {
		t.Fatal(err)
	}
	before := fx.callCount(t, fx.agentCalls)
	writeFile(t, fx.agentScript, fmt.Sprintf(
		"#!/usr/bin/env bash\nprintf 'call\\n' >> %q\n", fx.agentCalls), 0o755)

	cfg := fx.suiteConfig(t, out)
	cfg.Resume = true
	cfg.ResumePolicy = PolicyKeepFailures
	if _, _, err := runSuite(t, cfg); err == nil || errs.Code(err) != 1 {
		t.Fatalf("keep-failures resume exit = %v, want runner failure (1)", err)
	}
	if got := fx.callCount(t, fx.agentCalls); got != before {
		t.Fatalf("keep-failures reran the agent: %d → %d", before, got)
	}
	doc := readSuiteJSON(t, out)
	if doc.Counts["kept_failures"] != 3 {
		t.Fatalf("kept_failures count = %+v", doc.Counts)
	}
	rows := readResults(t, out)
	found := false
	for _, row := range rows {
		for _, run := range row.Runs {
			if row.Scenario == "compare" && run.Arm == "with" && run.Status == string(model.StatusInterrupted) {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no interrupted row in %+v", rows)
	}
}

func TestResumeRerunAllCreatesNewAttempts(t *testing.T) {
	fx := newFixture(t)
	out := filepath.Join(t.TempDir(), "suite")
	if _, _, err := runSuite(t, fx.suiteConfig(t, out)); err != nil {
		t.Fatal(err)
	}
	before := fx.callCount(t, fx.agentCalls)
	cfg := fx.suiteConfig(t, out)
	cfg.Resume = true
	cfg.ResumePolicy = PolicyRerunAll
	if _, _, err := runSuite(t, cfg); err != nil {
		t.Fatalf("rerun-all failed: %v", err)
	}
	if got := fx.callCount(t, fx.agentCalls); got != before*2 {
		t.Fatalf("rerun-all calls = %d, want %d", got, before*2)
	}
	doc := readSuiteJSON(t, out)
	if doc.Counts["retried_attempts"] != 3 {
		t.Fatalf("retried attempts = %+v", doc.Counts)
	}
}

func TestFingerprintInvalidationReruns(t *testing.T) {
	fx := newFixture(t)
	out := filepath.Join(t.TempDir(), "suite")
	if _, _, err := runSuite(t, fx.suiteConfig(t, out)); err != nil {
		t.Fatal(err)
	}
	before := fx.callCount(t, fx.agentCalls)
	task := filepath.Join(fx.root, "plugins", "demo", "evals", "compare", "task.md")
	writeFile(t, task, "# Task\n\nChanged.\n", 0o644)

	cfg := fx.suiteConfig(t, out)
	cfg.Resume = true
	if _, _, err := runSuite(t, cfg); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if got := fx.callCount(t, fx.agentCalls); got <= before {
		t.Fatalf("fingerprint change did not rerun: %d → %d", before, got)
	}
}

func TestChangedSelectsTouchedScenario(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	fx := newFixture(t)
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "."},
		{"-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-qm", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", fx.root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	writeFile(t, filepath.Join(fx.root, "plugins", "demo", "evals", "compare", "task.md"),
		"# Task\n\nEdited.\n", 0o644)
	plan, _, err := BuildPlan(fx.root, PlanOptions{
		Selection: scenario.Selection{Changed: "HEAD"},
		Runs:      1, Arm: "both", Jobs: 1, TimeoutSeconds: 900,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Selected) != 1 || plan.Selected[0].Pair() != "demo/compare" {
		t.Fatalf("changed selection = %+v", plan.Selected)
	}
}

func TestArmWithoutAppliesOnlyToComparison(t *testing.T) {
	fx := newFixture(t)
	out := filepath.Join(t.TempDir(), "suite")
	cfg := fx.suiteConfig(t, out)
	cfg.Arm = "without"
	if _, _, err := runSuite(t, cfg); err != nil {
		t.Fatalf("suite failed: %v", err)
	}
	if got := fx.callCount(t, fx.agentCalls); got != 2 {
		t.Fatalf("agent calls = %d, want 2", got)
	}
	for _, row := range readResults(t, out) {
		if row.Scenario == "compare" && (len(row.Runs) != 1 || row.Runs[0].Arm != "without") {
			t.Fatalf("comparison runs = %+v", row.Runs)
		}
		if row.Scenario == "functional" && (len(row.Runs) != 1 || row.Runs[0].Arm != "with") {
			t.Fatalf("functional runs = %+v", row.Runs)
		}
	}
}

func TestFailedAgentIsRecordedWithoutAbortingSuite(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, fx.agentScript, fmt.Sprintf(
		"#!/usr/bin/env bash\nprintf 'call\\n' >> %q\nexit 1\n", fx.agentCalls), 0o755)
	out := filepath.Join(t.TempDir(), "suite")
	cfg := fx.suiteConfig(t, out)
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	result, runErr := runner.Run(context.Background())
	if runErr == nil || errs.Code(runErr) != 1 {
		t.Fatalf("suite exit = %v, want 1", runErr)
	}
	if result.Executed != 2 || result.RunnerFailures != 2 {
		t.Fatalf("result = %+v", result)
	}
	rows := readResults(t, out)
	if len(rows) != 2 {
		t.Fatalf("rows = %d", len(rows))
	}
	for _, row := range rows {
		if row.Status != "runner-failed" {
			t.Fatalf("row status = %s", row.Status)
		}
		for _, run := range row.Runs {
			if run.Status != string(model.StatusAgentFailed) {
				t.Fatalf("run status = %s", run.Status)
			}
		}
	}
	doc := readSuiteJSON(t, out)
	if doc.Counts["failed_attempts"] != 3 {
		t.Fatalf("failed attempts = %+v", doc.Counts)
	}
}

func TestStrictRejectsPendingCriteria(t *testing.T) {
	fx := newFixture(t)
	cfg := fx.suiteConfig(t, filepath.Join(t.TempDir(), "suite"))
	cfg.Strict = true
	_, _, err := runSuite(t, cfg)
	if err == nil || errs.Code(err) != 1 {
		t.Fatalf("strict exit = %v, want 1", err)
	}
	if !strings.Contains(err.Error(), "pending") {
		t.Fatalf("strict message = %v", err)
	}
}

func TestBadOutAndJobsValidation(t *testing.T) {
	fx := newFixture(t)
	out := t.TempDir()
	writeFile(t, filepath.Join(out, "keep.txt"), "x", 0o644)
	cfg := fx.suiteConfig(t, out)
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, runErr := runner.Run(context.Background()); runErr == nil || errs.Code(runErr) != 2 {
		t.Fatalf("bad out exit = %v, want 2", runErr)
	}

	cfg = fx.suiteConfig(t, filepath.Join(t.TempDir(), "suite"))
	cfg.Jobs = 0
	if _, err := New(cfg); err == nil || errs.Code(err) != 2 {
		t.Fatalf("zero jobs = %v, want 2", err)
	}
	cfg = fx.suiteConfig(t, filepath.Join(t.TempDir(), "suite"))
	cfg.Runs = 0
	if _, err := New(cfg); err == nil || errs.Code(err) != 2 {
		t.Fatalf("zero runs = %v, want 2", err)
	}
	cfg = fx.suiteConfig(t, filepath.Join(t.TempDir(), "suite"))
	cfg.Jobs = -1
	if _, err := New(cfg); err == nil || errs.Code(err) != 2 {
		t.Fatalf("negative jobs = %v", err)
	}
}

func TestLockRejectsSecondWriter(t *testing.T) {
	fx := newFixture(t)
	out := filepath.Join(t.TempDir(), "suite")
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	release, err := lockFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	cfg := fx.suiteConfig(t, out)
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, runErr := runner.Run(context.Background())
	if runErr == nil || errs.Code(runErr) != 2 {
		t.Fatalf("lock exit = %v, want 2", runErr)
	}
}

func TestJobsRunScenariosConcurrently(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, fx.agentScript, fmt.Sprintf(
		"#!/usr/bin/env bash\nprintf 'call\\n' >> %q\nsleep 0.2\n", fx.agentCalls), 0o755)
	cfg := fx.suiteConfig(t, filepath.Join(t.TempDir(), "suite"))
	cfg.Jobs = 2
	start := time.Now()
	_, result, err := runSuite(t, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Executed != 2 {
		t.Fatalf("executed = %d", result.Executed)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("jobs did not run concurrently: %s", elapsed)
	}
	for _, name := range []string{"compare", "functional"} {
		if _, err := os.Stat(filepath.Join(cfg.Out, "logs", "demo", name+".status")); err != nil {
			t.Fatalf("missing status for %s: %v", name, err)
		}
	}
}

func TestRegradeOneJudgeCallPerRunAndPreservesExpenditure(t *testing.T) {
	fx := newFixture(t)
	out := filepath.Join(t.TempDir(), "suite")
	cfg := fx.suiteConfig(t, out)
	cfg.JudgeCmd = fx.judgeScript
	cfg.JudgeExec = fx.judgeExecutor(t, fx.judgeScript)
	if _, _, err := runSuite(t, cfg); err != nil {
		t.Fatalf("initial suite failed: %v", err)
	}
	if got := fx.callCount(t, fx.judgeCalls); got != 3 {
		t.Fatalf("initial judge calls = %d", got)
	}
	second := filepath.Join(fx.root, "judge-second.sh")
	writeFile(t, second, fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
printf 'call\n' >> %q
cat > judgement.json <<'JSON'
[{"name":"first","score":5,"max":10},{"name":"second","score":5,"max":10}]
JSON
`, fx.judgeCalls), 0o755)

	cfg = fx.suiteConfig(t, out)
	cfg.Resume = true
	cfg.Regrade = true
	cfg.JudgeCmd = second
	cfg.JudgeExec = fx.judgeExecutor(t, second)
	cfg.ResumePolicy = PolicyRetryFailed
	if _, _, err := runSuite(t, cfg); err != nil {
		t.Fatalf("regrade failed: %v", err)
	}
	if got := fx.callCount(t, fx.judgeCalls); got != 6 {
		t.Fatalf("judge calls after regrade = %d, want 6", got)
	}
	doc := readSuiteJSON(t, out)
	var compare ScenarioRow
	for _, row := range readResults(t, out) {
		if row.Scenario == "compare" {
			compare = row
		}
	}
	if compare.Expenditure.JudgeAttempts != 4 {
		t.Fatalf("judge attempts = %d", compare.Expenditure.JudgeAttempts)
	}
	for _, run := range compare.Runs {
		if run.Judged == nil || run.Judged.Score != 10 {
			t.Fatalf("regraded judgement = %+v", run.Judged)
		}
	}
	if doc.Counts["judge_attempts"] != 6 {
		t.Fatalf("suite judge attempts = %+v", doc.Counts)
	}
}

func TestJudgeProvenanceMismatchWithoutRegradeExitsTwo(t *testing.T) {
	fx := newFixture(t)
	out := filepath.Join(t.TempDir(), "suite")
	cfg := fx.suiteConfig(t, out)
	cfg.JudgeCmd = fx.judgeScript
	cfg.JudgeExec = fx.judgeExecutor(t, fx.judgeScript)
	if _, _, err := runSuite(t, cfg); err != nil {
		t.Fatalf("initial suite failed: %v", err)
	}
	second := filepath.Join(fx.root, "judge-second.sh")
	writeFile(t, second, "#!/usr/bin/env bash\ncat > judgement.json <<'JSON'\n[]\nJSON\n", 0o755)
	cfg = fx.suiteConfig(t, out)
	cfg.Resume = true
	cfg.JudgeCmd = second
	cfg.JudgeExec = fx.judgeExecutor(t, second)
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, runErr := runner.Run(context.Background())
	if runErr == nil || errs.Code(runErr) != 2 {
		t.Fatalf("provenance exit = %v, want 2", runErr)
	}
}

func TestSignalCancelsSuiteWithoutOrphans(t *testing.T) {
	fx := newFixture(t)
	started := filepath.Join(fx.root, "started")
	writeFile(t, fx.agentScript, fmt.Sprintf(
		"#!/usr/bin/env bash\nprintf 'call\\n' >> %q\ntouch %q\nsleep 30\n", fx.agentCalls, started), 0o755)
	cfg := fx.suiteConfig(t, filepath.Join(t.TempDir(), "suite"))
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := runner.Run(ctx)
		done <- err
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("agent did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled suite returned nil error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cancelled suite did not return; orphan worker likely")
	}
}

func TestResumeRequiresVersion3Manifest(t *testing.T) {
	fx := newFixture(t)
	out := filepath.Join(t.TempDir(), "suite")
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(out, "manifest.json"), `{"version":2,"scenarios":[]}`, 0o644)
	cfg := fx.suiteConfig(t, out)
	cfg.Resume = true
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, runErr := runner.Run(context.Background())
	if runErr == nil || errs.Code(runErr) != 2 {
		t.Fatalf("v2 resume exit = %v, want 2", runErr)
	}
}

func TestResumeJudgeChangeWithoutRegradeIsRefused(t *testing.T) {
	fx := newFixture(t)
	out := filepath.Join(t.TempDir(), "suite")
	cfg := fx.suiteConfig(t, out)
	cfg.JudgeCmd = fx.judgeScript
	cfg.JudgeExec = fx.judgeExecutor(t, fx.judgeScript)
	if _, _, err := runSuite(t, cfg); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(fx.root, "judge-second.sh")
	writeFile(t, second, "#!/usr/bin/env bash\nexit 0\n", 0o755)
	cfg = fx.suiteConfig(t, out)
	cfg.Resume = true
	cfg.JudgeCmd = second
	cfg.JudgeExec = fx.judgeExecutor(t, second)
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, runErr := runner.Run(context.Background())
	if runErr == nil || errs.Code(runErr) != 2 {
		t.Fatalf("judge change exit = %v, want 2", runErr)
	}
}

func TestAggregationConcurrentReadsAreConsistent(t *testing.T) {
	fx := newFixture(t)
	out := filepath.Join(t.TempDir(), "suite")
	if _, _, err := runSuite(t, fx.suiteConfig(t, out)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows := readResults(t, out)
			if len(rows) != 2 {
				t.Errorf("rows = %d", len(rows))
			}
		}()
	}
	wg.Wait()
}
