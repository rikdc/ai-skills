package report_test

import (
	"math"
	"strings"
	"testing"

	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/report"
)

func fptr(v float64) *float64 { return &v }
func iptr(v int64) *int64     { return &v }

func usage(cost float64, input, output int64) model.Usage {
	return model.Usage{
		State: model.UsageKnown, Known: true, Source: "fixture", Currency: "USD",
		CostUSD:     fptr(cost),
		InputTokens: iptr(input), OutputTokens: iptr(output),
		TotalTokens: iptr(input + output),
	}
}

func knownAttempt(cost float64, input, output int64, kind model.AttemptKind) model.AttemptInput {
	return model.AttemptInput{Kind: kind, Usage: usage(cost, input, output)}
}

func input(rubric []model.Criterion, runs []model.RunInput, mode string, configuredRuns int) *model.ReportInput {
	return &model.ReportInput{
		SuiteDir: "testdata/suite",
		Manifest: model.SuiteManifest{
			Version:   1,
			CreatedAt: "2026-01-02T03:04:05Z",
			Repo:      model.RepoInfo{GitSHA: "deadbee"},
			Config: model.SuiteConfig{
				AgentCmd: "claude -p --model fixture-model",
				Arm:      "both",
				Runs:     configuredRuns,
				Units:    []string{"skills/fixture"},
			},
			Scenarios: []model.PlannedScenario{{Plugin: "demo", Scenario: "case"}},
		},
		State: model.SuiteState{Counts: map[string]int{"agent_invocations": 2 * configuredRuns}},
		Scenarios: []model.ScenarioInput{{
			Plugin: "demo", Scenario: "case", Units: []string{"skills/fixture"},
			Mode: mode, Rubric: rubric, Runs: runs,
		}},
	}
}

func run(arm string, number int, status string, items []model.JudgedItem) model.RunInput {
	return model.RunInput{
		Arm: arm, Run: number, Status: status, Complete: status == "ok",
		DurationSeconds: fptr(10), MechanicalItems: items,
		Usage:         usage(0.01, 900, 100),
		AgentUsage:    usage(0.01, 900, 100),
		JudgeUsage:    usage(0.002, 80, 20),
		CombinedUsage: usage(0.012, 980, 120),
		Evidence:      "runs/demo/case/" + arm + "/run",
	}
}

func TestFixedDenominatorAndPending(t *testing.T) {
	rubric := []model.Criterion{{Name: "Scored", MaxScore: 52}, {Name: "Unjudged", MaxScore: 48}}
	in := input(rubric, []model.RunInput{
		run("with", 1, "ok", []model.JudgedItem{{Name: "Scored", Score: 52, Max: 52}}),
		run("without", 1, "ok", []model.JudgedItem{{Name: "Scored", Score: 52, Max: 52}}),
	}, "comparison", 1)
	got := report.Build(in)
	if got.Totals.With == nil || *got.Totals.With != 52 {
		t.Fatalf("with = %v, want 52 against the fixed denominator", got.Totals.With)
	}
	if got.Scenarios[0].Runs[0].Percent == nil || *got.Scenarios[0].Runs[0].Percent != 52 {
		t.Fatalf("percent = %v, want 52", got.Scenarios[0].Runs[0].Percent)
	}
	if got.Scenarios[0].Runs[0].PendingPoints != 48 {
		t.Fatalf("pending = %v, want 48", got.Scenarios[0].Runs[0].PendingPoints)
	}
	if got.Totals.PendingPoints != 96 || got.Totals.PendingCriteria != 2 {
		t.Fatalf("totals pending = %v/%d", got.Totals.PendingPoints, got.Totals.PendingCriteria)
	}
	if got.Verdict.Status != "incomplete" {
		t.Fatalf("verdict = %q, want incomplete", got.Verdict.Status)
	}
}

func TestHelpsVerdictAndCriterionPairing(t *testing.T) {
	rubric := []model.Criterion{{Name: "A", MaxScore: 60}, {Name: "B", MaxScore: 40}}
	items := func(a, b float64) []model.JudgedItem {
		return []model.JudgedItem{{Name: "A", Score: a, Max: 60}, {Name: "B", Score: b, Max: 40}}
	}
	in := input(rubric, []model.RunInput{
		run("with", 1, "ok", items(50, 40)),
		run("with", 2, "ok", items(60, 40)),
		run("without", 1, "ok", items(40, 40)),
		run("without", 2, "ok", items(45, 40)),
	}, "comparison", 2)
	got := report.Build(in)
	if got.Verdict.Status != "helps" {
		t.Fatalf("verdict = %q", got.Verdict.Status)
	}
	if got.Totals.With == nil || *got.Totals.With != 95 || got.Totals.Without == nil || *got.Totals.Without != 82.5 {
		t.Fatalf("with/without = %v/%v", got.Totals.With, got.Totals.Without)
	}
	criterion := got.Criteria[0]
	if criterion.Delta == nil || math.Abs(*criterion.Delta-12.5) > 1e-9 || criterion.PairedRuns != 2 {
		t.Fatalf("criterion A = %+v", criterion)
	}
	if got.Totals.Won != 1 || got.Totals.Lost != 0 || got.Totals.Tied != 1 {
		t.Fatalf("won/lost/tied = %d/%d/%d", got.Totals.Won, got.Totals.Lost, got.Totals.Tied)
	}
}

func TestHurtsAndRegression(t *testing.T) {
	rubric := []model.Criterion{{Name: "A", MaxScore: 60}, {Name: "B", MaxScore: 40}}
	items := func(a, b float64) []model.JudgedItem {
		return []model.JudgedItem{{Name: "A", Score: a, Max: 60}, {Name: "B", Score: b, Max: 40}}
	}
	in := input(rubric, []model.RunInput{
		run("with", 1, "ok", items(30, 40)),
		run("with", 2, "ok", items(40, 40)),
		run("without", 1, "ok", items(50, 40)),
		run("without", 2, "ok", items(55, 40)),
	}, "comparison", 2)
	got := report.Build(in)
	if got.Verdict.Status != "hurts" {
		t.Fatalf("verdict = %q", got.Verdict.Status)
	}
	if got.Totals.Regressions != 1 || len(got.Scenarios[0].Regressions) != 1 {
		t.Fatalf("regressions = %d", got.Totals.Regressions)
	}
	if got.Scenarios[0].Regressions[0].Name != "A" {
		t.Fatalf("regression = %+v", got.Scenarios[0].Regressions[0])
	}
}

func TestVerdictStates(t *testing.T) {
	rubric := []model.Criterion{{Name: "A", MaxScore: 100}}
	items := func(score float64) []model.JudgedItem {
		return []model.JudgedItem{{Name: "A", Score: score, Max: 100}}
	}
	cases := []struct {
		name       string
		runs       []model.RunInput
		configured int
		want       string
	}{
		{
			name: "indicative",
			runs: []model.RunInput{
				run("with", 1, "ok", items(60)),
				run("without", 1, "ok", items(50)),
			},
			configured: 1,
			want:       "indicative",
		},
		{
			name: "no measurable effect",
			runs: []model.RunInput{
				run("with", 1, "ok", items(50)), run("with", 2, "ok", items(60)),
				run("without", 1, "ok", items(50)), run("without", 2, "ok", items(60)),
			},
			configured: 2,
			want:       "no measurable effect",
		},
		{
			name: "inconclusive overlap",
			runs: []model.RunInput{
				run("with", 1, "ok", items(50)), run("with", 2, "ok", items(60)),
				run("without", 1, "ok", items(50)), run("without", 2, "ok", items(55)),
			},
			configured: 2,
			want:       "inconclusive",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := report.Build(input(rubric, tc.runs, "comparison", tc.configured))
			if got.Verdict.Status != tc.want {
				t.Fatalf("verdict = %q, want %q", got.Verdict.Status, tc.want)
			}
		})
	}
}

func TestFunctionalUsesAbsoluteScoreAndNoDelta(t *testing.T) {
	rubric := []model.Criterion{{Name: "A", MaxScore: 100}}
	in := input(rubric, []model.RunInput{
		run("with", 1, "ok", []model.JudgedItem{{Name: "A", Score: 80, Max: 100}}),
	}, "functional", 1)
	in.Scenarios[0].Runs[0].AgentUsage = usage(0.10, 900, 100)
	in.Scenarios[0].Runs[0].JudgeUsage = usage(0.20, 150, 50)
	in.Scenarios[0].Runs[0].CombinedUsage = usage(0.30, 1050, 150)
	got := report.Build(in)
	if got.Verdict.Status != "functional" {
		t.Fatalf("verdict = %q", got.Verdict.Status)
	}
	if got.Totals.With != nil || got.Totals.Without != nil || got.Totals.Delta != nil {
		t.Fatalf("functional totals must not carry a delta: %+v", got.Totals)
	}
	if got.Totals.FunctionalWith == nil || *got.Totals.FunctionalWith != 80 {
		t.Fatalf("functional_with = %v", got.Totals.FunctionalWith)
	}
	if got.Scenarios[0].Arms.Without.Runs != 0 || got.Scenarios[0].Delta != nil {
		t.Fatalf("scenario = %+v", got.Scenarios[0])
	}
	if got.Cost.With.Combined.Cost.Total == nil || math.Abs(*got.Cost.With.Combined.Cost.Total-0.30) > 1e-9 {
		t.Fatalf("cost with = %v", got.Cost.With.Combined.Cost.Total)
	}
	if got.Cost.DeltaTotals.CombinedCost != nil || got.Cost.Delta.CombinedCost != nil {
		t.Fatal("functional expenditure must not become a delta")
	}
}

func TestExpenditureClassifiesEveryAttemptOnce(t *testing.T) {
	in := input([]model.Criterion{{Name: "A", MaxScore: 100}}, nil, "comparison", 1)
	in.Attempts = []model.AttemptInput{
		knownAttempt(0.01, 100, 10, model.KindAgent),
		knownAttempt(0.002, 50, 5, model.KindJudge),
		{Kind: model.KindAgent, Usage: model.Usage{State: model.UsageUnknown, Known: false}},
		{Kind: model.KindJudge, Usage: model.Usage{State: model.UsageNotApplicable}},
	}
	got := report.Build(in)
	expenditure := got.Expenditure
	if expenditure.TotalAttempts != 4 || expenditure.KnownAttempts != 2 ||
		expenditure.UnknownAttempts != 1 || expenditure.NotApplicableAttempts != 1 {
		t.Fatalf("expenditure = %+v", expenditure)
	}
	if expenditure.Complete {
		t.Fatal("unknown attempt must make expenditure incomplete")
	}
	if expenditure.KnownTotal.State != model.UsageUnknown || expenditure.KnownTotal.Known {
		t.Fatalf("known total state = %+v", expenditure.KnownTotal)
	}
	if expenditure.KnownTotal.CostUSD == nil || math.Abs(*expenditure.KnownTotal.CostUSD-0.012) > 1e-9 {
		t.Fatalf("known total cost = %v", expenditure.KnownTotal.CostUSD)
	}
	if expenditure.AgentAttempts != 2 || expenditure.JudgeAttempts != 2 {
		t.Fatalf("kind counts = %+v", expenditure)
	}
}

func TestCompareMatchesByNameAndRubric(t *testing.T) {
	rubric := []model.Criterion{{Name: "A", MaxScore: 100}}
	build := func(withScore, withoutScore float64, extra bool) model.Report {
		runs := []model.RunInput{
			run("with", 1, "ok", []model.JudgedItem{{Name: "A", Score: withScore, Max: 100}}),
			run("without", 1, "ok", []model.JudgedItem{{Name: "A", Score: withoutScore, Max: 100}}),
		}
		in := input(rubric, runs, "comparison", 1)
		if extra {
			other := model.ScenarioInput{
				Plugin: "demo", Scenario: "extra", Mode: "comparison", Rubric: rubric,
				Runs: []model.RunInput{
					run("with", 1, "ok", []model.JudgedItem{{Name: "A", Score: 100, Max: 100}}),
					run("without", 1, "ok", []model.JudgedItem{{Name: "A", Score: 100, Max: 100}}),
				},
			}
			in.Scenarios = append(in.Scenarios, other)
		}
		return report.Build(in)
	}
	current := build(100, 95, true)
	baseline := build(100, 90, false)
	comparison := report.Compare(current, baseline)
	if comparison.Status != "comparable" || comparison.ComparableScenarios != 1 {
		t.Fatalf("comparison = %+v", comparison)
	}
	if comparison.CurrentScenarios != 2 || comparison.BaselineScenarios != 1 {
		t.Fatalf("counts = %+v", comparison)
	}
	if len(comparison.Excluded) != 1 || comparison.Excluded[0].Reason != "missing from baseline" {
		t.Fatalf("excluded = %+v", comparison.Excluded)
	}
	if comparison.Delta.Effect == nil || *comparison.Delta.Effect != -5 {
		t.Fatalf("effect = %v", comparison.Delta.Effect)
	}
}

func TestCompareRejectsChangedRubric(t *testing.T) {
	baseRubric := []model.Criterion{{Name: "A", MaxScore: 100}}
	curRubric := []model.Criterion{{Name: "B", MaxScore: 100}}
	items := func(name string, score float64) []model.JudgedItem {
		return []model.JudgedItem{{Name: name, Score: score, Max: 100}}
	}
	baseline := report.Build(input(baseRubric, []model.RunInput{
		run("with", 1, "ok", items("A", 100)),
		run("without", 1, "ok", items("A", 90)),
	}, "comparison", 1))
	current := report.Build(input(curRubric, []model.RunInput{
		run("with", 1, "ok", items("B", 100)),
		run("without", 1, "ok", items("B", 100)),
	}, "comparison", 1))
	comparison := report.Compare(current, baseline)
	if comparison.Comparable || comparison.ComparableScenarios != 0 {
		t.Fatalf("rubric change was compared: %+v", comparison)
	}
	if len(comparison.Excluded) != 1 || comparison.Excluded[0].Reason != "rubric changed" {
		t.Fatalf("excluded = %+v", comparison.Excluded)
	}
	if comparison.Delta.Effect != nil {
		t.Fatal("deltas must be withheld")
	}
}

func TestRenderersEscapeAndSelfContain(t *testing.T) {
	rubric := []model.Criterion{{Name: "A <script>alert(1)&</script>", MaxScore: 100}}
	in := input(rubric, []model.RunInput{
		run("with", 1, "ok", []model.JudgedItem{{Name: "A <script>alert(1)&</script>", Score: 100, Max: 100}}),
		run("without", 1, "ok", []model.JudgedItem{{Name: "A <script>alert(1)&</script>", Score: 50, Max: 100}}),
	}, "comparison", 1)
	in.Scenarios[0].Scenario = "evil<&>"
	got := report.Build(in)
	doc := report.RenderHTML(got)
	if !strings.Contains(doc, "evil&lt;&amp;&gt;") {
		t.Error("scenario name not escaped")
	}
	if !strings.Contains(doc, "&lt;script&gt;alert(1)&amp;&lt;/script&gt;") {
		t.Error("criterion name not escaped")
	}
	if strings.Contains(doc, "<script>alert(1)") {
		t.Error("raw script text emitted")
	}
	if strings.Contains(doc, `src="http`) || strings.Contains(doc, `href="http`) {
		t.Error("HTML is not self-contained")
	}
	markdown := report.RenderMarkdown(got)
	if !strings.Contains(markdown, "# Eval report: skills/fixture") {
		t.Error("markdown header missing")
	}
	cli := report.RenderCLI(got, false)
	if !strings.Contains(cli, "Verdict: indicative") {
		t.Error("CLI verdict missing")
	}
}

func TestPartialCostWithholdsEfficiency(t *testing.T) {
	rubric := []model.Criterion{{Name: "A", MaxScore: 100}}
	in := input(rubric, []model.RunInput{
		run("with", 1, "ok", []model.JudgedItem{{Name: "A", Score: 100, Max: 100}}),
		run("without", 1, "ok", []model.JudgedItem{{Name: "A", Score: 50, Max: 100}}),
	}, "comparison", 1)
	in.Scenarios[0].Runs[0].AgentUsage = usage(0.015, 900, 100)
	in.Scenarios[0].Runs[0].JudgeUsage = usage(0.003, 80, 20)
	in.Scenarios[0].Runs[0].CombinedUsage = usage(0.018, 980, 120)
	in.Scenarios[0].Runs[1].AgentUsage = usage(0.015, 900, 100)
	in.Scenarios[0].Runs[1].JudgeUsage = model.Usage{State: model.UsageUnknown}
	in.Scenarios[0].Runs[1].CombinedUsage = model.Usage{State: model.UsageUnknown}
	got := report.Build(in)
	if got.Cost.CostPerDeltaPoint != nil {
		t.Fatalf("partial coverage produced an efficiency claim: %v", got.Cost.CostPerDeltaPoint)
	}
	if got.Cost.With.Combined.Cost.Total == nil || math.Abs(*got.Cost.With.Combined.Cost.Total-0.018) > 1e-9 {
		t.Fatalf("with cost = %v", got.Cost.With.Combined.Cost.Total)
	}
	if got.Cost.Without.Combined.Cost.Total != nil || got.Cost.Without.Combined.Cost.RunsKnown != 0 {
		t.Fatalf("without cost = %+v", got.Cost.Without.Combined.Cost)
	}
	cli := report.RenderCLI(got, false)
	if !strings.Contains(cli, "coverage: agent 2/2, judge 1/2, total 1/2") {
		t.Errorf("CLI coverage wrong:\n%s", cli)
	}
	if !strings.Contains(cli, "Efficiency: —") {
		t.Error("CLI must refuse an efficiency claim")
	}
}

func TestSubCentFormatting(t *testing.T) {
	rubric := []model.Criterion{{Name: "A", MaxScore: 100}}
	in := input(rubric, []model.RunInput{
		run("with", 1, "ok", []model.JudgedItem{{Name: "A", Score: 100, Max: 100}}),
		run("without", 1, "ok", []model.JudgedItem{{Name: "A", Score: 99, Max: 100}}),
	}, "comparison", 1)
	in.Scenarios[0].Runs[0].AgentUsage = usage(0.0008, 90, 10)
	in.Scenarios[0].Runs[0].JudgeUsage = usage(0.0002, 9, 1)
	in.Scenarios[0].Runs[0].CombinedUsage = usage(0.0010, 99, 11)
	in.Scenarios[0].Runs[1].AgentUsage = usage(0.0004, 80, 10)
	in.Scenarios[0].Runs[1].JudgeUsage = usage(0.0001, 9, 1)
	in.Scenarios[0].Runs[1].CombinedUsage = usage(0.0005, 89, 11)
	got := report.Build(in)
	if got.Cost.CostPerDeltaPoint == nil {
		t.Fatal("cost per point missing")
	}
	for name, doc := range map[string]string{
		"cli":  report.RenderCLI(got, false),
		"md":   report.RenderMarkdown(got),
		"html": report.RenderHTML(got),
	} {
		if !strings.Contains(doc, "+$0.00050") {
			t.Errorf("%s: sub-half-cent delta not rendered with 5 decimals", name)
		}
		if !strings.Contains(doc, "$0.00050 per point") {
			t.Errorf("%s: sub-half-cent efficiency not rendered with 5 decimals", name)
		}
	}
}

func TestRunnerFailureForcesIncomplete(t *testing.T) {
	rubric := []model.Criterion{{Name: "A", MaxScore: 100}}
	in := input(rubric, []model.RunInput{
		run("with", 1, "ok", []model.JudgedItem{{Name: "A", Score: 100, Max: 100}}),
		run("without", 1, "agent-failed", nil),
	}, "comparison", 2)
	in.Results = []model.ResultRecord{{Plugin: "demo", Scenario: "case", Status: "runner-failed"}}
	got := report.Build(in)
	if got.Verdict.Status != "incomplete" {
		t.Fatalf("verdict = %q", got.Verdict.Status)
	}
	if got.Totals.FailedRuns != 1 || got.Totals.ResultsFailures != 1 || got.Totals.RunnerFailures != 1 {
		t.Fatalf("failure counts = %+v", got.Totals)
	}
}

func TestFailedRunUsageIsExpenditureOnly(t *testing.T) {
	rubric := []model.Criterion{{Name: "A", MaxScore: 100}}
	in := input(rubric, []model.RunInput{
		run("with", 1, "ok", []model.JudgedItem{{Name: "A", Score: 100, Max: 100}}),
		run("without", 1, "agent-failed", nil),
	}, "comparison", 1)
	in.Attempts = []model.AttemptInput{
		knownAttempt(5, 1000, 100, model.KindAgent),
		knownAttempt(1, 100, 10, model.KindJudge),
	}
	got := report.Build(in)
	if got.Cost.Without.Agent.Cost.Total != nil || got.Cost.Without.Agent.Cost.RunsKnown != 0 {
		t.Fatalf("failed run leaked into accepted cost: %+v", got.Cost.Without.Agent)
	}
	if got.Cost.With.Agent.Cost.Total == nil || *got.Cost.With.Agent.Cost.Total != 0.01 {
		t.Fatalf("accepted cost = %v", got.Cost.With.Agent.Cost.Total)
	}
	if got.Expenditure.KnownTotal.CostUSD == nil || *got.Expenditure.KnownTotal.CostUSD != 6 {
		t.Fatalf("expenditure = %+v", got.Expenditure.KnownTotal)
	}
}

func TestExpenditureRenderedInEveryFormat(t *testing.T) {
	in := input([]model.Criterion{{Name: "A", MaxScore: 100}}, []model.RunInput{
		run("with", 1, "ok", []model.JudgedItem{{Name: "A", Score: 100, Max: 100}}),
		run("without", 1, "ok", []model.JudgedItem{{Name: "A", Score: 90, Max: 100}}),
	}, "comparison", 1)
	in.Attempts = []model.AttemptInput{
		knownAttempt(0.02, 100, 10, model.KindAgent),
		{Kind: model.KindAgent, Usage: model.Usage{State: model.UsageUnknown, Known: false}},
		{Kind: model.KindJudge, Usage: model.Usage{State: model.UsageNotApplicable}},
	}
	got := report.Build(in)
	if got.Expenditure.Complete {
		t.Fatal("expenditure with an unknown attempt is not complete")
	}
	for name, doc := range map[string]string{
		"cli":  report.RenderCLI(got, false),
		"md":   report.RenderMarkdown(got),
		"html": report.RenderHTML(got),
	} {
		if !strings.Contains(doc, "Expenditure") {
			t.Errorf("%s: expenditure section missing", name)
		}
		if !strings.Contains(doc, "unknown") {
			t.Errorf("%s: unknown attempt count missing", name)
		}
		if !strings.Contains(strings.ToLower(doc), "lower bound") {
			t.Errorf("%s: lower-bound label missing", name)
		}
	}
}

func TestCompareWithholdsWallClockWhenScenariosAreExcluded(t *testing.T) {
	rubric := []model.Criterion{{Name: "A", MaxScore: 100}}
	baseInput := input(rubric, []model.RunInput{
		run("with", 1, "ok", []model.JudgedItem{{Name: "A", Score: 100, Max: 100}}),
		run("without", 1, "ok", []model.JudgedItem{{Name: "A", Score: 90, Max: 100}}),
	}, "comparison", 1)
	baseInput.State.DurationSeconds = fptr(10)
	base := report.Build(baseInput)

	currentInput := input(rubric, []model.RunInput{
		run("with", 1, "ok", []model.JudgedItem{{Name: "A", Score: 100, Max: 100}}),
		run("without", 1, "ok", []model.JudgedItem{{Name: "A", Score: 95, Max: 100}}),
	}, "comparison", 1)
	currentInput.State.DurationSeconds = fptr(20)
	currentInput.Scenarios = append(currentInput.Scenarios, model.ScenarioInput{
		Plugin: "demo", Scenario: "extra", Mode: "comparison", Rubric: rubric,
		Runs: []model.RunInput{
			run("with", 1, "ok", []model.JudgedItem{{Name: "A", Score: 100, Max: 100}}),
			run("without", 1, "ok", []model.JudgedItem{{Name: "A", Score: 100, Max: 100}}),
		},
	})
	comparison := report.Compare(report.Build(currentInput), base)
	if comparison.Delta.WallSeconds != nil {
		t.Fatalf("wall-clock delta was not withheld: %v", *comparison.Delta.WallSeconds)
	}
	if comparison.Status != "comparable" || comparison.ComparableScenarios != 1 {
		t.Fatalf("comparison = %+v", comparison)
	}
}

func TestBaselineNotComparableWhenCurrentIsNotOK(t *testing.T) {
	rubric := []model.Criterion{{Name: "A", MaxScore: 100}}
	base := report.Build(input(rubric, []model.RunInput{
		run("with", 1, "ok", []model.JudgedItem{{Name: "A", Score: 100, Max: 100}}),
		run("without", 1, "ok", []model.JudgedItem{{Name: "A", Score: 90, Max: 100}}),
	}, "comparison", 1))
	current := report.Build(input(rubric, []model.RunInput{
		run("with", 1, "agent-failed", []model.JudgedItem{{Name: "A", Score: 100, Max: 100}}),
		run("without", 1, "ok", []model.JudgedItem{{Name: "A", Score: 90, Max: 100}}),
	}, "comparison", 1))
	comparison := report.Compare(current, base)
	if comparison.Comparable || comparison.Status != "not comparable" {
		t.Fatalf("comparison = %+v", comparison)
	}
	if comparison.Delta.Effect != nil || comparison.Delta.WithScore != nil {
		t.Fatal("baseline deltas must be withheld without a comparable scenario")
	}
	if len(comparison.Excluded) != 1 || comparison.Excluded[0].Reason != "not comparable in current" {
		t.Fatalf("excluded = %+v", comparison.Excluded)
	}
}
