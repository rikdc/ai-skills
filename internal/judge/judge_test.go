package judge

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rikdc/ai-skills/internal/executor"
	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/platform/errs"
	"github.com/rikdc/ai-skills/internal/runner/attempts"
)

const testCommand = "stub-judge"

func knownJudgeUsage() model.Usage {
	input, output, total := int64(100), int64(20), int64(120)
	cost := 0.05
	return model.Usage{
		State:        model.UsageKnown,
		Known:        true,
		Source:       "test-judge",
		InputTokens:  &input,
		OutputTokens: &output,
		TotalTokens:  &total,
		CostUSD:      &cost,
	}
}

func knownAgentUsage() model.Usage {
	input, output, total := int64(10), int64(5), int64(15)
	cost := 0.01
	return model.Usage{
		State:        model.UsageKnown,
		Known:        true,
		Source:       "test-agent",
		InputTokens:  &input,
		OutputTokens: &output,
		TotalTokens:  &total,
		CostUSD:      &cost,
	}
}

// fixture is an accepted agent run ready for grading.
type fixture struct {
	t          *testing.T
	dir        string
	store      *attempts.Store
	agent      *attempts.Attempt
	request    model.GradingRequest
	rawRequest []byte
}

func newFixture(t *testing.T, pending []model.Criterion) *fixture {
	t.Helper()
	return newFixtureAt(t, t.TempDir(), pending)
}

func newFixtureAt(t *testing.T, dir string, pending []model.Criterion) *fixture {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, dir: dir, store: attempts.New(dir)}
	f.agent = f.addAgent(pending)
	if _, err := f.store.Accept(f.agent.ID, attempts.AcceptOptions{}); err != nil {
		t.Fatalf("accept agent: %v", err)
	}
	raw, err := os.ReadFile(f.store.GradingRequestPath())
	if err != nil {
		t.Fatal(err)
	}
	f.rawRequest = raw
	if err := json.Unmarshal(raw, &f.request); err != nil {
		t.Fatal(err)
	}
	return f
}

// addAgent creates a finished-ok execution attempt with evidence and request,
// but does not accept it.
func (f *fixture) addAgent(pending []model.Criterion) *attempts.Attempt {
	f.t.Helper()
	agent, err := f.store.Allocate(model.StartedAttempt{
		Kind:         model.KindAgent,
		RunID:        "demo/basic/with/run-1",
		Fingerprint:  "fingerprint-1",
		ConfigSHA256: "config-1",
		Executor:     "command",
	})
	if err != nil {
		f.t.Fatalf("allocate agent: %v", err)
	}
	files := map[string]string{
		"before.txt":      "before state\n",
		"after.txt":       "after state\n",
		"agent.log":       "agent output\n",
		"mechanical.json": `{"score":3,"max":5,"items":[{"name":"Mechanical","score":3,"max":5,"note":"mech"}]}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(agent.Dir, name), []byte(content), 0o600); err != nil {
			f.t.Fatal(err)
		}
	}
	paths := []string{"before.txt", "after.txt", "agent.log", "mechanical.json"}
	evidenceSHA, err := f.store.WriteEvidenceManifest(agent.ID, paths)
	if err != nil {
		f.t.Fatalf("write evidence: %v", err)
	}
	request := model.GradingRequest{
		SchemaVersion:  model.SchemaVersion,
		Plugin:         "demo",
		Scenario:       "basic",
		Arm:            "with",
		Run:            1,
		AgentAttempt:   agent.ID,
		EvidenceSHA256: evidenceSHA,
		Criteria: model.Rubric{
			Context:   "fixture context",
			Type:      "weighted_checklist",
			Checklist: append([]model.Criterion(nil), pending...),
		},
		CoveredByChecks:  []string{},
		JudgementPending: append([]model.Criterion(nil), pending...),
	}
	requestSHA, err := f.store.WriteGradingRequest(request)
	if err != nil {
		f.t.Fatalf("write request: %v", err)
	}
	agent, err = f.store.Finish(agent.ID, model.FinishedAttempt{
		Status:               model.StatusOK,
		StopReason:           model.StopCompleted,
		Usage:                knownAgentUsage(),
		ChecksStatus:         model.ChecksOK,
		EvidenceSHA256:       evidenceSHA,
		GradingRequestSHA256: requestSHA,
	})
	if err != nil {
		f.t.Fatalf("finish agent: %v", err)
	}
	f.request = request
	return agent
}

func standardPending() []model.Criterion {
	return []model.Criterion{
		{Name: "X", Description: "x description", MaxScore: 4},
		{Name: "Y", Description: "y description", MaxScore: 6},
	}
}

type fakeExecutor struct {
	command string
	calls   int
	last    executor.AgentRunRequest
	run     func(executor.AgentRunRequest) (executor.AgentRunResult, error)
}

func (f *fakeExecutor) Run(_ context.Context, req executor.AgentRunRequest) (executor.AgentRunResult, error) {
	f.calls++
	f.last = req
	if f.run != nil {
		return f.run(req)
	}
	return stubMaxJudgement(req)
}

func (f *fakeExecutor) Identity() executor.RuntimeIdentity {
	return executor.RuntimeIdentity{Kind: "command", Command: f.command}
}

func okJudgeResult(usage model.Usage) executor.AgentRunResult {
	code := 0
	return executor.AgentRunResult{
		Status:     model.StatusOK,
		StopReason: model.StopCompleted,
		Usage:      usage,
		ExitCode:   &code,
		Duration:   1500 * time.Millisecond,
	}
}

// stubMaxJudgement reads the staged grading request from the judge working
// directory and scores every pending criterion at its maximum.
func stubMaxJudgement(req executor.AgentRunRequest) (executor.AgentRunResult, error) {
	data, err := os.ReadFile(filepath.Join(req.Workspace, "grading-request.json"))
	if err != nil {
		return executor.AgentRunResult{}, err
	}
	var request model.GradingRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return executor.AgentRunResult{}, err
	}
	items := make([]model.JudgedItem, 0, len(request.JudgementPending))
	for _, criterion := range request.JudgementPending {
		items = append(items, model.JudgedItem{
			Name: criterion.Name, Score: criterion.MaxScore, Max: criterion.MaxScore, Note: "stub",
		})
	}
	judgement, err := json.Marshal(items)
	if err != nil {
		return executor.AgentRunResult{}, err
	}
	if err := os.WriteFile(filepath.Join(req.Workspace, "judgement.json"), judgement, 0o600); err != nil {
		return executor.AgentRunResult{}, err
	}
	return okJudgeResult(knownJudgeUsage()), nil
}

func newFake(command string) *fakeExecutor {
	return &fakeExecutor{command: command}
}

func gradeRun(t *testing.T, f *fixture, fake *fakeExecutor, command string, regrade bool) (Result, error) {
	t.Helper()
	return Grade(context.Background(), Config{
		RunDir:   f.dir,
		Command:  command,
		Timeout:  time.Minute,
		Regrade:  regrade,
		Executor: fake,
	})
}

func TestBuildPromptIncludesRunRubricAndEvidence(t *testing.T) {
	request := model.GradingRequest{
		Plugin:           "demo",
		Scenario:         "basic",
		Arm:              "with",
		Run:              2,
		AgentAttempt:     "agent-000001",
		Criteria:         model.Rubric{Context: "fixture context"},
		CoveredByChecks:  []string{"Mechanical A", "Mechanical B"},
		JudgementPending: standardPending(),
	}
	prompt := BuildPrompt(request)
	for _, want := range []string{
		"# Grade this eval run",
		"- Plugin: demo",
		"- Scenario: basic",
		"- Arm: with",
		"- Run: 2",
		"fixture context",
		"Mechanical A, Mechanical B",
		"- **X** (max 4): x description",
		"- **Y** (max 6): y description",
		"`before.txt`",
		"`after.txt`",
		"`agent.log`",
		"`setup.log`",
		"`mechanical.json`",
		"`snapshot.tar`",
		"`judgement.json`",
		"attempts/",
		"agent-000001",
		"Do not modify the evidence files.",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "(none)") {
		t.Errorf("covered list must not render as (none):\n%s", prompt)
	}
	request.CoveredByChecks = nil
	if !strings.Contains(BuildPrompt(request), "(none)") {
		t.Error("empty covered list must render as (none)")
	}
	if got, again := PromptVersion(), PromptVersion(); got == "" || got != again {
		t.Fatalf("PromptVersion must be stable and non-empty: %q %q", got, again)
	}
}

func TestGradeRunsJudgeInEvidenceDirAndAccepts(t *testing.T) {
	f := newFixture(t, standardPending())
	fake := newFake(testCommand)

	result, err := gradeRun(t, f, fake, testCommand, false)
	if err != nil {
		t.Fatalf("grade: %v", err)
	}
	if result.Outcome != OutcomeGraded {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if result.Score != 10 || result.Max != 10 {
		t.Fatalf("score = %v/%v", result.Score, result.Max)
	}
	if fake.calls != 1 {
		t.Fatalf("judge calls = %d", fake.calls)
	}
	if fake.last.Workspace != f.agent.Dir {
		t.Fatalf("judge workspace = %q, want %q", fake.last.Workspace, f.agent.Dir)
	}
	if fake.last.Kind != model.KindJudge || fake.last.LogFile != "judge.log" {
		t.Fatalf("judge request = %+v", fake.last)
	}

	pointer, _, err := f.store.LoadCurrent()
	if err != nil {
		t.Fatal(err)
	}
	if pointer.JudgeAttempt == "" {
		t.Fatal("current pointer has no judge attempt")
	}
	if pointer.GradedAgentAttempt != pointer.AgentAttempt ||
		pointer.GradedEvidenceSHA256 != pointer.AgentEvidenceSHA256 ||
		pointer.GradingRequestSHA256 != pointer.AgentGradingRequestSHA256 {
		t.Fatalf("pointer binding = %+v", pointer)
	}
	identitySHA, err := IdentitySHA256(CommandIdentity(testCommand))
	if err != nil {
		t.Fatal(err)
	}
	if pointer.JudgeIdentitySHA256 != identitySHA {
		t.Fatalf("pointer identity = %q, want %q", pointer.JudgeIdentitySHA256, identitySHA)
	}

	judge, err := f.store.LoadAttempt(pointer.JudgeAttempt)
	if err != nil {
		t.Fatal(err)
	}
	if judge.Finished == nil || judge.Finished.Status != model.StatusOK {
		t.Fatalf("judge attempt not ok: %+v", judge.Finished)
	}
	if judge.Finished.GradedAgentAttempt != pointer.AgentAttempt ||
		judge.Finished.GradedEvidenceSHA256 != pointer.AgentEvidenceSHA256 ||
		judge.Finished.GradingRequestSHA256 != pointer.AgentGradingRequestSHA256 ||
		judge.Finished.JudgeIdentitySHA256 != identitySHA {
		t.Fatalf("judge binding = %+v", judge.Finished)
	}
	for _, name := range []string{"started.json", "finished.json", "judge-prompt.md", "judge.log",
		"judgement.json", "judge-result.json", "judge-usage.json"} {
		if _, err := os.Stat(filepath.Join(judge.Dir, name)); err != nil {
			t.Errorf("judge attempt missing %s: %v", name, err)
		}
	}
	prompt, err := os.ReadFile(filepath.Join(judge.Dir, "judge-prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(prompt) != fake.last.Task {
		t.Error("stdin prompt differs from judge-prompt.md")
	}
	if !strings.Contains(string(prompt), "`before.txt`") {
		t.Errorf("prompt missing evidence reference:\n%s", prompt)
	}

	var judgeResult model.JudgeResult
	data, err := os.ReadFile(filepath.Join(judge.Dir, "judge-result.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &judgeResult); err != nil {
		t.Fatal(err)
	}
	if !judgeResult.Valid || judgeResult.JudgeCmd != testCommand ||
		judgeResult.JudgeIdentitySHA256 != identitySHA ||
		judgeResult.Attempt != judge.ID ||
		judgeResult.Identity.Command != testCommand ||
		judgeResult.GradedAgentAttempt != pointer.AgentAttempt ||
		judgeResult.GradedEvidenceSHA256 != pointer.AgentEvidenceSHA256 ||
		judgeResult.GradingRequestSHA256 != pointer.AgentGradingRequestSHA256 ||
		judgeResult.JudgementSHA256 != judge.Finished.JudgementSHA256 {
		t.Fatalf("judge-result = %+v", judgeResult)
	}

	// The judge's transient files are not left in the evidence directory.
	for _, name := range []string{"judgement.json", "grading-request.json"} {
		if _, err := os.Stat(filepath.Join(f.agent.Dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s left in evidence dir", name)
		}
	}

	again, err := gradeRun(t, f, fake, testCommand, false)
	if err != nil {
		t.Fatalf("second grade: %v", err)
	}
	if again.Outcome != OutcomeAlreadyComplete || again.Attempt != judge.ID {
		t.Fatalf("second grade = %+v", again)
	}
	if fake.calls != 1 {
		t.Fatalf("already graded run invoked the judge again: %d calls", fake.calls)
	}
}

func TestInvalidJudgementsStayPending(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"non-array", `{}`, "not a JSON array"},
		{"empty", `[]`, "empty"},
		{"subset", `[{"name":"X","score":4,"max":4}]`, "missing criterion"},
		{"superset", `[{"name":"X","score":4,"max":4},{"name":"Y","score":6,"max":6},{"name":"Z","score":1,"max":1}]`, "unknown criterion"},
		{"wrong max", `[{"name":"X","score":4,"max":5},{"name":"Y","score":6,"max":6}]`, "does not equal rubric max"},
		{"score over max", `[{"name":"X","score":9,"max":4},{"name":"Y","score":6,"max":6}]`, "> max"},
		{"negative", `[{"name":"X","score":-1,"max":4},{"name":"Y","score":6,"max":6}]`, "< 0"},
		{"duplicate", `[{"name":"X","score":4,"max":4},{"name":"X","score":4,"max":4}]`, "duplicate criterion"},
		{"empty name", `[{"name":"","score":0,"max":0}]`, "empty criterion name"},
		{"missing score", `[{"name":"X","max":4}]`, "missing score"},
		{"NaN", `[{"name":"X","score":NaN,"max":4}]`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, standardPending())
			fake := newFake(testCommand)
			fake.run = func(req executor.AgentRunRequest) (executor.AgentRunResult, error) {
				if err := os.WriteFile(filepath.Join(req.Workspace, "judgement.json"), []byte(tc.raw), 0o600); err != nil {
					t.Fatal(err)
				}
				return okJudgeResult(knownJudgeUsage()), nil
			}
			_, err := gradeRun(t, f, fake, testCommand, false)
			if err == nil {
				t.Fatal("invalid judgement was accepted")
			}
			if code := errs.Code(err); code != 1 {
				t.Fatalf("exit code = %d, want 1 (err=%v)", code, err)
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want %q", err, tc.want)
			}
			pointer, _, err := f.store.LoadCurrent()
			if err != nil {
				t.Fatal(err)
			}
			if pointer.JudgeAttempt != "" {
				t.Fatalf("invalid judgement became current: %+v", pointer)
			}
			list, err := f.store.Enumerate()
			if err != nil {
				t.Fatal(err)
			}
			var judges []*attempts.Attempt
			for _, attempt := range list {
				if attempt.Kind == model.KindJudge {
					judges = append(judges, attempt)
				}
			}
			if len(judges) != 1 {
				t.Fatalf("judge attempts = %d", len(judges))
			}
			if judges[0].Finished == nil || judges[0].Finished.Status == model.StatusOK {
				t.Fatalf("failed judge attempt recorded as ok: %+v", judges[0].Finished)
			}
			if judges[0].Finished.JudgementSHA256 == "" {
				t.Error("raw rejected judgement was not recorded")
			}
			log, err := os.ReadFile(filepath.Join(judges[0].Dir, "judge.log"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(log), "validation failed") {
				t.Errorf("judge.log missing validation error: %q", log)
			}
		})
	}
}

func TestJudgeUsageKnownAndUnknown(t *testing.T) {
	t.Run("known", func(t *testing.T) {
		f := newFixture(t, standardPending())
		fake := newFake(testCommand)
		if _, err := gradeRun(t, f, fake, testCommand, false); err != nil {
			t.Fatal(err)
		}
		pointer, _, _ := f.store.LoadCurrent()
		data, err := os.ReadFile(filepath.Join(f.store.AttemptsDir(), pointer.JudgeAttempt, "judge-usage.json"))
		if err != nil {
			t.Fatal(err)
		}
		var usage model.Usage
		if err := json.Unmarshal(data, &usage); err != nil {
			t.Fatal(err)
		}
		if usage.State != model.UsageKnown || !usage.Known || usage.InputTokens == nil || *usage.InputTokens != 100 {
			t.Fatalf("judge usage = %+v", usage)
		}
	})
	t.Run("unknown", func(t *testing.T) {
		f := newFixture(t, standardPending())
		fake := newFake(testCommand)
		fake.run = func(req executor.AgentRunRequest) (executor.AgentRunResult, error) {
			result, err := stubMaxJudgement(req)
			if err != nil {
				return result, err
			}
			result.Usage = model.Unknown("none", "no usage record")
			return result, nil
		}
		if _, err := gradeRun(t, f, fake, testCommand, false); err != nil {
			t.Fatal(err)
		}
		pointer, _, _ := f.store.LoadCurrent()
		data, err := os.ReadFile(filepath.Join(f.store.AttemptsDir(), pointer.JudgeAttempt, "judge-usage.json"))
		if err != nil {
			t.Fatal(err)
		}
		var usage model.Usage
		if err := json.Unmarshal(data, &usage); err != nil {
			t.Fatal(err)
		}
		if usage.State != model.UsageUnknown || usage.Known || usage.CostUSD != nil {
			t.Fatalf("judge usage = %+v", usage)
		}
	})
}

func TestProvenanceMismatchWithoutRegradeExitsTwo(t *testing.T) {
	f := newFixture(t, standardPending())
	first := newFake(testCommand)
	if _, err := gradeRun(t, f, first, testCommand, false); err != nil {
		t.Fatal(err)
	}
	pointer, _, _ := f.store.LoadCurrent()
	accepted := pointer.JudgeAttempt

	second := newFake("other-judge")
	_, err := gradeRun(t, f, second, "other-judge", false)
	if err == nil {
		t.Fatal("different judge identity was reused")
	}
	if code := errs.Code(err); code != 2 {
		t.Fatalf("exit code = %d, want 2 (err=%v)", code, err)
	}
	if !strings.Contains(err.Error(), "--regrade") {
		t.Fatalf("error does not suggest --regrade: %v", err)
	}
	if second.calls != 0 {
		t.Fatalf("provenance mismatch invoked the judge %d times", second.calls)
	}
	after, _, _ := f.store.LoadCurrent()
	if after.JudgeAttempt != accepted {
		t.Fatalf("pointer changed: %q -> %q", accepted, after.JudgeAttempt)
	}
}

func TestStaleBindingFieldsRejected(t *testing.T) {
	f := newFixture(t, standardPending())
	identitySHA, err := IdentitySHA256(CommandIdentity(testCommand))
	if err != nil {
		t.Fatal(err)
	}
	stale, err := f.store.Allocate(model.StartedAttempt{
		Kind: model.KindJudge, RunID: "demo/basic/with/run-1", Fingerprint: "fingerprint-1",
		ConfigSHA256: identitySHA, Executor: "command", Command: testCommand,
	})
	if err != nil {
		t.Fatal(err)
	}
	judgement, _ := json.Marshal([]model.JudgedItem{
		{Name: "X", Score: 4, Max: 4}, {Name: "Y", Score: 6, Max: 6},
	})
	if err := os.WriteFile(filepath.Join(stale.Dir, "judgement.json"), judgement, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Finish(stale.ID, model.FinishedAttempt{
		Status:               model.StatusOK,
		StopReason:           model.StopCompleted,
		Usage:                knownJudgeUsage(),
		ChecksStatus:         model.ChecksNotApplicable,
		GradedAgentAttempt:   "agent-999999",
		GradedEvidenceSHA256: "deadbeef",
		GradingRequestSHA256: "deadbeef",
		JudgeIdentitySHA256:  identitySHA,
		JudgementSHA256:      attempts.SHA256Hex(judgement),
	}); err != nil {
		t.Fatal(err)
	}

	fake := newFake(testCommand)
	_, err = gradeRun(t, f, fake, testCommand, false)
	if err == nil {
		t.Fatal("stale judge attempt was reused")
	}
	if code := errs.Code(err); code != 2 || !strings.Contains(err.Error(), "--regrade") {
		t.Fatalf("err = %v (code %d), want 2 with --regrade", err, code)
	}
	if fake.calls != 0 {
		t.Fatalf("stale binding invoked judge %d times", fake.calls)
	}
}

func TestChangedEvidenceRejected(t *testing.T) {
	f := newFixture(t, standardPending())
	if _, err := gradeRun(t, f, newFake(testCommand), testCommand, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.agent.Dir, "agent.log"), []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := newFake(testCommand)
	_, err := gradeRun(t, f, fake, testCommand, false)
	if err == nil {
		t.Fatal("changed evidence was accepted")
	}
	if code := errs.Code(err); code != 2 {
		t.Fatalf("exit code = %d, want 2 (err=%v)", code, err)
	}
	if fake.calls != 0 {
		t.Fatalf("changed evidence invoked judge %d times", fake.calls)
	}
}

func TestChangedRequestRejected(t *testing.T) {
	f := newFixture(t, standardPending())
	if _, err := gradeRun(t, f, newFake(testCommand), testCommand, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.store.GradingRequestPath(), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := newFake(testCommand)
	_, err := gradeRun(t, f, fake, testCommand, false)
	if err == nil {
		t.Fatal("changed grading request was accepted")
	}
	if code := errs.Code(err); code != 2 {
		t.Fatalf("exit code = %d, want 2 (err=%v)", code, err)
	}
	if fake.calls != 0 {
		t.Fatalf("changed request invoked judge %d times", fake.calls)
	}
}

func TestChangedAgentAttemptRejected(t *testing.T) {
	f := newFixture(t, standardPending())
	if _, err := gradeRun(t, f, newFake(testCommand), testCommand, false); err != nil {
		t.Fatal(err)
	}
	// A retry accepts a new execution attempt without a judge.
	replacement := f.addAgent(standardPending())
	if _, err := f.store.Accept(replacement.ID, attempts.AcceptOptions{}); err != nil {
		t.Fatal(err)
	}
	fake := newFake(testCommand)
	_, err := gradeRun(t, f, fake, testCommand, false)
	if err == nil {
		t.Fatal("judgement bound to the previous agent attempt was reused")
	}
	if code := errs.Code(err); code != 2 || !strings.Contains(err.Error(), "--regrade") {
		t.Fatalf("err = %v (code %d), want 2 with --regrade", err, code)
	}
	if fake.calls != 0 {
		t.Fatalf("stale agent binding invoked judge %d times", fake.calls)
	}

	// --regrade grades the new attempt without rerunning the agent.
	regraded, err := gradeRun(t, f, newFake(testCommand), testCommand, true)
	if err != nil {
		t.Fatalf("regrade after retry: %v", err)
	}
	if regraded.Outcome != OutcomeGraded {
		t.Fatalf("regrade outcome = %q", regraded.Outcome)
	}
}

func TestRegradePreservesPriorExpenditure(t *testing.T) {
	f := newFixture(t, standardPending())
	if _, err := gradeRun(t, f, newFake(testCommand), testCommand, false); err != nil {
		t.Fatal(err)
	}
	firstPointer, _, _ := f.store.LoadCurrent()
	firstDir := filepath.Join(f.store.AttemptsDir(), firstPointer.JudgeAttempt)
	firstFinished, err := os.ReadFile(filepath.Join(firstDir, "finished.json"))
	if err != nil {
		t.Fatal(err)
	}
	firstUsage, err := os.ReadFile(filepath.Join(firstDir, "judge-usage.json"))
	if err != nil {
		t.Fatal(err)
	}

	secondCommand := "second-judge"
	second := newFake(secondCommand)
	second.run = func(req executor.AgentRunRequest) (executor.AgentRunResult, error) {
		items := []model.JudgedItem{
			{Name: "X", Score: 0, Max: 4, Note: "second"},
			{Name: "Y", Score: 0, Max: 6, Note: "second"},
		}
		data, _ := json.Marshal(items)
		if err := os.WriteFile(filepath.Join(req.Workspace, "judgement.json"), data, 0o600); err != nil {
			return executor.AgentRunResult{}, err
		}
		return okJudgeResult(knownJudgeUsage()), nil
	}
	result, err := gradeRun(t, f, second, secondCommand, true)
	if err != nil {
		t.Fatalf("regrade: %v", err)
	}
	if result.Outcome != OutcomeGraded || result.Score != 0 || result.Max != 10 {
		t.Fatalf("regrade result = %+v", result)
	}
	if second.calls != 1 {
		t.Fatalf("regrade judge calls = %d", second.calls)
	}

	// The prior immutable attempt and its expenditure are untouched.
	afterFinished, err := os.ReadFile(filepath.Join(firstDir, "finished.json"))
	if err != nil {
		t.Fatal(err)
	}
	afterUsage, err := os.ReadFile(filepath.Join(firstDir, "judge-usage.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(afterFinished) != string(firstFinished) || string(afterUsage) != string(firstUsage) {
		t.Fatal("regrade modified the prior judge attempt")
	}

	secondPointer, _, _ := f.store.LoadCurrent()
	if secondPointer.JudgeAttempt == firstPointer.JudgeAttempt {
		t.Fatal("regrade did not create a new judge attempt")
	}
	wantIdentity, _ := IdentitySHA256(CommandIdentity(secondCommand))
	if secondPointer.JudgeIdentitySHA256 != wantIdentity {
		t.Fatalf("regrade identity = %q, want %q", secondPointer.JudgeIdentitySHA256, wantIdentity)
	}
	list, err := f.store.Enumerate()
	if err != nil {
		t.Fatal(err)
	}
	var judges int
	var priorUsage model.Usage
	for _, attempt := range list {
		if attempt.Kind != model.KindJudge {
			continue
		}
		judges++
		if attempt.ID == firstPointer.JudgeAttempt {
			priorUsage = attempt.EffectiveUsage()
		}
	}
	if judges != 2 {
		t.Fatalf("judge attempts after regrade = %d", judges)
	}
	if priorUsage.State != model.UsageKnown || priorUsage.InputTokens == nil || *priorUsage.InputTokens != 100 {
		t.Fatalf("prior judge expenditure lost: %+v", priorUsage)
	}
}

func TestZeroPendingNeedsNoJudge(t *testing.T) {
	f := newFixture(t, nil)
	fake := newFake(testCommand)
	result, err := gradeRun(t, f, fake, testCommand, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeNothingPending {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if fake.calls != 0 {
		t.Fatalf("judge invoked with nothing pending: %d", fake.calls)
	}
	list, err := f.store.Enumerate()
	if err != nil {
		t.Fatal(err)
	}
	for _, attempt := range list {
		if attempt.Kind == model.KindJudge {
			t.Fatalf("judge attempt created with nothing pending: %s", attempt.ID)
		}
	}
}

func TestGradeSuiteScansRunsAndCounts(t *testing.T) {
	suite := t.TempDir()
	layout := func(run string) string {
		return filepath.Join(suite, "runs", "demo", "basic", "with", run)
	}
	pending := newFixtureAt(t, layout("run-1"), standardPending())
	empty := newFixtureAt(t, layout("run-2"), nil)

	fake := newFake(testCommand)
	result, err := GradeSuite(context.Background(), Config{
		SuiteDir: suite,
		Command:  testCommand,
		Timeout:  time.Minute,
		Executor: fake,
	})
	if err != nil {
		t.Fatalf("suite grade: %v", err)
	}
	if result.Total != 2 || result.Graded != 1 || result.AlreadyComplete != 1 || result.Failed != 0 {
		t.Fatalf("suite result = %+v", result)
	}
	if fake.calls != 1 {
		t.Fatalf("suite judge calls = %d", fake.calls)
	}
	if result.Runs[0].Result.RunDir != pending.dir && result.Runs[1].Result.RunDir != pending.dir {
		t.Fatal("pending run missing from suite results")
	}
	if result.Runs[0].Result.RunDir != empty.dir && result.Runs[1].Result.RunDir != empty.dir {
		t.Fatal("empty run missing from suite results")
	}

	// A second pass reuses the accepted judgement and never invokes the judge.
	again, err := GradeSuite(context.Background(), Config{
		SuiteDir: suite,
		Command:  testCommand,
		Timeout:  time.Minute,
		Executor: fake,
	})
	if err != nil {
		t.Fatalf("second suite grade: %v", err)
	}
	if again.Graded != 0 || again.AlreadyComplete != 2 || fake.calls != 1 {
		t.Fatalf("second suite pass = %+v (calls %d)", again, fake.calls)
	}
}

func TestGradeSuiteStopsAfterCancellation(t *testing.T) {
	suite := t.TempDir()
	var fixtures []*fixture
	for _, run := range []string{"run-1", "run-2", "run-3"} {
		fixtures = append(fixtures, newFixtureAt(t,
			filepath.Join(suite, "runs", "demo", "basic", "with", run), standardPending()))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fake := newFake(testCommand)
	result, err := GradeSuite(ctx, Config{
		SuiteDir: suite,
		Command:  testCommand,
		Timeout:  time.Minute,
		Executor: fake,
	})
	if errs.Code(err) != errs.CodeOperational || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("err = %v, want operational cancellation", err)
	}
	if result.Total != 0 || fake.calls != 0 {
		t.Fatalf("graded after cancellation: %+v (calls %d)", result, fake.calls)
	}
	for _, f := range fixtures {
		list, err := f.store.Enumerate()
		if err != nil {
			t.Fatal(err)
		}
		for _, attempt := range list {
			if attempt.Kind == model.KindJudge {
				t.Fatalf("judge attempt %s recorded after cancellation", attempt.ID)
			}
		}
	}
}

func TestGradeSkipsSupersededRun(t *testing.T) {
	f := newFixture(t, standardPending())
	rerun, err := f.store.Allocate(model.StartedAttempt{
		Kind: model.KindAgent, RunID: "demo/basic/with/run-1", Fingerprint: "fp",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Finish(rerun.ID, model.FinishedAttempt{
		Status: model.StatusTimedOut, StopReason: model.StopTimeout, Usage: knownAgentUsage(),
	}); err != nil {
		t.Fatal(err)
	}
	fake := newFake(testCommand)
	result, err := gradeRun(t, f, fake, testCommand, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeSuperseded || fake.calls != 0 {
		t.Fatalf("outcome = %q, judge calls = %d; want superseded with no judge call", result.Outcome, fake.calls)
	}
}

func TestRegradeWithPrunedEvidenceFails(t *testing.T) {
	f := newFixture(t, standardPending())
	if _, err := gradeRun(t, f, newFake(testCommand), testCommand, false); err != nil {
		t.Fatal(err)
	}
	pointer, _, _ := f.store.LoadCurrent()
	if _, err := f.store.PruneEvidence(f.agent.ID, []string{"agent.log"}, "test"); err != nil {
		t.Fatal(err)
	}
	fake := newFake(testCommand)
	_, err := gradeRun(t, f, fake, testCommand, true)
	if err == nil {
		t.Fatal("regrade succeeded with pruned evidence")
	}
	if code := errs.Code(err); code != 1 {
		t.Fatalf("exit code = %d, want 1 (err=%v)", code, err)
	}
	if !strings.Contains(err.Error(), "pruned") {
		t.Fatalf("error does not name pruning: %v", err)
	}
	if fake.calls != 0 {
		t.Fatalf("pruned evidence invoked judge %d times", fake.calls)
	}
	after, _, _ := f.store.LoadCurrent()
	if after.JudgeAttempt != pointer.JudgeAttempt {
		t.Fatalf("failed regrade replaced the accepted judgement: %+v", after)
	}
}

func TestIdentitySHA256ChangesWithEveryIdentityInput(t *testing.T) {
	base := CommandIdentity("judge-a")
	baseHash, err := IdentitySHA256(base)
	if err != nil {
		t.Fatal(err)
	}
	if base.Generation.Temperature != "0.0" || base.Generation.Seed != nil ||
		base.Generation.MaxOutputTokens != 0 || base.Generation.MaxSteps != 0 ||
		base.Generation.ContextWindow != 0 {
		t.Fatalf("command generation shape = %+v", base.Generation)
	}
	if base.Provider != "" || base.BaseURL != "" || base.Model != "" {
		t.Fatalf("command native fields must be empty: %+v", base)
	}

	seed := int64(7)
	mutations := map[string]func(model.JudgeIdentity) model.JudgeIdentity{
		"command": func(id model.JudgeIdentity) model.JudgeIdentity {
			id.Command = "judge-b"
			return id
		},
		"kind": func(id model.JudgeIdentity) model.JudgeIdentity {
			id.Kind = "native"
			return id
		},
		"provider": func(id model.JudgeIdentity) model.JudgeIdentity {
			id.Provider = "openai-chat"
			return id
		},
		"base_url": func(id model.JudgeIdentity) model.JudgeIdentity {
			id.BaseURL = "https://example.test"
			return id
		},
		"model": func(id model.JudgeIdentity) model.JudgeIdentity {
			id.Model = "gpt-test"
			return id
		},
		"temperature": func(id model.JudgeIdentity) model.JudgeIdentity {
			id.Generation.Temperature = "0.2"
			return id
		},
		"seed": func(id model.JudgeIdentity) model.JudgeIdentity {
			id.Generation.Seed = &seed
			return id
		},
		"max_output_tokens": func(id model.JudgeIdentity) model.JudgeIdentity {
			id.Generation.MaxOutputTokens = 1024
			return id
		},
		"max_steps": func(id model.JudgeIdentity) model.JudgeIdentity {
			id.Generation.MaxSteps = 20
			return id
		},
		"context_window": func(id model.JudgeIdentity) model.JudgeIdentity {
			id.Generation.ContextWindow = 128000
			return id
		},
		"prompt_version": func(id model.JudgeIdentity) model.JudgeIdentity {
			id.PromptVersion = "different"
			return id
		},
		"grader_version": func(id model.JudgeIdentity) model.JudgeIdentity {
			id.GraderVersion = "evalctl/test"
			return id
		},
		"schema_version": func(id model.JudgeIdentity) model.JudgeIdentity {
			id.SchemaVersion = 4
			return id
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed, err := IdentitySHA256(mutate(base))
			if err != nil {
				t.Fatal(err)
			}
			if changed == baseHash {
				t.Fatalf("%s did not change judge_identity_sha256", name)
			}
		})
	}
}

func TestGradeRefreshesResultProjection(t *testing.T) {
	f := newFixture(t, standardPending())
	if _, err := gradeRun(t, f, newFake(testCommand), testCommand, false); err != nil {
		t.Fatal(err)
	}
	readResult := func() model.RunResult {
		t.Helper()
		data, err := os.ReadFile(f.store.ResultPath())
		if err != nil {
			t.Fatal(err)
		}
		var result model.RunResult
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	pointer, _, _ := f.store.LoadCurrent()
	result := readResult()
	if result.Mechanical.Score != 3 || result.Mechanical.Max != 5 || len(result.Mechanical.Items) != 1 {
		t.Fatalf("result projection lost mechanical items: %+v", result.Mechanical)
	}
	if result.JudgeAttempt != pointer.JudgeAttempt {
		t.Fatalf("result judge attempt = %q, want %q", result.JudgeAttempt, pointer.JudgeAttempt)
	}
	if result.JudgeUsage.State != model.UsageKnown || result.JudgeUsage.InputTokens == nil ||
		*result.JudgeUsage.InputTokens != 100 {
		t.Fatalf("result judge usage = %+v", result.JudgeUsage)
	}
	if !result.CombinedUsage.Known {
		t.Fatalf("result combined usage = %+v", result.CombinedUsage)
	}
	if !result.ChecksRequired {
		t.Fatal("result projection lost checks_required")
	}

	secondCommand := "second-judge"
	second := newFake(secondCommand)
	second.run = func(req executor.AgentRunRequest) (executor.AgentRunResult, error) {
		items := []model.JudgedItem{
			{Name: "X", Score: 0, Max: 4, Note: "second"},
			{Name: "Y", Score: 0, Max: 6, Note: "second"},
		}
		data, _ := json.Marshal(items)
		if err := os.WriteFile(filepath.Join(req.Workspace, "judgement.json"), data, 0o600); err != nil {
			return executor.AgentRunResult{}, err
		}
		return okJudgeResult(knownJudgeUsage()), nil
	}
	if _, err := gradeRun(t, f, second, secondCommand, true); err != nil {
		t.Fatalf("regrade: %v", err)
	}
	pointer, _, _ = f.store.LoadCurrent()
	regraded := readResult()
	if regraded.Mechanical.Score != 3 || len(regraded.Mechanical.Items) != 1 {
		t.Fatalf("regrade lost mechanical items: %+v", regraded.Mechanical)
	}
	if regraded.JudgeAttempt != pointer.JudgeAttempt || regraded.JudgeAttempt == "" {
		t.Fatalf("regrade result judge attempt = %q, want %q", regraded.JudgeAttempt, pointer.JudgeAttempt)
	}
	if regraded.JudgeIdentitySHA256 == "" {
		t.Fatal("regrade result lost judge identity")
	}
}
