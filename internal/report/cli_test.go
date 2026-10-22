package report_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rikdc/ai-skills/internal/cli"
	"github.com/rikdc/ai-skills/internal/platform/errs"
)

func writeFixture(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeCLISuite(t *testing.T, dir string, withScore, withoutScore float64) {
	t.Helper()
	writeFixture(t, filepath.Join(dir, "manifest.json"), map[string]any{
		"version": 1, "created_at": "2026-01-02T03:04:05Z",
		"config": map[string]any{
			"agent_cmd": "claude -p --model fixture-model", "arm": "both", "runs": 1,
			"units": []string{"skills/fixture"}, "git_sha": "deadbee",
		},
		"scenarios": []map[string]any{{
			"plugin": "demo", "scenario": "case", "units": []string{"skills/fixture"}, "mode": "comparison",
		}},
	})
	writeFixture(t, filepath.Join(dir, "suite.json"), map[string]any{
		"counts": map[string]any{"agent_invocations": 2, "duration_seconds": 20.0},
	})
	if err := os.WriteFile(filepath.Join(dir, "results.jsonl"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	rubric := map[string]any{
		"context": "x", "type": "weighted_checklist",
		"checklist": []map[string]any{{"name": "A", "description": "d", "max_score": 100}},
	}
	writeFixture(t, filepath.Join(dir, "criteria", "demo", "case.json"), rubric)
	for arm, score := range map[string]float64{"with": withScore, "without": withoutScore} {
		runDir := filepath.Join(dir, "runs", "demo", "case", arm, "run-1")
		writeFixture(t, filepath.Join(runDir, "mechanical.json"),
			[]map[string]any{{"name": "A", "score": score, "max": 100}})
		writeFixture(t, filepath.Join(runDir, "grading-request.json"), map[string]any{
			"criteria": rubric, "judgement_pending": []any{},
		})
		writeFixture(t, filepath.Join(runDir, "result.json"), map[string]any{
			"agent_exit": 0, "timed_out": false, "duration_seconds": 10.0,
		})
		writeFixture(t, filepath.Join(runDir, "judge-result.json"), map[string]any{"duration_seconds": 4.0})
	}
}

func execute(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := cli.NewRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func TestReportCommandJSONAndAll(t *testing.T) {
	suite := t.TempDir()
	makeCLISuite(t, suite, 100, 90)

	stdout, _, err := execute(t, "report", suite, "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Verdict struct {
			State string `json:"state"`
		} `json:"verdict"`
		Totals struct {
			With float64 `json:"with"`
		} `json:"totals"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if doc.Verdict.State != "indicative" || doc.Totals.With != 100 {
		t.Fatalf("report = %+v", doc)
	}
	if _, err := os.Stat(filepath.Join(suite, "report", "report.json")); err != nil {
		t.Fatalf("report.json not written: %v", err)
	}

	if _, _, err := execute(t, "report", suite, "--format", "all"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"report.json", "report.md", "report.html"} {
		if _, err := os.Stat(filepath.Join(suite, "report", name)); err != nil {
			t.Fatalf("%s not written: %v", name, err)
		}
	}
	htmlDoc, err := os.ReadFile(filepath.Join(suite, "report", "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(htmlDoc), "Change since baseline") {
		t.Fatal("baseline section rendered without a baseline")
	}
}

func TestReportCommandBaseline(t *testing.T) {
	baseline := t.TempDir()
	current := t.TempDir()
	makeCLISuite(t, baseline, 100, 90)
	makeCLISuite(t, current, 100, 95)
	stdout, _, err := execute(t, "report", current, "--format", "json", "--baseline", baseline)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Comparison struct {
			Status string `json:"status"`
			Delta  struct {
				Effect *float64 `json:"effect"`
			} `json:"delta"`
		} `json:"comparison"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Comparison.Status != "comparable" || doc.Comparison.Delta.Effect == nil || *doc.Comparison.Delta.Effect != -5 {
		t.Fatalf("comparison = %+v", doc.Comparison)
	}
}

func TestReportCommandErrors(t *testing.T) {
	_, _, err := execute(t, "report", filepath.Join(t.TempDir(), "missing"))
	if err == nil || errs.Code(err) != errs.CodeOperational {
		t.Fatalf("missing suite error = %v code %d", err, errs.Code(err))
	}
	suite := t.TempDir()
	makeCLISuite(t, suite, 100, 90)
	_, _, err = execute(t, "report", suite, "--format", "yaml")
	if err == nil || errs.Code(err) != errs.CodeUsage {
		t.Fatalf("bad format error = %v code %d", err, errs.Code(err))
	}

	// A --out that cannot be created is an error and leaves no partial report
	// behind.
	unwritable := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(unwritable, []byte("file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(unwritable, "child")
	_, _, err = execute(t, "report", suite, "--format", "json", "--out", out)
	if err == nil || errs.Code(err) != errs.CodeOperational {
		t.Fatalf("unwritable --out error = %v code %d", err, errs.Code(err))
	}
	if !strings.Contains(err.Error(), "cannot create report directory") {
		t.Fatalf("unwritable --out message = %v", err)
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Fatal("a partial report directory was created for an unwritable --out")
	}
	if info, statErr := os.Stat(unwritable); statErr != nil || !info.Mode().IsRegular() {
		t.Fatalf("the blocking file was replaced: %v", statErr)
	}
}

func TestReportCommandStreamsPerFormat(t *testing.T) {
	suite := t.TempDir()
	makeCLISuite(t, suite, 100, 90)

	stdout, stderr, err := execute(t, "report", suite)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "Eval report:") || !strings.Contains(stdout, "Report directory:") {
		t.Fatalf("cli stdout = %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("cli stderr = %q", stderr)
	}
	if _, err := os.Stat(filepath.Join(suite, "report", "report.json")); err != nil {
		t.Fatalf("cli did not always write report.json: %v", err)
	}

	stdout, stderr, err = execute(t, "report", suite, "--format", "md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "# Eval report:") {
		t.Fatalf("md stdout = %q", stdout)
	}
	if !strings.Contains(stderr, filepath.Join(suite, "report", "report.md")) {
		t.Fatalf("md stderr = %q", stderr)
	}

	stdout, stderr, err = execute(t, "report", suite, "--format", "html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, filepath.Join(suite, "report", "report.html")) {
		t.Fatalf("html stdout = %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("html stderr = %q", stderr)
	}

	stdout, stderr, err = execute(t, "report", suite, "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.TrimSpace(stdout), "{") {
		t.Fatalf("json stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "report.json") {
		t.Fatalf("json stderr = %q", stderr)
	}
}
