package suite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rikdc/ai-skills/internal/judge"
)

// gradeAfterRun reproduces the staleness: run a suite with no judge, then grade
// it with the standalone grade pass.
func gradeAfterRun(t *testing.T, fx *fixture, out string) {
	t.Helper()
	if _, _, err := runSuite(t, fx.suiteConfig(t, out)); err != nil {
		t.Fatalf("suite failed: %v", err)
	}
	result, err := judge.GradeSuite(context.Background(), judge.Config{
		SuiteDir: out,
		Command:  fx.judgeScript,
		Timeout:  30 * time.Second,
		Executor: fx.judgeExecutor(t, fx.judgeScript),
	})
	if err != nil || result.Graded != 3 {
		t.Fatalf("grade: %+v, %v", result, err)
	}
}

func TestSummaryAndCountsAreStaleUntilRefreshedAfterGrading(t *testing.T) {
	fx := newFixture(t)
	out := filepath.Join(t.TempDir(), "suite")
	gradeAfterRun(t, fx, out)

	// The artifacts written at run time predate the grading.
	stale := readSuiteJSON(t, out)
	if stale.Counts["judged_runs"] != 0 || stale.Counts["pending_runs"] != 3 {
		t.Fatalf("precondition: counts should still be pre-grade, got %+v", stale.Counts)
	}

	if err := Refresh(fx.root, out); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	doc := readSuiteJSON(t, out)
	want := map[string]int{
		"judged_runs":       3,
		"pending_runs":      0,
		"judge_attempts":    3,
		"total_attempts":    6,
		"agent_attempts":    3,
		"failed_attempts":   0,
		"agent_invocations": 3,
		"planned":           2,
		"executed":          2,
	}
	for key, value := range want {
		if doc.Counts[key] != value {
			t.Errorf("counts[%s] = %d, want %d (all: %+v)", key, doc.Counts[key], value, doc.Counts)
		}
	}
	if doc.DurationSeconds != stale.DurationSeconds || doc.Counts["duration_seconds"] != stale.Counts["duration_seconds"] {
		t.Errorf("refresh must keep the suite's recorded duration: %v vs %v", doc.DurationSeconds, stale.DurationSeconds)
	}

	summary, err := os.ReadFile(filepath.Join(out, "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(summary)
	if strings.Contains(text, "still have pending criteria") || !strings.Contains(text, "| 0 pending |") {
		t.Errorf("summary still reports pending work:\n%s", text)
	}
	if !strings.Contains(text, "| compare | 100% | 100% |") {
		t.Errorf("summary lacks the graded scores:\n%s", text)
	}
	if !strings.Contains(text, "Judged runs: 3") || !strings.Contains(text, "Pending runs: 0") {
		t.Errorf("summary header is stale:\n%s", text)
	}

	for _, row := range readResults(t, out) {
		if row.PendingCriteria != 0 {
			t.Errorf("results row %s still pending: %d", row.Scenario, row.PendingCriteria)
		}
	}
	index, err := os.ReadFile(filepath.Join(out, "grading", "index.tsv"))
	if err != nil || strings.Count(string(index), "\n") != 1 {
		t.Errorf("grading index should list no pending runs: %v\n%s", err, index)
	}
}

func TestRefreshKeepsFailedAttemptCounts(t *testing.T) {
	fx := newFixture(t)
	out := filepath.Join(t.TempDir(), "suite")
	cfg := fx.suiteConfig(t, out)
	cfg.JudgeCmd = "exit 1"
	cfg.JudgeExec = fx.judgeExecutor(t, "echo boom >&2; exit 1")
	_, _, _ = runSuite(t, cfg)
	before := readSuiteJSON(t, out)
	if before.Counts["failed_attempts"] == 0 {
		t.Fatalf("precondition: failing judge should leave failed attempts: %+v", before.Counts)
	}

	if err := Refresh(fx.root, out); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	after := readSuiteJSON(t, out)
	for _, key := range []string{"failed_attempts", "judge_attempts", "total_attempts", "planned", "executed"} {
		if after.Counts[key] != before.Counts[key] {
			t.Errorf("counts[%s] changed on refresh: %d -> %d", key, before.Counts[key], after.Counts[key])
		}
	}
}

func TestRefreshRejectsADirectoryThatIsNotASuite(t *testing.T) {
	err := Refresh(t.TempDir(), t.TempDir())
	if err == nil || !IsNotSuite(err) {
		t.Fatalf("err = %v, want a not-a-suite error", err)
	}
}

func TestRefreshRefusesALockedSuite(t *testing.T) {
	fx := newFixture(t)
	out := filepath.Join(t.TempDir(), "suite")
	if _, _, err := runSuite(t, fx.suiteConfig(t, out)); err != nil {
		t.Fatal(err)
	}
	release, err := lockFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := Refresh(fx.root, out); err == nil {
		t.Fatal("refresh must not write while another writer holds the suite")
	}
}
