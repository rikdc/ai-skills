package report_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/rikdc/ai-skills/internal/report"
	"github.com/rikdc/ai-skills/internal/reportio"
)

// TestShellV1Parity compares the Go report with scripts/eval-report.sh on a
// shell-produced v1 suite. The fixture under testdata/v1suite is a trimmed
// capture of a real shell run: manifest, suite.json, results.jsonl, rubric and
// the accepted run artifacts.
func TestShellV1Parity(t *testing.T) {
	suiteDir, err := filepath.Abs(filepath.Join("testdata", "v1suite"))
	if err != nil {
		t.Fatal(err)
	}
	shellDoc := shellReport(t, suiteDir)

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reportio.Load(suiteDir, reportio.Options{RepoRoot: repoRoot})
	if err != nil {
		t.Fatal(err)
	}
	goDoc := toJSONMap(t, report.Build(loaded))

	want := semanticReport(t, shellDoc)
	got := semanticReport(t, goDoc)
	if diffs := semanticDiff(got, want, ""); len(diffs) > 0 {
		t.Fatalf("Go report and shell report differ semantically: %v", diffs)
	}
}

// shellReport runs scripts/eval-report.sh --format json and returns the
// document. The test skips when the shell harness or jq is unavailable.
func shellReport(t *testing.T, suiteDir string) map[string]any {
	t.Helper()
	script := filepath.Join("..", "..", "scripts", "eval-report.sh")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("shell report is not available: %v", err)
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq is required to run the shell report")
	}
	shellDir := t.TempDir()
	cmd := exec.Command(script, suiteDir, "--format", "json", "--no-color", "--out", shellDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shell report failed: %v\n%s", err, output)
	}
	shellBytes, err := os.ReadFile(filepath.Join(shellDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var shellDoc map[string]any
	if err := json.Unmarshal(shellBytes, &shellDoc); err != nil {
		t.Fatal(err)
	}
	return shellDoc
}

func toJSONMap(t *testing.T, value any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// semanticReport projects both documents onto the v1 report contract. v3
// additions (expenditure, accepted usage aliases, version) are intentionally
// excluded.
func semanticReport(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	out := map[string]any{
		"metadata":  pick(t, doc, "metadata", "label", "units", "agent_cmd", "judge_cmd", "model", "git_sha", "git_dirty", "created_at", "arm", "n", "agent_invocations", "counts"),
		"totals":    pick(t, doc, "totals", "with", "without", "delta", "n", "functional", "functional_with", "pending_points", "pending_criteria", "runner_failures", "failed_scenarios", "failed_runs", "results_failures", "not_comparable", "with_min", "with_max", "without_min", "without_max", "coverage_percent", "scored_criteria_cells", "expected_criteria_cells", "won", "lost", "tied", "regressions"),
		"timing":    pick(t, doc, "timing", "wall_seconds", "agent_seconds", "judge_seconds", "runs", "timed_out_runs"),
		"cost":      doc["cost"],
		"verdict":   doc["verdict"],
		"criteria":  doc["criteria"],
		"scenarios": semanticScenarios(t, doc["scenarios"]),
	}
	return dropNil(out).(map[string]any)
}

// dropNil removes null object members. The shell report emits explicit nulls
// for unavailable values; the Go model uses omitempty. Both mean "not
// measured", so they are excluded from the semantic comparison.
func dropNil(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if item == nil {
				continue
			}
			out[key] = dropNil(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = dropNil(item)
		}
		return out
	default:
		return value
	}
}

func pick(t *testing.T, doc map[string]any, section string, keys ...string) map[string]any {
	t.Helper()
	raw, ok := doc[section].(map[string]any)
	if !ok {
		t.Fatalf("%s is not an object", section)
	}
	out := make(map[string]any, len(keys))
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		out[key] = value
	}
	return out
}

func semanticScenarios(t *testing.T, raw any) []any {
	t.Helper()
	scenarios, ok := raw.([]any)
	if !ok {
		t.Fatalf("scenarios is not an array")
	}
	out := make([]any, 0, len(scenarios))
	for _, item := range scenarios {
		sc := item.(map[string]any)
		copy := make(map[string]any, len(sc))
		for key, value := range sc {
			switch key {
			case "runs":
				copy[key] = semanticRuns(t, value)
			default:
				copy[key] = value
			}
		}
		out = append(out, copy)
	}
	return out
}

func semanticRuns(t *testing.T, raw any) []any {
	t.Helper()
	runs, ok := raw.([]any)
	if !ok {
		t.Fatalf("runs is not an array")
	}
	additive := map[string]bool{
		"agent_usage": true, "combined_usage": true, "complete": true,
		"max": true, "current_sha256": true, "stop_reason": true,
		"error": true, "agent_attempt": true, "judge_attempt": true,
	}
	out := make([]any, 0, len(runs))
	for _, item := range runs {
		run := item.(map[string]any)
		copy := make(map[string]any, len(run))
		for key, value := range run {
			if additive[key] {
				continue
			}
			switch key {
			case "usage", "judge_usage":
				copy[key] = stripUsageState(value)
			default:
				copy[key] = value
			}
		}
		out = append(out, copy)
	}
	return out
}

func stripUsageState(raw any) any {
	usage, ok := raw.(map[string]any)
	if !ok {
		return raw
	}
	out := make(map[string]any, len(usage))
	for key, value := range usage {
		switch key {
		case "state", "reason":
			continue
		default:
			out[key] = value
		}
	}
	return out
}

func semanticDiff(got, want any, path string) []string {
	switch g := got.(type) {
	case map[string]any:
		w, ok := want.(map[string]any)
		if !ok {
			return []string{path + ": type mismatch"}
		}
		var out []string
		for key, value := range g {
			if other, ok := w[key]; ok {
				out = append(out, semanticDiff(value, other, path+"/"+key)...)
			} else {
				out = append(out, path+"/"+key+": only in Go")
			}
		}
		for key := range w {
			if _, ok := g[key]; !ok {
				out = append(out, path+"/"+key+": only in shell")
			}
		}
		return out
	case []any:
		w, ok := want.([]any)
		if !ok {
			return []string{path + ": type mismatch"}
		}
		if len(g) != len(w) {
			return []string{path + ": length mismatch"}
		}
		var out []string
		for i := range g {
			out = append(out, semanticDiff(g[i], w[i], path+"["+strconv.Itoa(i)+"]")...)
		}
		return out
	default:
		if got != want {
			return []string{path + ": value mismatch"}
		}
		return nil
	}
}
