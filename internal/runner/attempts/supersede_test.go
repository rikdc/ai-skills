package attempts

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rikdc/ai-skills/internal/model"
)

// acceptedRun returns a store whose newest attempt is accepted and reusable.
func acceptedRun(t *testing.T, store *Store) *Attempt {
	t.Helper()
	attempt := mustAllocate(t, store, model.KindAgent, "fp")
	ev, reqSHA := addExecutionArtifacts(t, store, attempt)
	attempt = finishExecutionOK(t, store, attempt, ev, reqSHA)
	if _, _, err := store.Adopt(AdoptOptions{Fingerprint: "fp"}); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckReusable("fp"); err != nil {
		t.Fatalf("accepted run not reusable: %v", err)
	}
	return attempt
}

func failRerun(t *testing.T, store *Store, status model.RunStatus) *Attempt {
	t.Helper()
	rerun := mustAllocate(t, store, model.KindAgent, "fp")
	return mustFinish(t, store, rerun.ID, model.FinishedAttempt{
		Status:     status,
		StopReason: model.StopProviderError,
		Usage:      knownUsage(),
		Error:      "rerun failed",
	})
}

func TestSupersedingAttempt(t *testing.T) {
	finished := func(seq int, status model.RunStatus) *Attempt {
		return &Attempt{Kind: model.KindAgent, Sequence: seq, Status: status,
			Finished: &model.FinishedAttempt{Status: status}}
	}
	unfinished := func(seq int) *Attempt {
		return &Attempt{Kind: model.KindAgent, Sequence: seq, Status: model.StatusInterrupted}
	}
	judge := &Attempt{Kind: model.KindJudge, Sequence: 3, Finished: &model.FinishedAttempt{Status: model.StatusAgentFailed}}
	accepted := finished(2, model.StatusOK)
	for _, test := range []struct {
		name string
		list []*Attempt
		want int // sequence of the superseding attempt, 0 for none
	}{
		{"accepted only", []*Attempt{accepted}, 0},
		{"older failure", []*Attempt{finished(1, model.StatusAgentFailed), accepted}, 0},
		{"newer ok", []*Attempt{accepted, finished(3, model.StatusOK)}, 0},
		{"newer unfinished", []*Attempt{accepted, unfinished(3)}, 0},
		{"newer judge failure", []*Attempt{accepted, judge}, 0},
		{"newer failed", []*Attempt{accepted, finished(3, model.StatusAgentFailed)}, 3},
		{"newer cancelled", []*Attempt{accepted, finished(3, model.StatusCancelled)}, 3},
		{"newer timed out", []*Attempt{accepted, finished(3, model.StatusTimedOut)}, 3},
		{"crash after a failed rerun", []*Attempt{accepted, finished(3, model.StatusAgentFailed), unfinished(4)}, 3},
		{"ok after a failed rerun", []*Attempt{accepted, finished(3, model.StatusAgentFailed), finished(4, model.StatusOK)}, 0},
	} {
		got := SupersedingAttempt(test.list, accepted)
		switch {
		case test.want == 0 && got != nil:
			t.Errorf("%s: superseded by %d, want none", test.name, got.Sequence)
		case test.want != 0 && (got == nil || got.Sequence != test.want):
			t.Errorf("%s: superseded by %v, want %d", test.name, got, test.want)
		}
	}
	if SupersedingAttempt([]*Attempt{finished(3, model.StatusAgentFailed)}, nil) != nil {
		t.Error("no accepted attempt cannot be superseded")
	}
}

func TestFailedRerunSupersedesAcceptedRun(t *testing.T) {
	store := testStore(t)
	accepted := acceptedRun(t, store)
	rerun := failRerun(t, store, model.StatusAgentFailed)

	if err := store.CheckReusable("fp"); err == nil {
		t.Fatal("run with a failed rerun is still reusable")
	}
	superseding, err := store.Superseded()
	if err != nil || superseding == nil || superseding.ID != rerun.ID {
		t.Fatalf("Superseding = %v, %v; want %s", superseding, err, rerun.ID)
	}
	// Adoption must not restore the older ok attempt behind the failure.
	if _, changed, err := store.Adopt(AdoptOptions{Fingerprint: "fp"}); err != nil || changed {
		t.Fatalf("adopt after failed rerun: changed=%v err=%v", changed, err)
	}
	pointer, _, err := store.LoadCurrent()
	if err != nil || pointer.AgentAttempt != accepted.ID {
		t.Fatalf("current = %+v, %v; want the accepted pointer kept as provenance", pointer, err)
	}
}

func TestUnfinishedRerunDoesNotSupersede(t *testing.T) {
	store := testStore(t)
	acceptedRun(t, store)
	mustAllocate(t, store, model.KindAgent, "fp")

	if err := store.CheckReusable("fp"); err != nil {
		t.Fatalf("an in-flight or crashed rerun made the run non-reusable: %v", err)
	}
	if superseding, err := store.Superseded(); err != nil || superseding != nil {
		t.Fatalf("Superseding = %v, %v; want none", superseding, err)
	}
}

func TestCorruptOlderAttemptDoesNotBlockReuse(t *testing.T) {
	store := testStore(t)
	older := mustAllocate(t, store, model.KindAgent, "fp")
	acceptedRun(t, store)
	if err := os.WriteFile(filepath.Join(older.Dir, "started.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckReusable("fp"); err != nil {
		t.Fatalf("a corrupt older attempt blocked reuse: %v", err)
	}
}

func TestEmptyNewerAttemptDirectoryDoesNotBlockReuse(t *testing.T) {
	store := testStore(t)
	accepted := acceptedRun(t, store)
	// A crash during allocation can leave an attempt directory with no
	// started.json; it is not an attempt.
	if err := os.MkdirAll(filepath.Join(store.AttemptsDir(), formatAttemptID(model.KindAgent, accepted.Sequence+1)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckReusable("fp"); err != nil {
		t.Fatalf("an empty attempt directory blocked reuse: %v", err)
	}
}

func TestCrashAfterFailedRerunStillSupersedes(t *testing.T) {
	store := testStore(t)
	acceptedRun(t, store)
	failed := failRerun(t, store, model.StatusAgentFailed)
	mustAllocate(t, store, model.KindAgent, "fp")

	if err := store.CheckReusable("fp"); err == nil {
		t.Fatal("a crashed attempt hid the failed rerun")
	}
	if superseding, err := store.Superseded(); err != nil || superseding == nil || superseding.ID != failed.ID {
		t.Fatalf("Superseded = %v, %v; want %s", superseding, err, failed.ID)
	}
}

func TestCorruptOlderFinishedRecordDoesNotBlockReuse(t *testing.T) {
	store := testStore(t)
	older := failRerun(t, store, model.StatusAgentFailed)
	acceptedRun(t, store)
	if err := os.WriteFile(filepath.Join(older.Dir, "finished.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckReusable("fp"); err != nil {
		t.Fatalf("a corrupt older finished.json blocked reuse: %v", err)
	}
	if superseding, err := store.Superseded(); err != nil || superseding != nil {
		t.Fatalf("Superseded = %v, %v; want none", superseding, err)
	}
}

func TestCorruptNewerRerunIsAnErrorNotAnOutcomeChange(t *testing.T) {
	store := testStore(t)
	acceptedRun(t, store)
	rerun := failRerun(t, store, model.StatusAgentFailed)
	if err := os.WriteFile(filepath.Join(rerun.Dir, "finished.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The damaged attempt may be the run's outcome, so nothing may quietly
	// fall back to the accepted one.
	if err := store.CheckReusable("fp"); err == nil {
		t.Fatal("a damaged newer rerun left the accepted run reusable")
	}
	if _, err := store.Superseded(); err == nil {
		t.Fatal("Superseded ignored a damaged newer rerun")
	}
	before, _, _ := store.LoadCurrent()
	if _, _, err := store.Adopt(AdoptOptions{Fingerprint: "fp"}); err == nil {
		t.Fatal("Adopt ignored a damaged newer rerun")
	}
	if after, _, _ := store.LoadCurrent(); after.AgentAttempt != before.AgentAttempt {
		t.Fatalf("current.json repointed from %s to %s", before.AgentAttempt, after.AgentAttempt)
	}
}

func TestResultProjectionIsNotRebuiltFromSupersededPointer(t *testing.T) {
	store := testStore(t)
	acceptedRun(t, store)
	failRerun(t, store, model.StatusAgentFailed)
	_, err := store.EnsureResultProjection(RunContext{Plugin: "plugin", Scenario: "scenario", Arm: "with", Run: 1})
	if !errors.Is(err, ErrSuperseded) {
		t.Fatalf("EnsureResultProjection = %v, want ErrSuperseded", err)
	}
	if _, err := os.Stat(store.ResultPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("result.json written from a superseded pointer: %v", err)
	}
}
