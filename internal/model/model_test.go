package model

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func int64p(v int64) *int64       { return &v }
func float64p(v float64) *float64 { return &v }
func intp(v int) *int             { return &v }
func int64ptr(v int64) *int64     { return &v }

func roundTrip[T any](t *testing.T, in T) T {
	t.Helper()
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

func TestVersionConstants(t *testing.T) {
	if SchemaVersion != 3 {
		t.Fatalf("SchemaVersion = %d, want 3", SchemaVersion)
	}
	if ManifestVersion != SchemaVersion {
		t.Fatalf("ManifestVersion = %d, want %d", ManifestVersion, SchemaVersion)
	}
	if !strings.HasPrefix(GraderVersion, "evalctl/") || !strings.Contains(GraderVersion, "schema.3") {
		t.Fatalf("GraderVersion = %q", GraderVersion)
	}
}

func TestRunStatusValues(t *testing.T) {
	statuses := []RunStatus{
		StatusOK, StatusSetupFailed, StatusAgentFailed, StatusTimedOut,
		StatusEvidenceFailed, StatusChecksFailed, StatusCancelled,
		StatusIncomplete, StatusInterrupted, StatusMissing,
	}
	for _, status := range statuses {
		if !status.Valid() {
			t.Errorf("%q is not valid", status)
		}
	}
	if RunStatus("bogus").Valid() {
		t.Error("bogus status reported valid")
	}
	if StatusInterrupted != "interrupted" || StatusMissing != "missing" {
		t.Errorf("status strings = %q, %q", StatusInterrupted, StatusMissing)
	}
}

func TestUsageJSONStates(t *testing.T) {
	notApplicable := Usage{State: UsageNotApplicable, Source: "direct-hook"}
	data, err := json.Marshal(notApplicable)
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"input_tokens", "output_tokens", "cost_usd", "reason", "currency"} {
		if strings.Contains(string(data), absent) {
			t.Errorf("not_applicable usage contains %q: %s", absent, data)
		}
	}
	if !strings.Contains(string(data), `"known":false`) {
		t.Errorf("known compatibility field missing: %s", data)
	}

	unknown := Usage{
		State:       UsageUnknown,
		Known:       false,
		Source:      "opencode",
		Reason:      "missing usage",
		InputTokens: int64p(10),
	}
	got := roundTrip(t, unknown)
	if !reflect.DeepEqual(got, unknown) {
		t.Fatalf("unknown usage round trip = %#v, want %#v", got, unknown)
	}

	known := Usage{
		State:            UsageKnown,
		Known:            true,
		Source:           "provider",
		InputTokens:      int64p(1),
		OutputTokens:     int64p(2),
		ReasoningTokens:  int64p(3),
		CacheReadTokens:  int64p(4),
		CacheWriteTokens: int64p(5),
		TotalTokens:      int64p(15),
		CostUSD:          float64p(0.25),
		Currency:         "USD",
	}
	if got := roundTrip(t, known); !reflect.DeepEqual(got, known) {
		t.Fatalf("known usage round trip = %#v, want %#v", got, known)
	}
}

func TestStartedAttemptRoundTrip(t *testing.T) {
	in := StartedAttempt{
		SchemaVersion: SchemaVersion,
		AttemptID:     "agent-000001",
		Kind:          KindAgent,
		RunID:         "plugin/scenario/with/1",
		StartedAt:     "2026-10-19T00:00:00Z",
		Fingerprint:   "abc",
		ConfigSHA256:  "def",
		Executor:      "command",
		Model:         "model",
		Command:       "agent --flag",
	}
	if got := roundTrip(t, in); !reflect.DeepEqual(got, in) {
		t.Fatalf("round trip = %#v, want %#v", got, in)
	}
}

func TestFinishedAttemptRoundTrip(t *testing.T) {
	in := FinishedAttempt{
		SchemaVersion:        SchemaVersion,
		AttemptID:            "judge-000002",
		Kind:                 KindJudge,
		FinishedAt:           "2026-10-19T00:01:00Z",
		Status:               StatusOK,
		StopReason:           StopCompleted,
		Usage:                Usage{State: UsageKnown, Known: true, Source: "judge", TotalTokens: int64p(12)},
		ChecksStatus:         ChecksOK,
		MechanicalSHA256:     "m",
		EvidenceSHA256:       "e",
		GradingRequestSHA256: "g",
		GradedAgentAttempt:   "agent-000001",
		GradedEvidenceSHA256: "e",
		JudgeIdentitySHA256:  "ji",
		JudgementSHA256:      "j",
	}
	if got := roundTrip(t, in); !reflect.DeepEqual(got, in) {
		t.Fatalf("round trip = %#v, want %#v", got, in)
	}
}

func TestCurrentPointerOptionalJudgeFields(t *testing.T) {
	in := CurrentPointer{
		SchemaVersion:             SchemaVersion,
		AgentAttempt:              "agent-000001",
		AgentEvidenceSHA256:       "e",
		AgentGradingRequestSHA256: "g",
		AcceptedAt:                "2026-10-19T00:02:00Z",
		AcceptedBy:                "evalctl/0.1.0-dev",
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"judge_attempt", "judge_identity_sha256", "graded_agent_attempt"} {
		if strings.Contains(string(data), absent) {
			t.Errorf("pointer contains %q: %s", absent, data)
		}
	}
	if got := roundTrip(t, in); !reflect.DeepEqual(got, in) {
		t.Fatalf("round trip = %#v, want %#v", got, in)
	}
}

func TestRunResultNullFields(t *testing.T) {
	in := RunResult{
		SchemaVersion:  SchemaVersion,
		Plugin:         "plugin",
		Scenario:       "scenario",
		Arm:            "with",
		Mode:           "comparison",
		Run:            1,
		AgentAttempt:   "agent-000001",
		CurrentSHA256:  "c",
		Complete:       true,
		Status:         StatusOK,
		Fingerprint:    "f",
		Mechanical:     Mechanical{Score: 4, Max: 10, Items: []JudgedItem{{Name: "a", Score: 4, Max: 10}}},
		Usage:          Usage{State: UsageKnown, Known: true, Source: "agent"},
		AgentUsage:     Usage{State: UsageKnown, Known: true, Source: "agent"},
		JudgeUsage:     Usage{State: UsageNotApplicable, Source: "no-judge"},
		CombinedUsage:  Usage{State: UsageKnown, Known: true, Source: "agent"},
		Evidence:       "attempts/agent-000001",
		EvidenceSHA256: "e",
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"agent_exit":null`) {
		t.Errorf("agent_exit should be null: %s", data)
	}
	if !strings.Contains(string(data), `"duration_seconds":null`) {
		t.Errorf("duration_seconds should be null: %s", data)
	}
	if got := roundTrip(t, in); !reflect.DeepEqual(got, in) {
		t.Fatalf("round trip = %#v, want %#v", got, in)
	}
}

func TestGradingAndJudgementRoundTrip(t *testing.T) {
	request := GradingRequest{
		SchemaVersion:  SchemaVersion,
		Plugin:         "plugin",
		Scenario:       "scenario",
		Arm:            "with",
		Run:            1,
		AgentAttempt:   "agent-000001",
		EvidenceSHA256: "e",
		Criteria: Rubric{
			Context: "context",
			Type:    "weighted_checklist",
			Checklist: []Criterion{
				{Name: "a", Description: "first", MaxScore: 5},
				{Name: "b", Description: "second", MaxScore: 5},
			},
		},
		CoveredByChecks:  []string{"a"},
		JudgementPending: []Criterion{{Name: "b", Description: "second", MaxScore: 5}},
	}
	if got := roundTrip(t, request); !reflect.DeepEqual(got, request) {
		t.Fatalf("request round trip = %#v, want %#v", got, request)
	}

	judgement := Judgement{
		{Name: "a", Score: 1.5, Max: 5},
		{Name: "b", Score: 5, Max: 5, Note: "done"},
	}
	data, err := json.Marshal(judgement)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(strings.TrimSpace(string(data)), "{") {
		t.Fatalf("judgement must be a bare array: %s", data)
	}
	if got := roundTrip(t, judgement); !reflect.DeepEqual(got, judgement) {
		t.Fatalf("judgement round trip = %#v, want %#v", got, judgement)
	}
}

func TestJudgeIdentityAndResult(t *testing.T) {
	identity := JudgeIdentity{
		Kind:     "native",
		Provider: "openai-chat",
		BaseURL:  "https://api.openai.com/v1",
		Model:    "model",
		Generation: GenerationIdentity{
			Temperature:     "0.0",
			Seed:            nil,
			MaxOutputTokens: 4096,
			MaxSteps:        40,
			ContextWindow:   128000,
		},
		PromptVersion: "judge-prompt/1",
		GraderVersion: GraderVersion,
		SchemaVersion: SchemaVersion,
	}
	data, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"seed":null`) {
		t.Errorf("absent seed must serialize as null: %s", data)
	}
	if got := roundTrip(t, identity); !reflect.DeepEqual(got, identity) {
		t.Fatalf("identity round trip = %#v, want %#v", got, identity)
	}

	result := JudgeResult{
		SchemaVersion:        SchemaVersion,
		ExitCode:             0,
		TimedOut:             false,
		DurationSeconds:      float64p(12.4),
		Valid:                true,
		Identity:             identity,
		JudgeIdentitySHA256:  "ji",
		JudgeCmd:             "judge --flag",
		Attempt:              "judge-000002",
		GradedAgentAttempt:   "agent-000001",
		GradedEvidenceSHA256: "e",
		GradingRequestSHA256: "g",
		JudgementSHA256:      "j",
	}
	if got := roundTrip(t, result); !reflect.DeepEqual(got, result) {
		t.Fatalf("result round trip = %#v, want %#v", got, result)
	}
}

func TestSuiteManifestRoundTrip(t *testing.T) {
	manifest := SuiteManifest{
		Version:   ManifestVersion,
		CreatedAt: "2026-10-19T00:00:00Z",
		Tool: ToolInfo{
			Name: "evalctl", Version: Version, Commit: "abc",
			SchemaVersion: SchemaVersion, GraderVersion: GraderVersion,
		},
		Repo: RepoInfo{Root: "/repo", GitSHA: "abc", GitDirty: true},
		Config: SuiteConfig{
			Backend: "command", AgentCmd: "agent", Arm: "with", Runs: 2,
			TimeoutSeconds: 300, SetupTimeoutSeconds: 60, ChecksTimeoutSeconds: 60,
			JudgeTimeoutSeconds: 300, SkillDir: ".claude/skills", AuthPaths: []string{},
			Units: []string{"skills/pr"}, Judge: JudgeConfig{Backend: "command", Cmd: "judge"},
			AttemptArtifacts: "failed", HarnessVersion: Version,
		},
		Scenarios: []PlannedScenario{{
			Plugin: "plugin", Scenario: "scenario", Fingerprint: "f",
			Units: []string{"skills/pr"}, Mode: "comparison", Execution: "agent",
		}},
	}
	if got := roundTrip(t, manifest); !reflect.DeepEqual(got, manifest) {
		t.Fatalf("manifest round trip = %#v, want %#v", got, manifest)
	}
}

func TestReportRoundTrip(t *testing.T) {
	report := Report{
		Version: SchemaVersion,
		Metadata: ReportMetadata{
			SuiteDir: "suite", Units: []string{"skills/pr"}, N: 2,
			Counts: map[string]int{"ok": 2},
		},
		Totals: Totals{
			With: float64p(1), Without: float64p(0.5), Delta: float64p(0.5),
			N: 1, PendingPoints: 2, PendingCriteria: 1,
		},
		Timing: Timing{TotalSeconds: 10, MeanSeconds: float64p(5)},
		Cost:   CostReport{Currency: "USD"},
		Expenditure: Expenditure{
			Currency: "USD", KnownTotal: Usage{State: UsageKnown, Known: true, Source: "agent"},
			TotalAttempts: 2, KnownAttempts: 2, AgentAttempts: 2,
		},
		Verdict: Verdict{Status: "helps"},
		Criteria: []CriterionResult{{
			Name: "a", Max: 10, With: float64p(8), Without: float64p(4), Delta: float64p(4),
			PairedRuns: 1, Status: "scored",
		}},
		Scenarios:  []ScenarioResult{{Plugin: "p", Scenario: "s", Mode: "comparison", Status: "ok"}},
		Comparison: &Comparison{Status: "comparable", Comparable: true},
		Links:      Links{JSON: "report.json", Markdown: "report.md", HTML: "report.html"},
	}
	if got := roundTrip(t, report); !reflect.DeepEqual(got, report) {
		t.Fatalf("report round trip = %#v, want %#v", got, report)
	}
}

func TestAttemptKindAndChecksConstants(t *testing.T) {
	kinds := []AttemptKind{KindAgent, KindHook, KindJudge}
	want := []string{"agent", "hook", "judge"}
	for i, kind := range kinds {
		if string(kind) != want[i] {
			t.Errorf("kind %d = %q, want %q", i, kind, want[i])
		}
	}
	checks := []string{ChecksOK, ChecksNotApplicable, ChecksFailed}
	wantChecks := []string{"ok", "not-applicable", "failed"}
	for i, check := range checks {
		if check != wantChecks[i] {
			t.Errorf("checks status %d = %q, want %q", i, check, wantChecks[i])
		}
	}
}

func TestSameJudgeModel(t *testing.T) {
	cases := []struct {
		name                                         string
		agentCmd, agentModel, judgeCmd, judgeModelIn string
		wantName                                     string
		wantSame                                     bool
	}{
		{"flags match", "opencode run -m p/m1 x", "", "opencode run --model p/m1 y", "", "p/m1", true},
		{"flags differ", "opencode run -m p/m1", "", "opencode run -m p/m2", "", "p/m1", false},
		{"explicit names", "a", "m", "b", "m", "m", true},
		{"explicit overrides flag", "x -m p/m1", "m2", "y -m p/m1", "", "m2", false},
		{"identical commands without model", "claude -p", "", "claude -p", "", "", true},
		{"different commands without model", "claude -p", "", "codex exec", "", "", false},
		{"one side unknown", "claude -p", "", "judge -m p/m1", "", "", false},
		{"empty", "", "", "", "", "", false},
	}
	for _, tc := range cases {
		name, same := SameJudgeModel(tc.agentCmd, tc.agentModel, tc.judgeCmd, tc.judgeModelIn)
		if same != tc.wantSame || (same && name != tc.wantName) {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", tc.name, name, same, tc.wantName, tc.wantSame)
		}
	}
}

func TestUsageWithStateDerivesKnown(t *testing.T) {
	u := Unknown("x", "missing").WithState(UsageKnown)
	if u.State != UsageKnown || !u.Known {
		t.Fatalf("known: %+v", u)
	}
	u = u.WithState(UsageUnknown)
	if u.State != UsageUnknown || u.Known {
		t.Fatalf("unknown: %+v", u)
	}
}

func TestUsageMarshalDerivesKnownFromState(t *testing.T) {
	for _, test := range []struct {
		usage Usage
		want  string
	}{
		{Usage{State: UsageKnown, Known: false}, `"known":true`},
		{Usage{State: UsageUnknown, Known: true}, `"known":false`},
		{Usage{Known: true}, `"known":true`},
	} {
		data, err := json.Marshal(test.usage)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), test.want) {
			t.Errorf("%+v marshalled to %s, want %s", test.usage, data, test.want)
		}
		var round Usage
		if err := json.Unmarshal(data, &round); err != nil {
			t.Fatal(err)
		}
		if round.State != test.usage.State {
			t.Errorf("round trip state = %q, want %q", round.State, test.usage.State)
		}
	}
}

func usageWith(state UsageState, total *int64, cost *float64) Usage {
	u := Usage{Source: "t", TotalTokens: total, CostUSD: cost}.WithState(state)
	return u
}

func TestSumUsageRequiresEveryRecord(t *testing.T) {
	n := func(v int64) *int64 { return &v }
	c := func(v float64) *float64 { return &v }

	all := SumUsage("t", []Usage{
		usageWith(UsageKnown, n(10), c(0.1)),
		usageWith(UsageKnown, n(20), c(0.2)),
		NotApplicable("judge"),
	})
	if all.State != UsageKnown || all.TotalTokens == nil || *all.TotalTokens != 30 || all.CostUSD == nil {
		t.Fatalf("complete sum = %+v", all)
	}

	// One record without cost: tokens stay exact, cost is not a total.
	partialCost := SumUsage("t", []Usage{
		usageWith(UsageKnown, n(10), c(0.1)),
		usageWith(UsageKnown, n(20), nil),
	})
	if partialCost.State != UsageUnknown || partialCost.CostUSD != nil ||
		partialCost.TotalTokens == nil || *partialCost.TotalTokens != 30 {
		t.Fatalf("partial cost sum = %+v", partialCost)
	}

	withUnknown := SumUsage("t", []Usage{usageWith(UsageKnown, n(10), c(0.1)), Unknown("t", "")})
	if withUnknown.State != UsageUnknown || withUnknown.TotalTokens != nil || withUnknown.CostUSD != nil {
		t.Fatalf("sum with an unknown record = %+v", withUnknown)
	}

	if none := SumUsage("t", []Usage{NotApplicable("t")}); none.State != UsageNotApplicable {
		t.Fatalf("sum of nothing applicable = %+v", none)
	}
}

func TestSumLowerBoundCompleteness(t *testing.T) {
	n := func(v int64) *int64 { return &v }
	c := func(v float64) *float64 { return &v }

	total, complete := SumLowerBound("t", []Usage{
		usageWith(UsageKnown, n(10), c(0.1)),
		usageWith(UsageKnown, n(20), c(0.2)),
	})
	if !complete || total.CostUSD == nil || *total.CostUSD < 0.3-1e-9 {
		t.Fatalf("complete lower bound = %+v, %v", total, complete)
	}

	// A known record that reports no cost leaves the cost a lower bound.
	total, complete = SumLowerBound("t", []Usage{
		usageWith(UsageKnown, n(10), c(0.1)),
		usageWith(UsageKnown, n(20), nil),
	})
	if complete || total.CostUSD == nil || *total.CostUSD != 0.1 {
		t.Fatalf("missing cost lower bound = %+v, %v", total, complete)
	}

	// Partial components of an unknown record still count toward the bound.
	partial := Unknown("t", "no cost")
	partial.TotalTokens = n(5)
	total, complete = SumLowerBound("t", []Usage{usageWith(UsageKnown, n(10), nil), partial})
	if complete || total.TotalTokens == nil || *total.TotalTokens != 15 {
		t.Fatalf("unknown partial lower bound = %+v, %v", total, complete)
	}
}
