package reportio_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/reportio"
)

func TestV3StaleProjectionsDoNotOverrideAttempts(t *testing.T) {
	dir := newV3Suite(t)
	runDir := filepath.Join(dir, "runs", "demo", "v3case", "with", "run-1")

	// A stale, unbound projection claims a different exit, score and
	// evidence. It must not override current.json or the attempt files.
	writeJSON(t, filepath.Join(runDir, "result.json"), map[string]any{
		"schema_version": 3, "plugin": "demo", "scenario": "v3case",
		"arm": "with", "run": 1, "agent_attempt": "agent-000001",
		"current_sha256": "stale", "status": "agent-failed",
		"agent_exit": 99, "duration_seconds": 1.5,
		"mechanical": map[string]any{"items": []map[string]any{
			{"name": "A", "score": 0, "max": 60},
		}},
		"evidence": "runs/demo/v3case/with/run-1/attempts/stale",
	})
	// A stale grading request would make B look covered by checks; the
	// loader must rebuild pending criteria from the accepted attempt.
	writeJSON(t, filepath.Join(runDir, "grading-request.json"), model.GradingRequest{
		SchemaVersion: 3, Plugin: "demo", Scenario: "v3case", Arm: "with", Run: 1,
		AgentAttempt: "agent-000001", EvidenceSHA256: "ev1",
		Criteria:         fixtureRubric,
		JudgementPending: []model.Criterion{},
	})

	in, err := reportio.Load(dir, reportio.Options{})
	if err != nil {
		t.Fatal(err)
	}
	with := in.Scenarios[0].Runs[0]
	if with.Status != string(model.StatusOK) || !with.Complete {
		t.Fatalf("status = %q complete=%v, want ok", with.Status, with.Complete)
	}
	if with.AgentExit != nil && *with.AgentExit == 99 {
		t.Fatal("stale projection supplied the agent exit")
	}
	if with.DurationSeconds == nil || *with.DurationSeconds != 21.5 {
		t.Fatalf("duration = %v, want the attempt value", with.DurationSeconds)
	}
	if len(with.MechanicalItems) != 1 || with.MechanicalItems[0].Score != 60 {
		t.Fatalf("mechanical = %+v, want the attempt value", with.MechanicalItems)
	}
	if len(with.JudgedItems) != 1 || with.JudgedItems[0].Name != "B" {
		t.Fatalf("judged = %+v, want B from the immutable judge attempt", with.JudgedItems)
	}
	if with.Evidence == "runs/demo/v3case/with/run-1/attempts/stale" {
		t.Fatal("stale projection supplied the evidence link")
	}
}

func TestV3TamperedJudgementDigestIsRejected(t *testing.T) {
	dir := newV3Suite(t)
	judgement := filepath.Join(dir, "runs", "demo", "v3case", "with", "run-1",
		"attempts", "judge-000002", "judgement.json")
	writeJSON(t, judgement, []model.JudgedItem{{Name: "B", Score: 0, Max: 40}})

	in, err := reportio.Load(dir, reportio.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if in.Scenarios[0].Runs[0].JudgedItems != nil {
		t.Fatal("judgement with a mismatched digest was accepted")
	}
}

func TestV3InterruptedCurrentStatus(t *testing.T) {
	dir := newV3Suite(t)
	runDir := filepath.Join(dir, "runs", "demo", "v3case", "with", "run-1")
	// Point current at the interrupted attempt that already exists.
	writeJSON(t, filepath.Join(runDir, "current.json"), model.CurrentPointer{
		SchemaVersion:             3,
		AgentAttempt:              "agent-000003",
		AgentEvidenceSHA256:       "ev3",
		AgentGradingRequestSHA256: "req3",
	})

	in, err := reportio.Load(dir, reportio.Options{})
	if err != nil {
		t.Fatal(err)
	}
	with := in.Scenarios[0].Runs[0]
	if with.Status != string(model.StatusInterrupted) || with.Complete {
		t.Fatalf("status = %q complete=%v", with.Status, with.Complete)
	}
	if with.AgentUsage.State != model.UsageUnknown || with.CombinedUsage.State != model.UsageUnknown {
		t.Fatalf("usage = %+v / %+v", with.AgentUsage, with.CombinedUsage)
	}
	var unknown int
	for _, attempt := range in.Attempts {
		if attempt.Usage.State == model.UsageUnknown {
			unknown++
		}
	}
	if unknown == 0 {
		t.Fatal("interrupted attempt lost its unknown expenditure")
	}
}

func TestV3ExpenditureIncludesSupersededAttempts(t *testing.T) {
	dir := newV3Suite(t)
	runDir := filepath.Join(dir, "runs", "demo", "v3case", "with", "run-1")
	extra := filepath.Join(runDir, "attempts", "agent-000004")
	writeJSON(t, filepath.Join(extra, "started.json"), model.StartedAttempt{
		SchemaVersion: 3, AttemptID: "agent-000004", Kind: model.KindAgent,
		StartedAt: "2026-02-02T00:02:00Z",
	})
	writeJSON(t, filepath.Join(extra, "finished.json"), model.FinishedAttempt{
		SchemaVersion: 3, AttemptID: "agent-000004", Kind: model.KindAgent,
		FinishedAt: "2026-02-02T00:03:00Z", Status: model.StatusOK,
		Usage: model.Usage{State: model.UsageKnown, Known: true, Source: "x"},
	})
	writeJSON(t, filepath.Join(extra, "usage.json"), model.Usage{
		State: model.UsageKnown, Known: true, Source: "x",
		CostUSD: ptrFloat(1.25), TotalTokens: ptrInt(999),
	})

	in, err := reportio.Load(dir, reportio.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var acceptedCost, expenditureCost float64
	with := in.Scenarios[0].Runs[0]
	if with.AgentUsage.CostUSD == nil {
		t.Fatal("accepted agent usage missing")
	}
	acceptedCost = *with.AgentUsage.CostUSD
	for _, attempt := range in.Attempts {
		if attempt.Usage.CostUSD != nil {
			expenditureCost += *attempt.Usage.CostUSD
		}
	}
	if acceptedCost != 0.01 {
		t.Fatalf("accepted cost = %v, want 0.01", acceptedCost)
	}
	if expenditureCost != 0.01+0.005+1.25+0.01 {
		t.Fatalf("expenditure cost = %v; superseded attempts must be retained", expenditureCost)
	}
}

func TestV3RunWithoutCurrentKeepsAttemptExpenditure(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(dir, "results.jsonl"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(dir, "criteria", "demo", "case.json"), fixtureRubric)
	runDir := filepath.Join(dir, "runs", "demo", "case", "with", "run-1")
	attempt := filepath.Join(runDir, "attempts", "agent-000001")
	writeJSON(t, filepath.Join(attempt, "started.json"), map[string]any{
		"attempt_id": "agent-000001", "kind": "agent", "started_at": "2026-01-01T00:00:00Z",
	})
	writeJSON(t, filepath.Join(attempt, "finished.json"), map[string]any{
		"attempt_id": "agent-000001", "kind": "agent",
		"finished_at": "2026-01-01T00:01:00Z", "status": "ok",
	})
	writeJSON(t, filepath.Join(attempt, "usage.json"), map[string]any{
		"known": true, "cost_usd": 0.4, "total_tokens": 10,
	})

	in, err := reportio.Load(dir, reportio.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if in.Scenarios[0].Runs[0].Status != string(model.StatusIncomplete) {
		t.Fatalf("status = %q", in.Scenarios[0].Runs[0].Status)
	}
	if len(in.Attempts) != 1 || in.Attempts[0].Usage.CostUSD == nil ||
		*in.Attempts[0].Usage.CostUSD != 0.4 {
		t.Fatalf("attempts = %+v", in.Attempts)
	}
}

func TestV3CurrentPointsAtMissingAttempt(t *testing.T) {
	dir := newV3Suite(t)
	runDir := filepath.Join(dir, "runs", "demo", "v3case", "with", "run-1")
	writeJSON(t, filepath.Join(runDir, "current.json"), model.CurrentPointer{
		SchemaVersion:             3,
		AgentAttempt:              "agent-000099",
		AgentEvidenceSHA256:       "ev",
		AgentGradingRequestSHA256: "req",
	})
	in, err := reportio.Load(dir, reportio.Options{})
	if err != nil {
		t.Fatal(err)
	}
	run := in.Scenarios[0].Runs[0]
	if run.Status != string(model.StatusIncomplete) {
		t.Fatalf("status = %q, want incomplete", run.Status)
	}
	if run.AgentAttempt != "agent-000099" {
		t.Fatalf("accepted pointer was not preserved: %q", run.AgentAttempt)
	}
}

func skillRunInput(t *testing.T, dir, arm string) model.RunInput {
	t.Helper()
	in, err := reportio.Load(dir, reportio.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range in.Scenarios[0].Runs {
		if run.Arm == arm {
			return run
		}
	}
	t.Fatalf("no %s run", arm)
	return model.RunInput{}
}

func agentLogPath(dir, arm string) string {
	return filepath.Join(dir, "runs", "demo", "v3case", arm, "run-1", "attempts", "agent-000001", "agent.log")
}

func TestV3SkillUseIsDetectedFromTheAcceptedAgentLog(t *testing.T) {
	invoked := `{"type":"tool_use","part":{"tool":"skill","state":{"input":{"name":"demo"}}}}` + "\n"
	idle := `{"type":"text","part":{"text":"hi"}}` + "\n"
	cases := []struct {
		name string
		log  *string
		want string
	}{
		{"invoked", &invoked, model.SkillInvoked},
		{"not invoked", &idle, model.SkillNotInvoked},
		{"log missing", nil, model.SkillUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := newV3Suite(t)
			if tc.log != nil {
				writeText(t, agentLogPath(dir, "with"), *tc.log)
				writeText(t, agentLogPath(dir, "without"), invoked)
			}
			with := skillRunInput(t, dir, "with")
			if with.SkillUse == nil || with.SkillUse.State != tc.want {
				t.Fatalf("with skill use = %+v, want %s", with.SkillUse, tc.want)
			}
			if without := skillRunInput(t, dir, "without"); without.SkillUse != nil {
				t.Fatalf("the without arm has no skill: %+v", without.SkillUse)
			}
		})
	}
}

func TestV3RecordedSkillUseWinsWhenLogIsGone(t *testing.T) {
	dir := newV3Suite(t)
	runDir := filepath.Join(dir, "runs", "demo", "v3case", "with", "run-1")
	zero := 0
	writeJSON(t, filepath.Join(runDir, "result.json"), model.RunResult{
		SchemaVersion: 3, Plugin: "demo", Scenario: "v3case", Arm: "with", Run: 1,
		AgentAttempt: "agent-000001", Status: model.StatusOK, AgentExit: &zero,
		SkillUse: &model.SkillUse{State: model.SkillNotInvoked, Skills: map[string]string{"demo": model.SkillNotInvoked}},
	})
	with := skillRunInput(t, dir, "with")
	if with.SkillUse == nil || with.SkillUse.State != model.SkillNotInvoked {
		t.Fatalf("skill use = %+v, want the recorded not-invoked", with.SkillUse)
	}
}

func TestV3AttemptsCarryFailureStatusAndError(t *testing.T) {
	dir := newV3Suite(t)
	runDir := filepath.Join(dir, "runs", "demo", "v3case", "with", "run-1")
	failed := filepath.Join(runDir, "attempts", "judge-000005")
	writeJSON(t, filepath.Join(failed, "started.json"), model.StartedAttempt{
		SchemaVersion: 3, AttemptID: "judge-000005", Kind: model.KindJudge, StartedAt: "2026-02-02T00:02:00Z",
	})
	writeJSON(t, filepath.Join(failed, "finished.json"), model.FinishedAttempt{
		SchemaVersion: 3, AttemptID: "judge-000005", Kind: model.KindJudge,
		FinishedAt: "2026-02-02T00:02:01Z", Status: model.StatusAgentFailed,
		Error: "judge invocation failed: Unexpected server error",
	})
	in, err := reportio.Load(dir, reportio.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var sawFailed, sawInterrupted bool
	for _, attempt := range in.Attempts {
		switch attempt.Status {
		case model.StatusAgentFailed:
			sawFailed = attempt.Kind == model.KindJudge && attempt.Error == "judge invocation failed: Unexpected server error"
		case model.StatusInterrupted:
			sawInterrupted = true
		}
	}
	if !sawFailed {
		t.Fatal("failed judge attempt lost its status or error")
	}
	if !sawInterrupted {
		t.Fatal("attempt without finished.json must be interrupted")
	}
}

func TestV3FailedRerunSupersedesAcceptedAttempt(t *testing.T) {
	dir := newV3Suite(t)
	runDir := filepath.Join(dir, "runs", "demo", "v3case", "with", "run-1")
	failed := filepath.Join(runDir, "attempts", "agent-000004")
	writeJSON(t, filepath.Join(failed, "started.json"), model.StartedAttempt{
		SchemaVersion: 3, AttemptID: "agent-000004", Kind: model.KindAgent,
		StartedAt: "2026-02-02T00:02:00Z",
	})
	writeJSON(t, filepath.Join(failed, "finished.json"), model.FinishedAttempt{
		SchemaVersion: 3, AttemptID: "agent-000004", Kind: model.KindAgent,
		FinishedAt: "2026-02-02T00:03:00Z", Status: model.StatusAgentFailed,
		StopReason: model.StopProviderError, Usage: model.Usage{State: model.UsageKnown, Known: true, Source: "x"},
	})
	// A failed allocation after it is not an attempt and must not hide it.
	if err := os.MkdirAll(filepath.Join(runDir, "attempts", "agent-000005"), 0o700); err != nil {
		t.Fatal(err)
	}

	in, err := reportio.Load(dir, reportio.Options{})
	if err != nil {
		t.Fatal(err)
	}
	with := in.Scenarios[0].Runs[0]
	if with.Status != string(model.StatusAgentFailed) || with.Complete || with.AgentAttempt != "agent-000004" ||
		len(with.JudgedItems) != 0 || with.JudgeAttempt != "" {
		t.Fatalf("with run = %+v, want the failed rerun", with)
	}
	if with.DurationSeconds == nil || *with.DurationSeconds != 60 {
		t.Fatalf("duration = %v, want the rerun's 60s", with.DurationSeconds)
	}
}
