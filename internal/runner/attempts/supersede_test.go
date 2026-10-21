package attempts

import (
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

func TestSupersedes(t *testing.T) {
	finished := func(seq int, status model.RunStatus) *Attempt {
		return &Attempt{Kind: model.KindAgent, Sequence: seq, Status: status,
			Finished: &model.FinishedAttempt{Status: status}}
	}
	accepted := finished(2, model.StatusOK)
	for _, test := range []struct {
		name   string
		latest *Attempt
		want   bool
	}{
		{"no attempts", nil, false},
		{"accepted is latest", accepted, false},
		{"older failure", finished(1, model.StatusAgentFailed), false},
		{"newer ok", finished(3, model.StatusOK), false},
		{"newer unfinished", &Attempt{Kind: model.KindAgent, Sequence: 3, Status: model.StatusInterrupted}, false},
		{"newer failed", finished(3, model.StatusAgentFailed), true},
		{"newer cancelled", finished(3, model.StatusCancelled), true},
		{"newer timed out", finished(3, model.StatusTimedOut), true},
	} {
		if got := Supersedes(test.latest, accepted); got != test.want {
			t.Errorf("%s: Supersedes = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestFailedRerunSupersedesAcceptedRun(t *testing.T) {
	store := testStore(t)
	accepted := acceptedRun(t, store)
	rerun := failRerun(t, store, model.StatusAgentFailed)

	if err := store.CheckReusable("fp"); err == nil {
		t.Fatal("run with a failed rerun is still reusable")
	}
	superseding, err := store.Superseding()
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
	if superseding, err := store.Superseding(); err != nil || superseding != nil {
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
