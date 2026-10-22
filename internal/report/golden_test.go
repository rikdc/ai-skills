package report_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/report"
)

var update = flag.Bool("update", false, "update golden report files")

func goldenInput() *model.ReportInput {
	compareRubric := []model.Criterion{{Name: "A", MaxScore: 60}, {Name: "B", MaxScore: 40}}
	compareItems := func(a, b float64) []model.JudgedItem {
		return []model.JudgedItem{{Name: "A", Score: a, Max: 60}, {Name: "B", Score: b, Max: 40}}
	}
	compareRuns := []model.RunInput{
		run("with", 1, "ok", compareItems(50, 40)),
		run("with", 2, "ok", compareItems(60, 40)),
		run("without", 1, "ok", compareItems(40, 40)),
		run("without", 2, "ok", compareItems(45, 40)),
	}
	for i := range compareRuns {
		compareRuns[i].AgentUsage = usage(0.010+float64(i)*0.001, 900, 100)
		compareRuns[i].Usage = compareRuns[i].AgentUsage
		compareRuns[i].JudgeUsage = usage(0.002, 80, 20)
		compareRuns[i].CombinedUsage = usage(0.012+float64(i)*0.001, 980, 120)
	}
	functionalRubric := []model.Criterion{{Name: "A", MaxScore: 100}}
	functionalRuns := []model.RunInput{
		run("with", 1, "ok", []model.JudgedItem{{Name: "A", Score: 80, Max: 100}}),
		run("with", 2, "ok", []model.JudgedItem{{Name: "A", Score: 90, Max: 100}}),
	}
	for i := range functionalRuns {
		functionalRuns[i].AgentUsage = usage(0.05, 500, 50)
		functionalRuns[i].Usage = functionalRuns[i].AgentUsage
		functionalRuns[i].JudgeUsage = usage(0.02, 40, 10)
		functionalRuns[i].CombinedUsage = usage(0.07, 540, 60)
	}
	pendingRubric := []model.Criterion{{Name: "Scored", MaxScore: 52}, {Name: "Unjudged", MaxScore: 48}}
	pendingRuns := []model.RunInput{
		run("with", 1, "ok", []model.JudgedItem{{Name: "Scored", Score: 52, Max: 52}}),
		run("without", 1, "ok", []model.JudgedItem{{Name: "Scored", Score: 52, Max: 52}}),
	}
	pendingRuns[0].JudgeUsage = model.Usage{State: model.UsageUnknown, Known: false}
	pendingRuns[1].JudgeUsage = model.Usage{State: model.UsageUnknown, Known: false}

	in := &model.ReportInput{
		SuiteDir: "testdata/golden/suite",
		Manifest: model.SuiteManifest{
			Version:   1,
			CreatedAt: "2026-01-02T03:04:05Z",
			Repo:      model.RepoInfo{GitSHA: "deadbee"},
			Config: model.SuiteConfig{
				AgentCmd: "claude -p --model fixture-model",
				Arm:      "both",
				Runs:     2,
				Units:    []string{"skills/fixture"},
			},
			Scenarios: []model.PlannedScenario{
				{Plugin: "demo", Scenario: "compare"},
				{Plugin: "demo", Scenario: "functional"},
				{Plugin: "demo", Scenario: "pending"},
			},
		},
		State: model.SuiteState{
			Counts:          map[string]int{"agent_invocations": 6},
			DurationSeconds: fptr(42.5),
		},
		Results: []model.ResultRecord{},
		Scenarios: []model.ScenarioInput{
			{Plugin: "demo", Scenario: "compare", Units: []string{"skills/fixture"}, Mode: "comparison", Rubric: compareRubric, Runs: compareRuns},
			{Plugin: "demo", Scenario: "functional", Units: []string{"skills/fixture"}, Mode: "functional", Rubric: functionalRubric, Runs: functionalRuns},
			{Plugin: "demo", Scenario: "pending", Units: []string{"skills/fixture"}, Mode: "comparison", Rubric: pendingRubric, Runs: pendingRuns},
		},
		Attempts: []model.AttemptInput{
			knownAttempt(0.010, 900, 100, model.KindAgent),
			knownAttempt(0.002, 80, 20, model.KindJudge),
			{Kind: model.KindAgent, Usage: model.Usage{State: model.UsageUnknown}},
			{Kind: model.KindJudge, Usage: model.Usage{State: model.UsageNotApplicable}},
		},
	}
	return in
}

func TestGoldenReport(t *testing.T) {
	got := report.Build(goldenInput())
	dir := filepath.Join("testdata", "golden")
	jsonBytes, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	jsonBytes = append(jsonBytes, '\n')
	files := map[string][]byte{
		"report.json": jsonBytes,
		"report.md":   []byte(report.RenderMarkdown(got)),
		"report.html": []byte(report.RenderHTML(got)),
		"cli.txt":     []byte(report.RenderCLI(got, false)),
	}
	for name, data := range files {
		path := filepath.Join(dir, name)
		if *update {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v (run go test ./internal/report -update)", name, err)
		}
		if string(want) != string(data) {
			t.Errorf("%s does not match golden output (run go test ./internal/report -update)", name)
		}
	}
}
