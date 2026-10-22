package attempts

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rikdc/ai-skills/internal/model"
)

var errBoom = errors.New("injected crash")

func testStore(t *testing.T) *Store {
	t.Helper()
	return New(t.TempDir())
}

func int64p(v int64) *int64 { return &v }

func knownUsage() model.Usage {
	return model.Usage{
		State:       model.UsageKnown,
		Known:       true,
		Source:      "test",
		InputTokens: int64p(10),
		TotalTokens: int64p(15),
	}
}

func judgeUsage() model.Usage {
	return model.Usage{
		State:       model.UsageKnown,
		Known:       true,
		Source:      "test-judge",
		InputTokens: int64p(4),
		TotalTokens: int64p(4),
	}
}

func mustAllocate(t *testing.T, store *Store, kind model.AttemptKind, fingerprint string) *Attempt {
	t.Helper()
	attempt, err := store.Allocate(model.StartedAttempt{
		Kind:         kind,
		RunID:        "plugin/scenario/with/1",
		Fingerprint:  fingerprint,
		ConfigSHA256: "config-1",
		Executor:     "command",
		Model:        "test-model",
	})
	if err != nil {
		t.Fatalf("allocate %s: %v", kind, err)
	}
	return attempt
}

func mustFinish(t *testing.T, store *Store, id string, finished model.FinishedAttempt) *Attempt {
	t.Helper()
	attempt, err := store.Finish(id, finished)
	if err != nil {
		t.Fatalf("finish %s: %v", id, err)
	}
	return attempt
}

// addExecutionArtifacts writes the graded artifacts, the canonical evidence
// manifest, and the grading request for an execution attempt.
func addExecutionArtifacts(t *testing.T, store *Store, attempt *Attempt) (ev string, reqSHA string) {
	t.Helper()
	return addExecutionArtifactsWithLog(t, store, attempt, "agent output\n")
}

// addExecutionArtifactsWithLog is addExecutionArtifacts with caller-chosen
// agent log content; an empty log omits agent.log from the evidence.
func addExecutionArtifactsWithLog(t *testing.T, store *Store, attempt *Attempt, log string) (ev string, reqSHA string) {
	t.Helper()
	evidence := []string{"snapshot.tar", "mechanical.json"}
	if log != "" {
		if err := os.WriteFile(filepath.Join(attempt.Dir, "agent.log"), []byte(log), 0o600); err != nil {
			t.Fatal(err)
		}
		evidence = []string{"snapshot.tar", "agent.log", "mechanical.json"}
	}
	if err := os.WriteFile(filepath.Join(attempt.Dir, "mechanical.json"), []byte(`{"score":1,"max":2,"items":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attempt.Dir, "snapshot.tar"), []byte("tar bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	var err error
	ev, err = store.WriteEvidenceManifest(attempt.ID, evidence)
	if err != nil {
		t.Fatalf("write evidence: %v", err)
	}
	request := model.GradingRequest{
		SchemaVersion:  model.SchemaVersion,
		Plugin:         "plugin",
		Scenario:       "scenario",
		Arm:            "with",
		Run:            1,
		AgentAttempt:   attempt.ID,
		EvidenceSHA256: ev,
		Criteria: model.Rubric{
			Type: "absolute",
			Checklist: []model.Criterion{
				{Name: "quality", Description: "quality", MaxScore: 1},
			},
		},
		CoveredByChecks:  []string{},
		JudgementPending: []model.Criterion{},
	}
	reqSHA, err = store.WriteGradingRequest(request)
	if err != nil {
		t.Fatalf("write grading request: %v", err)
	}
	return ev, reqSHA
}

func finishExecutionOK(t *testing.T, store *Store, attempt *Attempt, ev, reqSHA string) *Attempt {
	t.Helper()
	finished := model.FinishedAttempt{
		Status:               model.StatusOK,
		StopReason:           model.StopCompleted,
		Usage:                knownUsage(),
		ChecksStatus:         model.ChecksOK,
		MechanicalSHA256:     "mechanical-digest",
		EvidenceSHA256:       ev,
		GradingRequestSHA256: reqSHA,
	}
	return mustFinish(t, store, attempt.ID, finished)
}

func finishJudge(t *testing.T, store *Store, agentID, ev, reqSHA, identity string) *Attempt {
	t.Helper()
	judge := mustAllocate(t, store, model.KindJudge, "fingerprint-1")
	judgement, err := json.Marshal([]model.JudgedItem{{Name: "quality", Score: 1, Max: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(judge.Dir, "judgement.json"), judgement, 0o600); err != nil {
		t.Fatal(err)
	}
	return mustFinish(t, store, judge.ID, model.FinishedAttempt{
		Status:               model.StatusOK,
		Usage:                judgeUsage(),
		ChecksStatus:         model.ChecksNotApplicable,
		GradedAgentAttempt:   agentID,
		GradedEvidenceSHA256: ev,
		GradingRequestSHA256: reqSHA,
		JudgeIdentitySHA256:  identity,
		JudgementSHA256:      SHA256Hex(judgement),
	})
}

func acceptedExecution(t *testing.T, store *Store) (*Attempt, string) {
	t.Helper()
	attempt := mustAllocate(t, store, model.KindAgent, "fingerprint-1")
	ev, reqSHA := addExecutionArtifacts(t, store, attempt)
	attempt = finishExecutionOK(t, store, attempt, ev, reqSHA)
	if _, err := store.Accept(attempt.ID, AcceptOptions{AcceptedBy: "evalctl/test"}); err != nil {
		t.Fatalf("accept: %v", err)
	}
	return attempt, ev
}

func TestAllocateWritesStartedBeforeReturn(t *testing.T) {
	store := testStore(t)
	attempt := mustAllocate(t, store, model.KindAgent, "fp")

	if attempt.ID != "agent-000001" {
		t.Fatalf("attempt id = %q", attempt.ID)
	}
	if attempt.Status != model.StatusInterrupted {
		t.Fatalf("in-progress status = %q", attempt.Status)
	}
	data, err := os.ReadFile(filepath.Join(attempt.Dir, "started.json"))
	if err != nil {
		t.Fatalf("started.json: %v", err)
	}
	var started model.StartedAttempt
	if err := json.Unmarshal(data, &started); err != nil {
		t.Fatal(err)
	}
	if started.AttemptID != attempt.ID || started.Kind != model.KindAgent {
		t.Fatalf("started record = %+v", started)
	}
	if started.SchemaVersion != model.SchemaVersion || started.StartedAt == "" {
		t.Fatalf("started metadata = %+v", started)
	}
	if _, err := os.Stat(filepath.Join(attempt.Dir, "finished.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("finished.json should not exist: %v", err)
	}
	usage := attempt.EffectiveUsage()
	if usage.State != model.UsageUnknown || usage.Known {
		t.Fatalf("interrupted usage = %+v", usage)
	}
	if _, _, err := store.LoadCurrent(); !errors.Is(err, ErrNoCurrent) {
		t.Fatalf("current after allocate = %v", err)
	}
}

func TestAllocateSharesOneSequenceAcrossKinds(t *testing.T) {
	store := testStore(t)
	agent := mustAllocate(t, store, model.KindAgent, "fp")
	judge := mustAllocate(t, store, model.KindJudge, "fp")
	hook := mustAllocate(t, store, model.KindHook, "fp")
	if agent.ID != "agent-000001" || judge.ID != "judge-000002" || hook.ID != "hook-000003" {
		t.Fatalf("ids = %q, %q, %q", agent.ID, judge.ID, hook.ID)
	}
}

func TestFinishIsImmutable(t *testing.T) {
	store := testStore(t)
	attempt := mustAllocate(t, store, model.KindAgent, "fp")
	finished := mustFinish(t, store, attempt.ID, model.FinishedAttempt{
		Status: model.StatusAgentFailed,
		Usage:  knownUsage(),
	})
	if finished.Status != model.StatusAgentFailed {
		t.Fatalf("status = %q", finished.Status)
	}
	_, err := store.Finish(attempt.ID, model.FinishedAttempt{Status: model.StatusOK, Usage: knownUsage()})
	if !errors.Is(err, ErrAlreadyFinished) {
		t.Fatalf("second finish error = %v, want ErrAlreadyFinished", err)
	}
	reloaded, err := store.LoadAttempt(attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Finished == nil || reloaded.Finished.Status != model.StatusAgentFailed {
		t.Fatalf("finished record changed: %+v", reloaded.Finished)
	}
	if _, err := store.Finish("agent-000099", model.FinishedAttempt{Status: model.StatusOK}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("finish without started = %v", err)
	}
}

func TestFinishRejectsDerivedInterruptedStatus(t *testing.T) {
	store := testStore(t)
	attempt := mustAllocate(t, store, model.KindAgent, "fp")
	_, err := store.Finish(attempt.ID, model.FinishedAttempt{Status: model.StatusInterrupted})
	if !errors.Is(err, ErrInvalidAttempt) {
		t.Fatalf("error = %v", err)
	}
}

func TestEnumerateCountsEachAttemptOnceAndIgnoresStrayDirectories(t *testing.T) {
	store := testStore(t)
	agent := mustAllocate(t, store, model.KindAgent, "fp")
	hook := mustAllocate(t, store, model.KindHook, "fp")
	if err := os.Mkdir(filepath.Join(store.AttemptsDir(), "agent-000009"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.AttemptsDir(), "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustFinish(t, store, agent.ID, model.FinishedAttempt{Status: model.StatusOK, Usage: knownUsage()})

	enumerated, err := store.Enumerate()
	if err != nil {
		t.Fatal(err)
	}
	if len(enumerated) != 2 {
		t.Fatalf("attempt count = %d, want 2", len(enumerated))
	}
	if enumerated[0].ID != agent.ID || enumerated[1].ID != hook.ID {
		t.Fatalf("order = %q, %q", enumerated[0].ID, enumerated[1].ID)
	}
	if enumerated[0].Status != model.StatusOK || enumerated[1].Status != model.StatusInterrupted {
		t.Fatalf("statuses = %q, %q", enumerated[0].Status, enumerated[1].Status)
	}
}

func TestFinishedAttemptHasNoAcceptanceFields(t *testing.T) {
	record := model.FinishedAttempt{SchemaVersion: 3, AttemptID: "agent-000001", Kind: model.KindAgent, Status: model.StatusOK}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{"accepted", "superseded_by"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("finished record contains %q: %s", forbidden, text)
		}
	}
}

func TestIndexIsDerivedDedupedAndNotAuthoritative(t *testing.T) {
	store := testStore(t)
	agent := mustAllocate(t, store, model.KindAgent, "fp")
	mustFinish(t, store, agent.ID, model.FinishedAttempt{Status: model.StatusOK, Usage: knownUsage()})
	judge := mustAllocate(t, store, model.KindJudge, "fp")
	mustFinish(t, store, judge.ID, model.FinishedAttempt{Status: model.StatusOK, Usage: knownUsage()})

	entries, err := store.RebuildIndex()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("index entries = %d", len(entries))
	}
	if err := os.WriteFile(store.IndexPath(), []byte(`{"attempt_id":"agent-000001","kind":"agent","sequence":1,"status":"ok","started_at":"x"}
{"attempt_id":"agent-000001","kind":"agent","sequence":1,"status":"ok","started_at":"x"}
not-json
`), 0o600); err != nil {
		t.Fatal(err)
	}
	read, err := store.ReadIndex()
	if err != nil {
		t.Fatal(err)
	}
	if len(read) != 1 || read[0].AttemptID != agent.ID {
		t.Fatalf("deduped index = %+v", read)
	}

	before, err := store.Enumerate()
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 2 {
		t.Fatalf("index affected enumeration: %d", len(before))
	}
	if _, err := store.RebuildIndex(); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := store.ReadIndex()
	if err != nil {
		t.Fatal(err)
	}
	if len(rebuilt) != 2 {
		t.Fatalf("rebuilt index = %+v", rebuilt)
	}
}

func TestEvidenceManifestCanonicalAndTamperDetected(t *testing.T) {
	store := testStore(t)
	attempt := mustAllocate(t, store, model.KindAgent, "fp")
	ev, _ := addExecutionArtifacts(t, store, attempt)
	finishExecutionOK(t, store, attempt, ev, "request-digest")

	attempt, err := store.LoadAttempt(attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyEvidence(attempt); err != nil {
		t.Fatalf("verify evidence: %v", err)
	}
	manifest, raw, err := ReadEvidenceManifest(attempt.Dir)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := CanonicalJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(canonical) {
		t.Fatalf("evidence.json is not canonical: %s", raw)
	}
	if SHA256Hex(raw) != ev {
		t.Fatalf("evidence digest mismatch")
	}
	wantPaths := []string{"agent.log", "mechanical.json", "snapshot.tar"}
	var gotPaths []string
	for _, file := range manifest.Files {
		gotPaths = append(gotPaths, file.Path)
	}
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("manifest paths = %v, want %v", gotPaths, wantPaths)
	}

	if err := os.WriteFile(filepath.Join(attempt.Dir, "agent.log"), []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyEvidence(attempt); err == nil {
		t.Fatal("tampered evidence verified")
	}
}

func TestInvalidCurrentIsNeverAccepted(t *testing.T) {
	store := testStore(t)
	if err := os.WriteFile(store.CurrentPath(), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LoadCurrent(); !errors.Is(err, ErrInvalidCurrent) {
		t.Fatalf("load current = %v", err)
	}
	if store.Reusable("fp") {
		t.Fatal("invalid current reported reusable")
	}
	if _, err := store.EnsureResultProjection(RunContext{}); !errors.Is(err, ErrInvalidCurrent) {
		t.Fatalf("projection error = %v", err)
	}
}

func TestAllocateFailureWhenAttemptsDirectoryIsReadOnly(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	store := testStore(t)
	if err := os.MkdirAll(store.AttemptsDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(store.AttemptsDir(), 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(store.AttemptsDir(), 0o700)
	if _, err := store.Allocate(model.StartedAttempt{Kind: model.KindAgent}); err == nil {
		t.Fatal("allocate succeeded in read-only attempts directory")
	}
}
