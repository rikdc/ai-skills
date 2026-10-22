package report_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/report"
	"github.com/rikdc/ai-skills/internal/reportio"
)

// TestV3ShellParity materialises a temporary legacy view from a Go-produced
// v3 attempt tree, runs the shell report on it, and compares the semantic
// report with the native v3 load. The legacy view is test-only.
func TestV3ShellParity(t *testing.T) {
	root, suiteDir := runStubV3Suite(t)
	legacy := t.TempDir()
	materializeLegacyView(t, suiteDir, legacy)

	shellDoc := shellReport(t, legacy)
	loaded, err := reportio.Load(suiteDir, reportio.Options{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	native := toJSONMap(t, report.Build(loaded))

	want := semanticReport(t, shellDoc)
	got := semanticReport(t, native)
	if diffs := semanticDiff(got, want, ""); len(diffs) > 0 {
		t.Fatalf("v3 report and shell legacy-view report differ semantically: %v", diffs)
	}
}

// materializeLegacyView copies the accepted v3 artifacts into the v1 layout
// that scripts/eval-report.sh reads. It lives only under a temp directory and
// never touches the source suite.
func materializeLegacyView(t *testing.T, suiteDir, dst string) {
	t.Helper()
	materializeManifest(t, suiteDir, filepath.Join(dst, "manifest.json"))
	copyFile(t, filepath.Join(suiteDir, "suite.json"), filepath.Join(dst, "suite.json"))
	copyFile(t, filepath.Join(suiteDir, "results.jsonl"), filepath.Join(dst, "results.jsonl"))
	copyTree(t, filepath.Join(suiteDir, "criteria"), filepath.Join(dst, "criteria"))

	var manifest model.SuiteManifest
	data, err := os.ReadFile(filepath.Join(suiteDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	arm := manifest.Config.Arm
	if arm == "" {
		arm = "both"
	}
	runs := manifest.Config.Runs
	if runs <= 0 {
		runs = 1
	}
	for _, planned := range manifest.Scenarios {
		arms := []string{"with", "without"}
		if planned.Mode == string(model.ModeFunctional) || arm == "with" {
			arms = []string{"with"}
		} else if arm == "without" {
			arms = []string{"without"}
		}
		for _, armName := range arms {
			for run := 1; run <= runs; run++ {
				src := filepath.Join(suiteDir, "runs", planned.Plugin, planned.Scenario,
					armName, "run-"+strconv.Itoa(run))
				target := filepath.Join(dst, "runs", planned.Plugin, planned.Scenario,
					armName, "run-"+strconv.Itoa(run))
				materializeRun(t, src, target)
			}
		}
	}
}

// materializeManifest rewrites the v3-only judge and repo fields into the v1
// manifest keys the shell report reads. This is a test-only compatibility
// view, not a production storage format.
func materializeManifest(t *testing.T, suiteDir, dst string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(suiteDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	config, _ := doc["config"].(map[string]any)
	if judge, ok := config["judge"].(map[string]any); ok {
		if cmd, ok := judge["cmd"].(string); ok && config["judge_cmd"] == nil {
			config["judge_cmd"] = cmd
		}
	}
	repo, _ := doc["repo"].(map[string]any)
	if config["git_sha"] == nil {
		config["git_sha"] = repo["git_sha"]
	}
	if config["git_dirty"] == nil {
		config["git_dirty"] = repo["git_dirty"]
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func materializeRun(t *testing.T, src, dst string) {
	t.Helper()
	var current model.CurrentPointer
	data, err := os.ReadFile(filepath.Join(src, "current.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &current); err != nil {
		t.Fatal(err)
	}
	agentDir := filepath.Join(src, "attempts", current.AgentAttempt)
	copyFile(t, filepath.Join(src, "result.json"), filepath.Join(dst, "result.json"))
	copyFile(t, filepath.Join(src, "grading-request.json"), filepath.Join(dst, "grading-request.json"))
	copyFile(t, filepath.Join(agentDir, "mechanical.json"), filepath.Join(dst, "mechanical.json"))
	copyFile(t, filepath.Join(agentDir, "usage.json"), filepath.Join(dst, "usage.json"))
	copyFile(t, filepath.Join(agentDir, "snapshot.tar"), filepath.Join(dst, "snapshot.tar"))
	if current.JudgeAttempt != "" {
		judgeDir := filepath.Join(src, "attempts", current.JudgeAttempt)
		copyFile(t, filepath.Join(judgeDir, "judgement.json"), filepath.Join(dst, "judgement.json"))
		copyFile(t, filepath.Join(judgeDir, "judge-result.json"), filepath.Join(dst, "judge-result.json"))
		copyFile(t, filepath.Join(judgeDir, "judge-usage.json"), filepath.Join(dst, "judge-usage.json"))
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		copyFile(t, path, filepath.Join(dst, rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
