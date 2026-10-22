package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rikdc/ai-skills/internal/judge"
	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/platform/errs"
)

func executeCommand(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func TestVersionCommand(t *testing.T) {
	stdout, _, err := executeCommand(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, model.Version) {
		t.Errorf("version output missing version: %q", stdout)
	}
	if !strings.Contains(stdout, "schema=3") {
		t.Errorf("version output missing schema: %q", stdout)
	}
	if !strings.Contains(stdout, model.GraderVersion) {
		t.Errorf("version output missing grader: %q", stdout)
	}
}

func TestVersionCommandJSON(t *testing.T) {
	stdout, _, err := executeCommand(t, "version", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Version       string `json:"version"`
		Commit        string `json:"commit"`
		SchemaVersion int    `json:"schema_version"`
		GraderVersion string `json:"grader_version"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, err)
	}
	if payload.Version != model.Version || payload.SchemaVersion != model.SchemaVersion || payload.GraderVersion != model.GraderVersion {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestVersionCommandWithoutRoot(t *testing.T) {
	stdout, _, err := executeCommand(t, "--root", filepath.Join(t.TempDir(), "missing"), "version")
	if err != nil {
		t.Fatalf("version must not require a repository root: %v", err)
	}
	if !strings.Contains(stdout, "evalctl") {
		t.Fatalf("output = %q", stdout)
	}
}

func TestDoctorCommand(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EVALCTL_ROOT", root)
	t.Setenv("EVALCTL_API_KEY", "super-secret-value")
	t.Setenv("OPENAI_API_KEY", "")

	stdout, _, err := executeCommand(t, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(root)
	if !strings.Contains(stdout, "root: "+abs) {
		t.Errorf("doctor missing root: %q", stdout)
	}
	if !strings.Contains(stdout, "bash:") {
		t.Errorf("doctor missing bash: %q", stdout)
	}
	if !strings.Contains(stdout, "api key env: EVALCTL_API_KEY=set OPENAI_API_KEY=not set") {
		t.Errorf("doctor missing api-key presence: %q", stdout)
	}
	if strings.Contains(stdout, "super-secret-value") {
		t.Fatalf("doctor leaked the API key value: %q", stdout)
	}
}

func TestDoctorRootFlagWinsOverEnv(t *testing.T) {
	flagRoot := t.TempDir()
	envRoot := t.TempDir()
	t.Setenv("EVALCTL_ROOT", envRoot)
	stdout, _, err := executeCommand(t, "--root", flagRoot, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	flagAbs, _ := filepath.Abs(flagRoot)
	if !strings.Contains(stdout, "root: "+flagAbs) {
		t.Fatalf("flag root not reported: %q", stdout)
	}
}

func TestDoctorReportsMissingRoot(t *testing.T) {
	stdout, _, err := executeCommand(t, "--root", filepath.Join(t.TempDir(), "missing"), "doctor")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "root: not found") {
		t.Fatalf("doctor output = %q", stdout)
	}
}

func TestSelftestQuick(t *testing.T) {
	stdout, _, err := executeCommand(t, "selftest", "--quick")
	if err != nil {
		t.Fatalf("selftest --quick: %v", err)
	}
	if !strings.Contains(stdout, "selftest passed") {
		t.Fatalf("selftest output = %q", stdout)
	}
	if strings.Contains(stdout, "not implemented in this increment") {
		t.Fatalf("selftest is still a stub: %q", stdout)
	}
}

func TestSelftestRejectsBadJobs(t *testing.T) {
	_, _, err := executeCommand(t, "selftest", "--quick", "--jobs", "0")
	if code := errs.Code(err); code != 2 {
		t.Fatalf("exit code = %d, want 2 (err=%v)", code, err)
	}
}

func TestMilestoneBBackendErrors(t *testing.T) {
	for _, args := range [][]string{
		{"run", "--backend", "openai", "--agent-cmd", "true"},
		{"suite", "--backend", "openai", "--agent-cmd", "true"},
	} {
		_, _, err := executeCommand(t, append([]string{"--root", t.TempDir()}, args...)...)
		if errs.Code(err) != 2 || !strings.Contains(err.Error(), "milestone B") {
			t.Fatalf("%v: err = %v (code %d)", args, err, errs.Code(err))
		}
	}
}

func TestMilestoneBJudgeErrors(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{
		{"suite", "--judge-backend", "openai", "--agent-cmd", "true"},
		{"suite", "--judge-model", "gpt-4o", "--agent-cmd", "true"},
		{"suite", "--judge-base-url", "http://localhost", "--agent-cmd", "true"},
		{"grade", "--judge-backend", "openai", "--judge-cmd", "true", "--run-dir", root},
		{"grade", "--judge-model", "gpt-4o", "--judge-cmd", "true", "--run-dir", root},
	} {
		_, _, err := executeCommand(t, append([]string{"--root", root}, args...)...)
		if errs.Code(err) != 2 || !strings.Contains(err.Error(), "milestone B") {
			t.Fatalf("%v: err = %v (code %d)", args, err, errs.Code(err))
		}
	}
}

func TestMilestoneBNativeRuntimeFlags(t *testing.T) {
	root := t.TempDir()
	for arg := range map[string]bool{
		"--base-url=http://localhost": true,
		"--api-key-env=KEY":           true,
		"--no-api-key=true":           true,
		"--temperature=0.5":           true,
		"--seed=1":                    true,
		"--max-output-tokens=1":       true,
		"--max-steps=1":               true,
		"--max-total-tokens=1":        true,
		"--context-window=1":          true,
		"--keep-recent-groups=1":      true,
		"--request-timeout=1s":        true,
		"--max-prompt-bytes=1":        true,
		"--max-tool-output-bytes=1":   true,
		"--max-archive-entries=1":     true,
		"--max-archive-path-bytes=1":  true,
	} {
		_, _, err := executeCommand(t, "--root", root, "run", arg, "--agent-cmd", "true")
		if errs.Code(err) != 2 || !strings.Contains(err.Error(), "not implemented") {
			t.Fatalf("%s: err = %v (code %d)", arg, err, errs.Code(err))
		}
	}
}

func TestMilestoneCErrors(t *testing.T) {
	root := cliRoot(t)
	for _, args := range [][]string{
		{"run", "--plugin", "beta", "--scenario", "direct", "--agent-cmd", "true"},
		{"suite", "--plugin", "beta", "--scenario", "direct", "--agent-cmd", "true", "--out", filepath.Join(t.TempDir(), "suite")},
	} {
		_, _, err := executeCommand(t, append([]string{"--root", root}, args...)...)
		if errs.Code(err) != 2 || !strings.Contains(err.Error(), "milestone C") {
			t.Fatalf("%v: err = %v (code %d)", args, err, errs.Code(err))
		}
	}
}

func TestBackendEnvFallback(t *testing.T) {
	t.Setenv("EVALCTL_BACKEND", "openai")
	for _, args := range [][]string{
		{"run", "--agent-cmd", "true"},
		{"suite", "--agent-cmd", "true"},
	} {
		_, _, err := executeCommand(t, append([]string{"--root", t.TempDir()}, args...)...)
		if errs.Code(err) != 2 || !strings.Contains(err.Error(), "milestone B") {
			t.Fatalf("%v: err = %v (code %d)", args, err, errs.Code(err))
		}
	}
}

func TestAuthPathRequiresAuthCLI(t *testing.T) {
	root := cliRoot(t)
	_, _, err := executeCommand(t, "--root", root, "run",
		"--plugin", "alpha", "--scenario", "compare", "--agent-cmd", "true",
		"--auth-path", ".claude.json")
	if errs.Code(err) != 2 || !strings.Contains(err.Error(), "--auth-path requires --auth-cli") {
		t.Fatalf("err = %v (code %d)", err, errs.Code(err))
	}
}

func TestAuthPathRejectsEscape(t *testing.T) {
	root := cliRoot(t)
	_, _, err := executeCommand(t, "--root", root, "run",
		"--plugin", "alpha", "--scenario", "compare", "--agent-cmd", "true",
		"--auth-cli", "--auth-path", "../secret")
	if errs.Code(err) != 2 || !strings.Contains(err.Error(), "must stay under the home directory") {
		t.Fatalf("err = %v (code %d)", err, errs.Code(err))
	}
}

func TestModelEnvFallback(t *testing.T) {
	root := cliRoot(t)
	t.Setenv("EVALCTL_MODEL", "selftest-model")
	out := filepath.Join(t.TempDir(), "run")
	_, stderr, err := executeCommand(t, "--root", root, "run",
		"--plugin", "alpha", "--scenario", "compare", "--arm", "with",
		"--agent-cmd", "true", "--out", out)
	if err != nil {
		t.Fatalf("run: %v (stderr %s)", err, stderr)
	}
	data, err := os.ReadFile(filepath.Join(out, "alpha", "compare", "with", "run-1", "attempts", "agent-000001", "started.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"model": "selftest-model"`) {
		t.Fatalf("started.json did not record EVALCTL_MODEL: %s", data)
	}
}

func TestAttemptArtifactsValidation(t *testing.T) {
	root := cliRoot(t)
	_, _, err := executeCommand(t, "--root", root, "run",
		"--plugin", "alpha", "--scenario", "compare", "--agent-cmd", "true",
		"--attempt-artifacts", "sometimes")
	if errs.Code(err) != 2 || !strings.Contains(err.Error(), "--attempt-artifacts") {
		t.Fatalf("err = %v (code %d)", err, errs.Code(err))
	}
}

func TestInvalidLogFormat(t *testing.T) {
	_, _, err := executeCommand(t, "--log-format", "yaml", "version")
	if code := errs.Code(err); code != 2 {
		t.Fatalf("exit code = %d, want 2 (err=%v)", code, err)
	}
}

func TestUnknownCommandExitTwo(t *testing.T) {
	err := Execute(context.Background(), []string{"nope"})
	if code := errs.Code(err); code != 2 {
		t.Fatalf("exit code = %d, want 2 (err=%v)", code, err)
	}
}

func TestUnknownFlagExitTwo(t *testing.T) {
	_, _, err := executeCommand(t, "--nope", "version")
	if code := errs.Code(err); code != 2 {
		t.Fatalf("exit code = %d, want 2 (err=%v)", code, err)
	}
}

func TestRootHelp(t *testing.T) {
	stdout, _, err := executeCommand(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"run", "suite", "grade", "report", "list", "plan", "check", "selftest", "doctor", "version"} {
		if !strings.Contains(stdout, name) {
			t.Errorf("help missing %q", name)
		}
	}
}

func TestExecuteReturnsNilOnSuccess(t *testing.T) {
	if err := Execute(context.Background(), []string{"version"}); err != nil {
		t.Fatalf("Execute(version) = %v", err)
	}
}

func TestPositionalArgErrorsExitTwo(t *testing.T) {
	for _, args := range [][]string{
		{"report"},
		{"report", "a", "b"},
		{"run", "extra"},
		{"version", "extra"},
		{"completion", "bash", "extra"},
	} {
		err := Execute(context.Background(), args)
		if code := errs.Code(err); code != 2 {
			t.Errorf("%v: exit code = %d, want 2 (err=%v)", args, code, err)
		}
	}
}

func TestUnknownCommandSuggests(t *testing.T) {
	err := Execute(context.Background(), []string{"verison"})
	if code := errs.Code(err); code != 2 {
		t.Fatalf("exit code = %d, want 2 (err=%v)", code, err)
	}
	if !strings.Contains(err.Error(), `unknown command "verison" for "evalctl"`) ||
		!strings.Contains(err.Error(), "Did you mean this?\n\tversion") {
		t.Fatalf("err = %q", err)
	}
}

func TestBareRootPrintsHelp(t *testing.T) {
	stdout, _, err := executeCommand(t)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "Available Commands") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestVersionWithoutWorkingDirectory(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	gone := t.TempDir()
	if err := os.Chdir(gone); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EVALCTL_ROOT", "")

	if _, _, err := executeCommand(t, "version"); err != nil {
		t.Fatalf("version must not need a working directory: %v", err)
	}
	stdout, _, _ := executeCommand(t, "doctor")
	if !strings.Contains(stdout, "root: not found") {
		t.Fatalf("doctor stdout = %q, want missing root", stdout)
	}
}

func TestEmptyLogFormatRejected(t *testing.T) {
	_, _, err := executeCommand(t, "--log-format", "", "version")
	if code := errs.Code(err); code != 2 {
		t.Fatalf("exit code = %d, want 2 (err=%v)", code, err)
	}
}

func TestGradeSummaryLine(t *testing.T) {
	plain := gradeSummaryLine(judge.SuiteResult{Total: 3, Graded: 2, AlreadyComplete: 1})
	if plain != "Grading: 3, graded: 2, already complete: 1, failed: 0" {
		t.Fatalf("line = %q", plain)
	}
	with := gradeSummaryLine(judge.SuiteResult{Total: 4, Graded: 2, AlreadyComplete: 1, Superseded: 1})
	if with != "Grading: 4, graded: 2, already complete: 1, superseded: 1, failed: 0" {
		t.Fatalf("line = %q", with)
	}
}
