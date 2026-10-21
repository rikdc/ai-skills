package attempts

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/rikdc/ai-skills/internal/model"
)

func TestCrashAfterStartedBeforeLaunch(t *testing.T) {
	store := testStore(t)
	store.Hooks.AfterAllocate = func(string) error { return errBoom }
	if _, err := store.Allocate(model.StartedAttempt{
		Kind:         model.KindAgent,
		Fingerprint:  "fp",
		ConfigSHA256: "config-1",
		Executor:     "command",
	}); !errors.Is(err, errBoom) {
		t.Fatalf("allocate error = %v", err)
	}
	store.Hooks.AfterAllocate = nil

	attempts, err := store.Enumerate()
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 {
		t.Fatalf("attempts = %d, want the started attempt", len(attempts))
	}
	attempt := attempts[0]
	if attempt.Status != model.StatusInterrupted || !attempt.Interrupted() {
		t.Fatalf("status = %q", attempt.Status)
	}
	if usage := attempt.EffectiveUsage(); usage.State != model.UsageUnknown || usage.Known {
		t.Fatalf("usage = %+v", usage)
	}
	if store.Reusable("fp") {
		t.Fatal("interrupted attempt reported reusable")
	}
	if _, _, err := store.LoadCurrent(); !errors.Is(err, ErrNoCurrent) {
		t.Fatalf("current = %v", err)
	}
}

func TestCrashDuringExecutionLeavesInterruptedAttempt(t *testing.T) {
	store := testStore(t)
	attempt := mustAllocate(t, store, model.KindAgent, "fp")
	if err := os.WriteFile(filepath.Join(attempt.Dir, "agent.log"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.LoadAttempt(attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != model.StatusInterrupted {
		t.Fatalf("status = %q", reloaded.Status)
	}
	if err := VerifyEvidence(reloaded); err == nil {
		t.Fatal("partial evidence verified")
	}
	if _, err := store.Accept(attempt.ID, AcceptOptions{}); err == nil {
		t.Fatal("accepted interrupted attempt")
	}
	if _, changed, err := store.Adopt(AdoptOptions{Fingerprint: "fp"}); err != nil {
		t.Fatal(err)
	} else if changed {
		t.Fatal("interrupted attempt was adopted")
	}
	if store.Reusable("fp") {
		t.Fatal("interrupted attempt reusable")
	}
}

func TestCrashAfterFinishedBeforePointerAdoptsWithoutRerun(t *testing.T) {
	store := testStore(t)
	attempt := mustAllocate(t, store, model.KindAgent, "fp")
	ev, reqSHA := addExecutionArtifacts(t, store, attempt)
	finishExecutionOK(t, store, attempt, ev, reqSHA)

	if _, _, err := store.LoadCurrent(); !errors.Is(err, ErrNoCurrent) {
		t.Fatalf("current before adoption = %v", err)
	}
	if store.Reusable("fp") {
		t.Fatal("finished but unaccepted attempt reported reusable")
	}
	before, _ := store.Enumerate()

	pointer, changed, err := store.Adopt(AdoptOptions{Fingerprint: "fp", AcceptedBy: "manual-adopt/test"})
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if !changed || pointer.AgentAttempt != attempt.ID {
		t.Fatalf("adoption = %+v changed=%v", pointer, changed)
	}
	if pointer.AgentEvidenceSHA256 != ev || pointer.AgentGradingRequestSHA256 != reqSHA {
		t.Fatalf("pointer bindings = %+v", pointer)
	}
	if pointer.AcceptedBy != "manual-adopt/test" {
		t.Fatalf("accepted_by = %q", pointer.AcceptedBy)
	}
	after, _ := store.Enumerate()
	if len(before) != len(after) {
		t.Fatalf("adoption double-counted attempts: %d -> %d", len(before), len(after))
	}
	if err := store.CheckReusable("fp"); err != nil {
		t.Fatalf("adopted attempt not reusable: %v", err)
	}
	if store.Reusable("different") {
		t.Fatal("fingerprint mismatch reported reusable")
	}

	again, changed, err := store.Adopt(AdoptOptions{Fingerprint: "fp"})
	if err != nil {
		t.Fatal(err)
	}
	if changed || again.AgentAttempt != attempt.ID {
		t.Fatalf("second adoption changed=%v pointer=%+v", changed, again)
	}

	// A crash after finished but before projections: rebuild them.
	if _, err := os.Stat(store.ResultPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("result projection unexpectedly present: %v", err)
	}
	result, err := store.EnsureResultProjection(RunContext{Plugin: "plugin", Scenario: "scenario", Arm: "with", Mode: "absolute", Run: 1, ChecksRequired: true})
	if err != nil {
		t.Fatalf("result projection: %v", err)
	}
	if !result.Complete || result.AgentAttempt != attempt.ID || result.Status != model.StatusOK {
		t.Fatalf("result = %+v", result)
	}
	if result.CurrentSHA256 == "" {
		t.Fatal("result has no current_sha256")
	}
}

func TestAdoptionChoosesHighestMatchingAttempt(t *testing.T) {
	store := testStore(t)
	var matching []*Attempt
	for i := 0; i < 3; i++ {
		attempt := mustAllocate(t, store, model.KindAgent, "fp")
		ev, reqSHA := addExecutionArtifacts(t, store, attempt)
		matching = append(matching, finishExecutionOK(t, store, attempt, ev, reqSHA))
	}
	stale := mustAllocate(t, store, model.KindAgent, "other")
	ev, reqSHA := addExecutionArtifacts(t, store, stale)
	stale = finishExecutionOK(t, store, stale, ev, reqSHA)

	pointer, changed, err := store.Adopt(AdoptOptions{Fingerprint: "fp"})
	if err != nil {
		t.Fatal(err)
	}
	if !changed || pointer.AgentAttempt != matching[2].ID {
		t.Fatalf("adopted %q, want highest matching %q", pointer.AgentAttempt, matching[2].ID)
	}
	// The stale attempt is retained and still counted.
	all, err := store.Enumerate()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("attempt count = %d", len(all))
	}
}

func TestAdoptionSkipsFinishedNonOK(t *testing.T) {
	store := testStore(t)
	attempt := mustAllocate(t, store, model.KindAgent, "fp")
	usage := knownUsage()
	mustFinish(t, store, attempt.ID, model.FinishedAttempt{
		Status: model.StatusAgentFailed,
		Usage:  usage,
	})
	pointer, changed, err := store.Adopt(AdoptOptions{Fingerprint: "fp"})
	if err != nil {
		t.Fatal(err)
	}
	if changed || pointer != nil {
		t.Fatalf("adopted failed attempt: %+v", pointer)
	}
	all, _ := store.Enumerate()
	if len(all) != 1 || all[0].Finished.Status != model.StatusAgentFailed {
		t.Fatalf("failed attempt not retained: %+v", all)
	}
	got := all[0].EffectiveUsage()
	if got.State != model.UsageKnown || got.TotalTokens == nil || *got.TotalTokens != 15 {
		t.Fatalf("usage = %+v", got)
	}
}

func TestJudgeAdoptedInSamePointerTransition(t *testing.T) {
	store := testStore(t)
	agent := mustAllocate(t, store, model.KindAgent, "fp")
	ev, reqSHA := addExecutionArtifacts(t, store, agent)
	finishExecutionOK(t, store, agent, ev, reqSHA)
	judge := finishJudge(t, store, agent.ID, ev, reqSHA, "identity-1")

	pointer, changed, err := store.Adopt(AdoptOptions{Fingerprint: "fp", JudgeIdentitySHA256: "identity-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !changed || pointer.AgentAttempt != agent.ID || pointer.JudgeAttempt != judge.ID {
		t.Fatalf("pointer = %+v", pointer)
	}
	if pointer.GradedAgentAttempt != agent.ID || pointer.GradedEvidenceSHA256 != ev ||
		pointer.GradingRequestSHA256 != reqSHA || pointer.JudgeIdentitySHA256 != "identity-1" {
		t.Fatalf("judge binding fields = %+v", pointer)
	}
	if err := store.CheckReusable("fp"); err != nil {
		t.Fatalf("reusable: %v", err)
	}
}

func TestAcceptRejectsJudgeWithMismatchedBinding(t *testing.T) {
	store := testStore(t)
	agent := mustAllocate(t, store, model.KindAgent, "fp")
	ev, reqSHA := addExecutionArtifacts(t, store, agent)
	finishExecutionOK(t, store, agent, ev, reqSHA)
	judge := finishJudge(t, store, agent.ID, ev, reqSHA, "identity-1")

	judge, err := store.LoadAttempt(judge.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Rewrite finished.json with a different binding digest. The record is
	// immutable in normal operation; this simulates an unrelated/stale judge.
	judge.Finished.GradedEvidenceSHA256 = "different"
	data, err := marshalIndented(judge.Finished)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(judge.Dir, "finished.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Accept(agent.ID, AcceptOptions{JudgeAttempt: judge.ID}); err == nil {
		t.Fatal("accepted judge with mismatched binding")
	}
	if _, _, err := store.LoadCurrent(); !errors.Is(err, ErrNoCurrent) {
		t.Fatalf("pointer written despite rejected judge: %v", err)
	}
}

func TestPointerCrashBeforeRenameKeepsOldPointer(t *testing.T) {
	store := testStore(t)
	first, _ := acceptedExecution(t, store)
	second := mustAllocate(t, store, model.KindAgent, "fingerprint-1")
	ev, reqSHA := addExecutionArtifacts(t, store, second)
	finishExecutionOK(t, store, second, ev, reqSHA)

	store.Hooks.BeforePointerRename = func() error { return errBoom }
	if _, err := store.Accept(second.ID, AcceptOptions{}); !errors.Is(err, errBoom) {
		t.Fatalf("accept error = %v", err)
	}
	store.Hooks.BeforePointerRename = nil

	pointer, _, err := store.LoadCurrent()
	if err != nil {
		t.Fatal(err)
	}
	if pointer.AgentAttempt != first.ID {
		t.Fatalf("pointer = %q, want old %q", pointer.AgentAttempt, first.ID)
	}
	// The finished attempt is still unaccepted and adoptable.
	if _, changed, err := store.Adopt(AdoptOptions{Fingerprint: "fingerprint-1"}); err != nil {
		t.Fatal(err)
	} else if !changed {
		t.Fatal("finished attempt not adopted after pre-rename crash")
	}
}

func TestPointerCrashAfterRenameLeavesNewPointer(t *testing.T) {
	store := testStore(t)
	acceptedExecution(t, store)
	second := mustAllocate(t, store, model.KindAgent, "fingerprint-1")
	ev, reqSHA := addExecutionArtifacts(t, store, second)
	finishExecutionOK(t, store, second, ev, reqSHA)

	store.Hooks.AfterPointerRename = func() error { return errBoom }
	if _, err := store.Accept(second.ID, AcceptOptions{}); !errors.Is(err, errBoom) {
		t.Fatalf("accept error = %v", err)
	}
	store.Hooks.AfterPointerRename = nil

	pointer, _, err := store.LoadCurrent()
	if err != nil {
		t.Fatal(err)
	}
	if pointer.AgentAttempt != second.ID {
		t.Fatalf("pointer = %q, want new %q", pointer.AgentAttempt, second.ID)
	}
	if err := store.CheckReusable("fingerprint-1"); err != nil {
		t.Fatalf("reusable after post-rename crash: %v", err)
	}
}

func TestBetweenAgentAndJudgeAttempts(t *testing.T) {
	store := testStore(t)
	agent, ev := acceptedExecution(t, store)
	judge := mustAllocate(t, store, model.KindJudge, "fingerprint-1")

	// The accepted execution remains current while the judge is interrupted.
	pointer, _, err := store.LoadCurrent()
	if err != nil {
		t.Fatal(err)
	}
	if pointer.AgentAttempt != agent.ID || pointer.JudgeAttempt != "" {
		t.Fatalf("pointer = %+v", pointer)
	}
	if err := store.CheckReusable("fingerprint-1"); err != nil {
		t.Fatalf("agent-only acceptance should be reusable: %v", err)
	}
	judge, err = store.LoadAttempt(judge.ID)
	if err != nil {
		t.Fatal(err)
	}
	if judge.Status != model.StatusInterrupted {
		t.Fatalf("judge status = %q", judge.Status)
	}
	if usage := judge.EffectiveUsage(); usage.State != model.UsageUnknown || usage.Known {
		t.Fatalf("interrupted judge usage = %+v", usage)
	}
	if _, err := store.Accept(agent.ID, AcceptOptions{JudgeAttempt: judge.ID}); err == nil {
		t.Fatal("accepted interrupted judge")
	}

	// Rerun the judge only; the agent attempt is untouched.
	judgeRetry := finishJudge(t, store, agent.ID, ev, pointer.AgentGradingRequestSHA256, "identity-1")
	updated, err := store.Accept(agent.ID, AcceptOptions{JudgeAttempt: judgeRetry.ID})
	if err != nil {
		t.Fatalf("accept judge retry: %v", err)
	}
	if updated.AgentAttempt != agent.ID || updated.JudgeAttempt != judgeRetry.ID {
		t.Fatalf("updated pointer = %+v", updated)
	}
	all, err := store.Enumerate()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("attempt count = %d, want agent + interrupted judge + judge retry", len(all))
	}
}

func TestCurrentWinsOverStaleResultProjection(t *testing.T) {
	store := testStore(t)
	attempt, _ := acceptedExecution(t, store)
	ctx := RunContext{Plugin: "plugin", Scenario: "scenario", Arm: "with", Mode: "absolute", Run: 1}

	first, err := store.EnsureResultProjection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.ResultPath(), []byte(`{"status":"missing","complete":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := store.EnsureResultProjection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, rebuilt) {
		t.Fatalf("rebuild differs:\n%+v\n%+v", first, rebuilt)
	}
	raw, err := os.ReadFile(store.ResultPath())
	if err != nil {
		t.Fatal(err)
	}
	var onDisk model.RunResult
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatal(err)
	}
	pointer, currentRaw, err := store.LoadCurrent()
	if err != nil {
		t.Fatal(err)
	}
	if onDisk.AgentAttempt != attempt.ID || onDisk.AgentAttempt != pointer.AgentAttempt {
		t.Fatalf("result attempt = %q", onDisk.AgentAttempt)
	}
	if onDisk.CurrentSHA256 != SHA256Hex(currentRaw) {
		t.Fatalf("current_sha256 = %q, want %q", onDisk.CurrentSHA256, SHA256Hex(currentRaw))
	}
	if onDisk.Usage.TotalTokens == nil || onDisk.AgentUsage.TotalTokens == nil {
		t.Fatalf("usage aliases missing: %+v", onDisk)
	}
	if *onDisk.Usage.TotalTokens != *onDisk.AgentUsage.TotalTokens {
		t.Fatalf("usage alias differs from agent_usage")
	}
	if onDisk.Mechanical.Max != 2 || onDisk.Mechanical.Score != 1 {
		t.Fatalf("mechanical projection = %+v", onDisk.Mechanical)
	}
}

func TestGradingRequestProjectionRebuiltCanonically(t *testing.T) {
	store := testStore(t)
	attempt, ev := acceptedExecution(t, store)
	pointer, _, err := store.LoadCurrent()
	if err != nil {
		t.Fatal(err)
	}
	request, raw, digest, err := store.ReadGradingRequest()
	if err != nil {
		t.Fatal(err)
	}
	if digest != pointer.AgentGradingRequestSHA256 {
		t.Fatalf("request digest %q does not match pointer %q", digest, pointer.AgentGradingRequestSHA256)
	}
	canonical, err := CanonicalJSON(request)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(canonical) {
		t.Fatal("grading request is not canonical")
	}
	if request.AgentAttempt != attempt.ID || request.EvidenceSHA256 != ev {
		t.Fatalf("request binding = %+v", request)
	}

	// Crash after pointer before projection: delete and rebuild.
	if err := os.Remove(store.GradingRequestPath()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureGradingRequestProjection(request); err != nil {
		t.Fatalf("rebuild request: %v", err)
	}
	_, rebuilt, rebuiltDigest, err := store.ReadGradingRequest()
	if err != nil {
		t.Fatal(err)
	}
	if rebuiltDigest != digest || string(rebuilt) != string(raw) {
		t.Fatal("rebuilt request differs")
	}

	// Stale content must not be treated as an accepted binding.
	staleRequest := request
	staleRequest.AgentAttempt = attempt.ID
	staleRequest.CoveredByChecks = []string{"quality"}
	if _, err := store.EnsureGradingRequestProjection(staleRequest); err == nil {
		t.Fatal("stale grading request accepted")
	}
	staleBytes, err := CanonicalJSON(staleRequest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.GradingRequestPath(), staleBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if store.Reusable("fingerprint-1") {
		t.Fatal("stale grading-request projection reported reusable")
	}
	if _, err := store.EnsureGradingRequestProjection(request); err != nil {
		t.Fatal(err)
	}
	if !store.Reusable("fingerprint-1") {
		t.Fatal("rebuilt grading-request projection not reusable")
	}
}

func TestPruningMarkerKeepsMetadataAndUsage(t *testing.T) {
	store := testStore(t)
	attempt, ev := acceptedExecution(t, store)
	if err := os.WriteFile(filepath.Join(attempt.Dir, "usage.json"), []byte(`{"state":"known"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	marker, err := store.PruneEvidence(attempt.ID, []string{"snapshot.tar", "agent.log"}, "retention")
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if marker.EvidenceSHA256 != ev {
		t.Fatalf("marker original evidence digest = %q, want %q", marker.EvidenceSHA256, ev)
	}
	if !reflect.DeepEqual(marker.Paths, []string{"agent.log", "snapshot.tar"}) {
		t.Fatalf("marker paths = %v", marker.Paths)
	}
	for _, name := range []string{"started.json", "finished.json", "usage.json", "evidence.json", "evidence_pruned.json"} {
		if _, err := os.Stat(filepath.Join(attempt.Dir, name)); err != nil {
			t.Fatalf("%s removed: %v", name, err)
		}
	}
	for _, name := range []string{"agent.log", "snapshot.tar"} {
		if _, err := os.Stat(filepath.Join(attempt.Dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s not pruned: %v", name, err)
		}
	}
	reloaded, err := store.LoadAttempt(attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyEvidence(reloaded); err != nil {
		t.Fatalf("pruned evidence should still verify: %v", err)
	}
	if !store.Reusable("fingerprint-1") {
		t.Fatal("pruned accepted attempt not reusable")
	}
	all, _ := store.Enumerate()
	if len(all) != 1 {
		t.Fatalf("pruned attempt count = %d", len(all))
	}

	if _, err := store.PruneEvidence(attempt.ID, []string{"usage.json"}, "bad"); err == nil {
		t.Fatal("pruned usage.json")
	}
	if _, err := store.PruneEvidence(attempt.ID, []string{"../outside"}, "bad"); err == nil {
		t.Fatal("pruned escaping path")
	}
}

func TestAdoptRequiresFingerprint(t *testing.T) {
	store := testStore(t)
	if _, _, err := store.Adopt(AdoptOptions{}); err == nil {
		t.Fatal("adopt without fingerprint succeeded")
	}
}

func TestFinishWriteFailpoints(t *testing.T) {
	store := testStore(t)
	attempt := mustAllocate(t, store, model.KindAgent, "fp")

	store.Hooks.BeforeFinishWrite = func(string) error { return errBoom }
	if _, err := store.Finish(attempt.ID, model.FinishedAttempt{Status: model.StatusAgentFailed, Usage: knownUsage()}); !errors.Is(err, errBoom) {
		t.Fatalf("before-finish error = %v", err)
	}
	store.Hooks.BeforeFinishWrite = nil
	reloaded, err := store.LoadAttempt(attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Interrupted() {
		t.Fatalf("attempt not interrupted after pre-finish crash: %+v", reloaded.Finished)
	}

	store.Hooks.AfterFinishWrite = func(string) error { return errBoom }
	if _, err := store.Finish(attempt.ID, model.FinishedAttempt{Status: model.StatusOK, Usage: knownUsage()}); !errors.Is(err, errBoom) {
		t.Fatalf("after-finish error = %v", err)
	}
	store.Hooks.AfterFinishWrite = nil
	// The immutable outcome exists even though the caller saw a crash.
	reloaded, err = store.LoadAttempt(attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Finished == nil || reloaded.Finished.Status != model.StatusOK {
		t.Fatalf("finished record = %+v", reloaded.Finished)
	}
}

func TestProjectionCrashKeepsPointer(t *testing.T) {
	store := testStore(t)
	attempt, _ := acceptedExecution(t, store)
	ctx := RunContext{Plugin: "plugin", Scenario: "scenario", Arm: "with", Mode: "absolute", Run: 1}
	store.Hooks.BeforeProjectionWrite = func(string) error { return errBoom }
	if _, err := store.EnsureResultProjection(ctx); !errors.Is(err, errBoom) {
		t.Fatalf("projection error = %v", err)
	}
	store.Hooks.BeforeProjectionWrite = nil
	if _, err := os.Stat(store.ResultPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("projection written despite crash hook: %v", err)
	}
	pointer, _, err := store.LoadCurrent()
	if err != nil {
		t.Fatal(err)
	}
	if pointer.AgentAttempt != attempt.ID {
		t.Fatalf("pointer lost: %+v", pointer)
	}
	if _, err := store.EnsureResultProjection(ctx); err != nil {
		t.Fatalf("projection rebuild: %v", err)
	}
}

func TestResultProjectionIncludesAcceptedJudge(t *testing.T) {
	store := testStore(t)
	agent := mustAllocate(t, store, model.KindAgent, "fp")
	ev, reqSHA := addExecutionArtifacts(t, store, agent)
	finishExecutionOK(t, store, agent, ev, reqSHA)
	judge := finishJudge(t, store, agent.ID, ev, reqSHA, "identity-1")
	if _, err := store.Accept(agent.ID, AcceptOptions{JudgeAttempt: judge.ID}); err != nil {
		t.Fatalf("accept: %v", err)
	}

	result, err := store.EnsureResultProjection(RunContext{
		Plugin: "plugin", Scenario: "scenario", Arm: "with", Mode: "absolute", Run: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.JudgeAttempt != judge.ID || result.JudgeIdentitySHA256 != "identity-1" {
		t.Fatalf("judge projection = %+v", result)
	}
	if result.JudgeUsage.State != model.UsageKnown {
		t.Fatalf("judge usage = %+v", result.JudgeUsage)
	}
	if result.CombinedUsage.State != model.UsageKnown || result.CombinedUsage.TotalTokens == nil ||
		*result.CombinedUsage.TotalTokens != 19 {
		t.Fatalf("combined usage = %+v", result.CombinedUsage)
	}
	if result.AgentUsage.TotalTokens == nil || *result.AgentUsage.TotalTokens != 15 {
		t.Fatalf("agent usage = %+v", result.AgentUsage)
	}
}

func TestWriteCurrentValidatesAndHashes(t *testing.T) {
	store := testStore(t)
	attempt, _ := acceptedExecution(t, store)
	pointer, _, err := store.LoadCurrent()
	if err != nil {
		t.Fatal(err)
	}
	pointer.AcceptedBy = "manual-adopt/test"
	if err := store.WriteCurrent(pointer); err != nil {
		t.Fatalf("write current: %v", err)
	}
	reloaded, raw, err := store.LoadCurrent()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.AcceptedBy != "manual-adopt/test" || reloaded.AgentAttempt != attempt.ID {
		t.Fatalf("pointer = %+v", reloaded)
	}
	digest, err := store.CurrentSHA256()
	if err != nil {
		t.Fatal(err)
	}
	if digest != SHA256Hex(raw) {
		t.Fatalf("digest %q does not hash current bytes", digest)
	}

	invalid := model.CurrentPointer{SchemaVersion: model.SchemaVersion, AgentAttempt: attempt.ID}
	if err := store.WriteCurrent(invalid); !errors.Is(err, ErrInvalidCurrent) {
		t.Fatalf("invalid pointer error = %v", err)
	}
}
