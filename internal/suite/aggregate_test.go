package suite

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rikdc/ai-skills/internal/model"
)

// pendingStats builds the aggregate a summary needs: one scenario whose runs
// still carry pending criteria.
func pendingStats(runs ...RunRow) aggregateStats {
	return aggregateStats{
		Counts: map[string]int{"pending_runs": len(runs)},
		Scenarios: []ScenarioRow{{
			Plugin:   "git-workflow",
			Scenario: "pr",
			Status:   "ok",
			Runs:     runs,
		}},
	}
}

// unjudgedRun is an accepted run whose criteria were never judged.
func unjudgedRun() RunRow {
	return RunRow{Arm: "with", Run: 1, Status: string(model.StatusOK), Pending: 3}
}

// failedRun is a run with no accepted attempt, so grading cannot judge it.
func failedRun() RunRow {
	return RunRow{Arm: "with", Run: 2, Status: string(model.StatusTimedOut), TimedOut: true, Pending: 5}
}

func TestRenderSummaryGradeHintMatchesCredentialMode(t *testing.T) {
	stats := pendingStats(unjudgedRun())

	plain := renderSummary(model.SuiteManifest{}, stats, "/suite")
	if !strings.Contains(plain, "`evalctl grade --suite <suite>`") {
		t.Fatalf("summary lacks the grade hint:\n%s", plain)
	}
	if strings.Contains(plain, "--auth-cli") {
		t.Fatalf("summary suggests --auth-cli without credentials:\n%s", plain)
	}

	credentialed := renderSummary(model.SuiteManifest{Config: model.SuiteConfig{AuthCLI: true}}, stats, "/suite")
	if !strings.Contains(credentialed, "`evalctl grade --suite <suite> --auth-cli`") {
		t.Fatalf("summary omits --auth-cli for a credentialed suite:\n%s", credentialed)
	}
}

func TestRenderSummaryPendingHintMatchesCause(t *testing.T) {
	manifest := model.SuiteManifest{Config: model.SuiteConfig{
		Runs:           1,
		Arm:            "both",
		AgentCmd:       "claude -p",
		TimeoutSeconds: 900,
	}}

	retryOnly := renderSummary(manifest, pendingStats(failedRun()), "/suite")
	if !strings.Contains(retryOnly, "no attempt was accepted") {
		t.Fatalf("a run without an accepted attempt must get the retry hint:\n%s", retryOnly)
	}
	if !strings.Contains(retryOnly, "--resume-policy retry-failed") {
		t.Fatalf("retry hint lacks the resume policy:\n%s", retryOnly)
	}
	if strings.Contains(retryOnly, "Grade them") {
		t.Fatalf("a timed-out run must not be reported as merely ungraded:\n%s", retryOnly)
	}

	gradeOnly := renderSummary(manifest, pendingStats(unjudgedRun()), "/suite")
	if !strings.Contains(gradeOnly, "Grade them") {
		t.Fatalf("an accepted but unjudged run must get the grade hint:\n%s", gradeOnly)
	}
	if strings.Contains(gradeOnly, "no attempt was accepted") {
		t.Fatalf("an accepted run must not be reported as needing a retry:\n%s", gradeOnly)
	}

	both := renderSummary(manifest, pendingStats(failedRun(), unjudgedRun()), "/suite")
	if !strings.Contains(both, "no attempt was accepted") || !strings.Contains(both, "Grade them") {
		t.Fatalf("both pending causes must be reported:\n%s", both)
	}
	// The counts are per cause, not the combined pending-run total.
	if !strings.Contains(both, "**1 run(s) have pending criteria because no attempt was accepted**") ||
		!strings.Contains(both, "**1 run(s) still have pending criteria.**") {
		t.Fatalf("pending hints do not count their own cause:\n%s", both)
	}
}

func TestRenderSummaryRetryCommandRoundTrips(t *testing.T) {
	manifest := model.SuiteManifest{
		Config: model.SuiteConfig{
			Runs:           2,
			Arm:            "with",
			AgentCmd:       `opencode run --auto --format json "$(cat)"`,
			TimeoutSeconds: 900,
			AuthCLI:        true,
			Units:          []string{"skills/pr"},
			Judge:          model.JudgeConfig{Cmd: "claude -p don't-stop"},
			AuthPaths:      []string{".config/opencode"},
		},
		Scenarios: []model.PlannedScenario{
			{Plugin: "git-workflow", Scenario: "pr"},
			{Plugin: "git-workflow", Scenario: "pr-hard"},
			{Plugin: "nix-skills", Scenario: "nix-expert-secrets-review"},
		},
	}
	suiteDir := filepath.Join(t.TempDir(), "suite with 'quote'")

	command := retryCommand(manifest, suiteDir)
	want := []string{
		"suite", "--resume", "--resume-policy", "retry-failed", "--out", suiteDir,
		"--plugin", "git-workflow", "--plugin", "nix-skills",
		"--unit", "skills/pr", "--runs", "2", "--arm", "with",
		"--agent-cmd", `opencode run --auto --format json "$(cat)"`,
		"--judge-cmd", "claude -p don't-stop",
		"--auth-cli", "--auth-path", ".config/opencode",
		"--timeout", "900s",
	}
	if !strings.HasPrefix(command, "bin/evalctl ") {
		t.Fatalf("retry command must start with the repo binary: %s", command)
	}

	// Run the rendered command through a stub bin/evalctl that echoes its argv,
	// so the shell has to reproduce every value byte-for-byte.
	root := t.TempDir()
	stub := filepath.Join(root, "bin", "evalctl")
	if err := os.MkdirAll(filepath.Dir(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nfor arg in \"$@\"; do printf '%s\\n' \"$arg\"; done\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rendered retry command does not run: %v\n%s", err, command)
	}
	got := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(got) != len(want) {
		t.Fatalf("retry command argv = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q\ncommand: %s", i, got[i], want[i], command)
		}
	}
}
