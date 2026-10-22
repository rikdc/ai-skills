package reportio_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/reportio"
)

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeText(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
}

var fixtureRubric = model.Rubric{
	Context: "x",
	Type:    "weighted_checklist",
	Checklist: []model.Criterion{
		{Name: "A", Description: "d", MaxScore: 60},
		{Name: "B", Description: "d", MaxScore: 40},
	},
}

func newLegacySuite(t *testing.T, dir string, runs int) {
	t.Helper()
	writeJSON(t, filepath.Join(dir, "manifest.json"), map[string]any{
		"version":    1,
		"created_at": "2026-01-02T03:04:05Z",
		"config": map[string]any{
			"agent_cmd": "claude -p --model fixture-model",
			"judge_cmd": "",
			"arm":       "both",
			"runs":      runs,
			"units":     []string{"skills/fixture"},
			"git_sha":   "deadbee",
		},
		"scenarios": []map[string]any{{
			"plugin": "demo", "scenario": "case", "units": []string{"skills/fixture"},
			"fingerprint": "fp", "mode": "comparison",
		}},
	})
	writeJSON(t, filepath.Join(dir, "suite.json"), map[string]any{
		"counts": map[string]any{"agent_invocations": 2 * runs, "duration_seconds": 21.5},
	})
	writeText(t, filepath.Join(dir, "results.jsonl"), "")
	writeJSON(t, filepath.Join(dir, "criteria", "demo", "case.json"), fixtureRubric)
}

func addLegacyRun(t *testing.T, dir, arm string, run int, items []model.JudgedItem, judged []model.JudgedItem) string {
	t.Helper()
	runDir := filepath.Join(dir, "runs", "demo", "case", arm, "run-"+itoa(run))
	writeJSON(t, filepath.Join(runDir, "mechanical.json"), items)
	pending := []model.Criterion{}
	for _, criterion := range fixtureRubric.Checklist {
		covered := false
		for _, item := range items {
			if item.Name == criterion.Name {
				covered = true
			}
		}
		if !covered {
			pending = append(pending, criterion)
		}
	}
	writeJSON(t, filepath.Join(runDir, "grading-request.json"), model.GradingRequest{
		Criteria: fixtureRubric, JudgementPending: pending,
	})
	if judged != nil {
		writeJSON(t, filepath.Join(runDir, "judgement.json"), judged)
	}
	writeJSON(t, filepath.Join(runDir, "result.json"), map[string]any{
		"plugin": "demo", "scenario": "case", "arm": arm, "run": run,
		"agent_exit": 0, "timed_out": false, "duration_seconds": 10.0,
	})
	writeJSON(t, filepath.Join(runDir, "judge-result.json"), map[string]any{
		"exit_code": 0, "duration_seconds": 4.0,
	})
	return runDir
}

func itoa(v int) string { return strconv.Itoa(v) }

func TestLoadLegacySuite(t *testing.T) {
	dir := t.TempDir()
	newLegacySuite(t, dir, 1)
	mechanical := []model.JudgedItem{{Name: "A", Score: 50, Max: 60}}
	runDir := addLegacyRun(t, dir, "with", 1, mechanical, nil)
	pending := []model.JudgedItem{{Name: "B", Score: 34, Max: 40}}
	writeJSON(t, filepath.Join(runDir, "judgement.json"), pending)
	writeJSON(t, filepath.Join(runDir, "usage.json"), map[string]any{
		"known": true, "source": "opencode-json", "cost_usd": 0.015, "total_tokens": 1000,
	})
	writeJSON(t, filepath.Join(runDir, "judge-usage.json"), map[string]any{
		"known": true, "source": "claude-json", "cost_usd": 0.003, "total_tokens": 100,
	})
	addLegacyRun(t, dir, "without", 1, mechanical, nil)

	in, err := reportio.Load(dir, reportio.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if in.ManifestVersion != 1 || len(in.Scenarios) != 1 {
		t.Fatalf("unexpected input: %+v", in)
	}
	sc := in.Scenarios[0]
	if len(sc.Runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(sc.Runs))
	}
	with := sc.Runs[0]
	if with.Status != string(model.StatusOK) {
		t.Errorf("status = %q", with.Status)
	}
	if len(with.JudgedItems) != 1 || with.JudgedItems[0].Name != "B" {
		t.Errorf("judged = %+v", with.JudgedItems)
	}
	if with.AgentUsage.State != model.UsageKnown || with.AgentUsage.CostUSD == nil || *with.AgentUsage.CostUSD != 0.015 {
		t.Errorf("agent usage = %+v", with.AgentUsage)
	}
	if with.JudgeUsage.State != model.UsageKnown || with.CombinedUsage.State != model.UsageKnown {
		t.Errorf("judge/combined usage = %+v / %+v", with.JudgeUsage, with.CombinedUsage)
	}
	if len(in.Attempts) != 4 {
		t.Fatalf("attempts = %d, want 4", len(in.Attempts))
	}
}

func TestInvalidJudgementIgnored(t *testing.T) {
	dir := t.TempDir()
	newLegacySuite(t, dir, 1)
	mechanical := []model.JudgedItem{{Name: "A", Score: 50, Max: 60}}
	runDir := addLegacyRun(t, dir, "with", 1, mechanical, nil)
	// Wrong max for B: the judgement must be ignored and B stays pending.
	writeJSON(t, filepath.Join(runDir, "judgement.json"), []model.JudgedItem{{Name: "B", Score: 10, Max: 10}})
	in, err := reportio.Load(dir, reportio.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if in.Scenarios[0].Runs[0].JudgedItems != nil {
		t.Fatalf("invalid judgement accepted: %+v", in.Scenarios[0].Runs[0].JudgedItems)
	}
}

func TestMissingRunIsExplicit(t *testing.T) {
	dir := t.TempDir()
	newLegacySuite(t, dir, 2)
	mechanical := []model.JudgedItem{{Name: "A", Score: 60, Max: 60}, {Name: "B", Score: 40, Max: 40}}
	addLegacyRun(t, dir, "with", 1, mechanical, nil)
	addLegacyRun(t, dir, "without", 1, mechanical, nil)
	in, err := reportio.Load(dir, reportio.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var missing int
	for _, run := range in.Scenarios[0].Runs {
		if run.Status == string(model.StatusMissing) {
			missing++
		}
	}
	if missing != 2 {
		t.Fatalf("missing rows = %d, want 2", missing)
	}
}

func TestMissingCriteriaIsAnError(t *testing.T) {
	dir := t.TempDir()
	newLegacySuite(t, dir, 1)
	if err := os.Remove(filepath.Join(dir, "criteria", "demo", "case.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := reportio.Load(dir, reportio.Options{}); err == nil {
		t.Fatal("expected a missing-rubric error")
	}
}

func TestLoadV3SuiteWithAttemptsAndBinding(t *testing.T) {
	dir := newV3Suite(t)
	in, err := reportio.Load(dir, reportio.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Scenarios) != 1 || len(in.Scenarios[0].Runs) != 2 {
		t.Fatalf("unexpected scenarios: %+v", in.Scenarios)
	}
	with := in.Scenarios[0].Runs[0]
	if with.Status != string(model.StatusOK) || with.AgentExit == nil || *with.AgentExit != 0 {
		t.Errorf("with run = %+v", with)
	}
	if len(with.JudgedItems) != 1 || with.JudgedItems[0].Name != "B" {
		t.Errorf("judged = %+v", with.JudgedItems)
	}
	if with.DurationSeconds == nil || *with.DurationSeconds != 21.5 {
		t.Errorf("duration = %v", with.DurationSeconds)
	}
	if with.AgentUsage.State != model.UsageKnown || with.JudgeUsage.State != model.UsageKnown {
		t.Errorf("usage = %+v / %+v", with.AgentUsage, with.JudgeUsage)
	}
	without := in.Scenarios[0].Runs[1]
	if without.JudgedItems != nil {
		t.Errorf("without has no judge attempt and must stay pending: %+v", without.JudgedItems)
	}
	if len(in.Attempts) != 4 {
		t.Fatalf("attempts = %d, want 4 (agent, judge, interrupted, agent)", len(in.Attempts))
	}
	var interrupted int
	for _, attempt := range in.Attempts {
		if attempt.Usage.State == model.UsageUnknown {
			interrupted++
		}
	}
	if interrupted != 1 {
		t.Fatalf("unknown attempts = %d, want 1", interrupted)
	}
}

func TestV3TamperedJudgeBindingIsRejected(t *testing.T) {
	dir := newV3Suite(t)
	finished := filepath.Join(dir, "runs", "demo", "v3case", "with", "run-1",
		"attempts", "judge-000002", "finished.json")
	var doc map[string]any
	data, err := os.ReadFile(finished)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	doc["graded_evidence_sha256"] = "tampered"
	writeJSON(t, finished, doc)

	in, err := reportio.Load(dir, reportio.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if in.Scenarios[0].Runs[0].JudgedItems != nil {
		t.Fatal("tampered judgement binding was accepted")
	}
}

func TestV3RunWithoutCurrentIsIncomplete(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, "manifest.json"), map[string]any{
		"version": 3, "created_at": "2026-01-01T00:00:00Z",
		"repo":   map[string]any{"git_sha": "abc"},
		"config": map[string]any{"runs": 1, "arm": "both"},
		"scenarios": []map[string]any{{
			"plugin": "demo", "scenario": "case", "mode": "comparison",
		}},
	})
	writeJSON(t, filepath.Join(dir, "suite.json"), map[string]any{"counts": map[string]any{}})
	writeText(t, filepath.Join(dir, "results.jsonl"), "")
	writeJSON(t, filepath.Join(dir, "criteria", "demo", "case.json"), fixtureRubric)
	runDir := filepath.Join(dir, "runs", "demo", "case", "with", "run-1")
	writeJSON(t, filepath.Join(runDir, "attempts", "agent-000001", "started.json"), map[string]any{
		"attempt_id": "agent-000001", "kind": "agent", "started_at": "2026-01-01T00:00:00Z",
	})
	in, err := reportio.Load(dir, reportio.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if in.Scenarios[0].Runs[0].Status != string(model.StatusIncomplete) {
		t.Fatalf("status = %q, want incomplete", in.Scenarios[0].Runs[0].Status)
	}
	if len(in.Attempts) != 1 || in.Attempts[0].Usage.State != model.UsageUnknown {
		t.Fatalf("attempts = %+v", in.Attempts)
	}
}

func TestLegacyJudgeUsageStates(t *testing.T) {
	dir := t.TempDir()
	newLegacySuite(t, dir, 1)
	mechanical := []model.JudgedItem{{Name: "A", Score: 60, Max: 60}, {Name: "B", Score: 40, Max: 40}}
	addLegacyRun(t, dir, "with", 1, mechanical, nil)
	noJudge := filepath.Join(dir, "runs", "demo", "case", "without", "run-1")
	addLegacyRun(t, dir, "without", 1, mechanical, nil)
	// A run directory with an agent log but no judge invocation: judge usage
	// is not applicable, not unknown.
	if err := os.Remove(filepath.Join(noJudge, "judge-result.json")); err != nil {
		t.Fatal(err)
	}
	in, err := reportio.Load(dir, reportio.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var without model.RunInput
	for _, run := range in.Scenarios[0].Runs {
		if run.Arm == "without" {
			without = run
		}
	}
	if without.JudgeUsage.State != model.UsageNotApplicable {
		t.Fatalf("judge usage = %q, want not_applicable", without.JudgeUsage.State)
	}
	if without.AgentUsage.State != model.UsageUnknown {
		t.Fatalf("agent usage = %q, want unknown", without.AgentUsage.State)
	}
}

func newV3Suite(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, "manifest.json"), map[string]any{
		"version": 3, "created_at": "2026-02-02T00:00:00Z",
		"repo":   map[string]any{"git_sha": "abc123", "git_dirty": false},
		"config": map[string]any{"backend": "command", "model": "m1", "agent_cmd": "cmd", "arm": "both", "runs": 1, "units": []string{"skills/demo"}},
		"scenarios": []map[string]any{{
			"plugin": "demo", "scenario": "v3case", "units": []string{"skills/demo"},
			"mode": "comparison", "execution": "agent",
		}},
	})
	writeJSON(t, filepath.Join(dir, "suite.json"), map[string]any{
		"counts": map[string]any{"agent_invocations": 2, "duration_seconds": 42.5},
	})
	writeText(t, filepath.Join(dir, "results.jsonl"), "")
	writeJSON(t, filepath.Join(dir, "criteria", "demo", "v3case.json"), fixtureRubric)

	for _, arm := range []string{"with", "without"} {
		runDir := filepath.Join(dir, "runs", "demo", "v3case", arm, "run-1")
		writeJSON(t, filepath.Join(runDir, "current.json"), model.CurrentPointer{
			SchemaVersion:             3,
			AgentAttempt:              "agent-000001",
			AgentEvidenceSHA256:       "ev1",
			AgentGradingRequestSHA256: "req1",
			JudgeAttempt:              "judge-000002",
			JudgeIdentitySHA256:       "jid",
			GradedAgentAttempt:        "agent-000001",
			GradedEvidenceSHA256:      "ev1",
			GradingRequestSHA256:      "req1",
		})
		writeJSON(t, filepath.Join(runDir, "attempts", "agent-000001", "started.json"), model.StartedAttempt{
			SchemaVersion: 3, AttemptID: "agent-000001", Kind: model.KindAgent,
			StartedAt: "2026-02-02T00:00:00Z", Executor: "command",
		})
		writeJSON(t, filepath.Join(runDir, "attempts", "agent-000001", "finished.json"), model.FinishedAttempt{
			SchemaVersion: 3, AttemptID: "agent-000001", Kind: model.KindAgent,
			FinishedAt: "2026-02-02T00:00:20Z", Status: model.StatusOK,
			StopReason: model.StopCompleted, ChecksStatus: model.ChecksOK,
			Usage: model.Usage{State: model.UsageKnown, Known: true, Source: "x"},
		})
		writeJSON(t, filepath.Join(runDir, "attempts", "agent-000001", "usage.json"), model.Usage{
			State: model.UsageKnown, Known: true, Source: "x",
			CostUSD: ptrFloat(0.01), TotalTokens: ptrInt(100), InputTokens: ptrInt(80), OutputTokens: ptrInt(20),
		})
		writeJSON(t, filepath.Join(runDir, "attempts", "agent-000001", "mechanical.json"),
			[]model.JudgedItem{{Name: "A", Score: 60, Max: 60}})
		zeroExit := 0
		writeJSON(t, filepath.Join(runDir, "result.json"), model.RunResult{
			SchemaVersion: 3, Plugin: "demo", Scenario: "v3case", Arm: arm, Run: 1,
			AgentAttempt: "agent-000001", JudgeAttempt: "judge-000002", Complete: true,
			Status: model.StatusOK, AgentExit: &zeroExit, DurationSeconds: ptrFloat(21.5),
			Mechanical: model.Mechanical{Score: 60, Max: 60, Items: []model.JudgedItem{{Name: "A", Score: 60, Max: 60}}},
			Evidence:   filepath.ToSlash(filepath.Join("runs", "demo", "v3case", arm, "run-1")),
		})
		writeJSON(t, filepath.Join(runDir, "grading-request.json"), model.GradingRequest{
			SchemaVersion: 3, Plugin: "demo", Scenario: "v3case", Arm: arm, Run: 1,
			AgentAttempt: "agent-000001", EvidenceSHA256: "ev1", Criteria: fixtureRubric,
			CoveredByChecks:  []string{"A"},
			JudgementPending: []model.Criterion{{Name: "B", Description: "d", MaxScore: 40}},
		})
	}

	judgeDir := filepath.Join(dir, "runs", "demo", "v3case", "with", "run-1", "attempts", "judge-000002")
	writeJSON(t, filepath.Join(judgeDir, "started.json"), model.StartedAttempt{
		SchemaVersion: 3, AttemptID: "judge-000002", Kind: model.KindJudge,
		StartedAt: "2026-02-02T00:00:20Z",
	})
	writeJSON(t, filepath.Join(judgeDir, "finished.json"), model.FinishedAttempt{
		SchemaVersion: 3, AttemptID: "judge-000002", Kind: model.KindJudge,
		FinishedAt: "2026-02-02T00:00:26Z", Status: model.StatusOK,
		Usage:                model.Usage{State: model.UsageKnown, Known: true, Source: "judge"},
		GradedAgentAttempt:   "agent-000001",
		GradedEvidenceSHA256: "ev1",
		GradingRequestSHA256: "req1",
		JudgeIdentitySHA256:  "jid",
	})
	writeJSON(t, filepath.Join(judgeDir, "judge-usage.json"), model.Usage{
		State: model.UsageKnown, Known: true, Source: "judge",
		CostUSD: ptrFloat(0.005), TotalTokens: ptrInt(50),
	})
	writeJSON(t, filepath.Join(judgeDir, "judgement.json"), []model.JudgedItem{{Name: "B", Score: 30, Max: 40}})
	writeJSON(t, filepath.Join(judgeDir, "judge-result.json"), map[string]any{"duration_seconds": 6.0})

	interrupted := filepath.Join(dir, "runs", "demo", "v3case", "with", "run-1", "attempts", "agent-000003")
	writeJSON(t, filepath.Join(interrupted, "started.json"), map[string]any{
		"attempt_id": "agent-000003", "kind": "agent", "started_at": "2026-02-02T00:01:00Z",
	})
	return dir
}

func ptrFloat(v float64) *float64 { return &v }
func ptrInt(v int64) *int64       { return &v }
