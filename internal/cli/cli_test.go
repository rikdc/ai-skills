package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

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

func TestStubCommandsExitTwo(t *testing.T) {
	for _, name := range []string{"run", "suite", "grade", "selftest"} {
		t.Run(name, func(t *testing.T) {
			_, _, err := executeCommand(t, name)
			if err == nil {
				t.Fatalf("%s: expected an error", name)
			}
			if code := errs.Code(err); code != 2 {
				t.Fatalf("%s: exit code = %d, want 2", name, code)
			}
			if !strings.Contains(err.Error(), "not implemented in this increment") {
				t.Fatalf("%s: message = %q", name, err)
			}
		})
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
