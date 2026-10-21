package report_test

import (
	"strings"
	"testing"

	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/report"
)

var skillRubric = []model.Criterion{{Name: "A", MaxScore: 100}}

func scoredRun(arm string, number int, score float64, skill string) model.RunInput {
	r := run(arm, number, "ok", []model.JudgedItem{{Name: "A", Score: score, Max: 100}})
	if skill != "" {
		r.SkillUse = &model.SkillUse{State: skill, Skills: map[string]string{"fixture": skill}}
	}
	return r
}

// twoScenarios builds a suite where "used" always invokes the skill and
// "idle" is given the supplied with-arm skill state.
func twoScenarios(idleSkill string, n int) *model.ReportInput {
	in := input(skillRubric, nil, "comparison", n)
	in.Scenarios = nil
	for _, sc := range []struct {
		name  string
		skill string
		with  float64
	}{{"used", model.SkillInvoked, 100}, {"idle", idleSkill, 100}} {
		scenario := model.ScenarioInput{
			Plugin: "demo", Scenario: sc.name, Units: []string{"skills/fixture"},
			Mode: "comparison", Rubric: skillRubric,
		}
		for number := 1; number <= n; number++ {
			scenario.Runs = append(scenario.Runs,
				scoredRun("with", number, sc.with, sc.skill),
				scoredRun("without", number, 50, ""))
		}
		in.Scenarios = append(in.Scenarios, scenario)
	}
	return in
}

func findWarning(r model.Report, code string) *model.Warning {
	for i := range r.Warnings {
		if r.Warnings[i].Code == code {
			return &r.Warnings[i]
		}
	}
	return nil
}

func scenarioNamed(t *testing.T, r model.Report, name string) model.ScenarioResult {
	t.Helper()
	for _, sc := range r.Scenarios {
		if sc.Scenario == name {
			return sc
		}
	}
	t.Fatalf("no scenario %s", name)
	return model.ScenarioResult{}
}

func TestScenarioWhereSkillWasNeverInvokedIsExcludedFromTheHeadline(t *testing.T) {
	in := twoScenarios(model.SkillNotInvoked, 3)
	// The idle scenario's with arm scored badly because the skill was unused.
	for i := range in.Scenarios[1].Runs {
		if in.Scenarios[1].Runs[i].Arm == "with" {
			in.Scenarios[1].Runs[i] = scoredRun("with", in.Scenarios[1].Runs[i].Run, 0, model.SkillNotInvoked)
		}
	}
	got := report.Build(in)

	idle := scenarioNamed(t, got, "idle")
	if idle.Status != "skill-not-exercised" || idle.Skill == nil || idle.Skill.Exercised ||
		idle.Skill.NotInvoked != 3 || idle.Skill.Invoked != 0 {
		t.Fatalf("idle = status %q skill %+v", idle.Status, idle.Skill)
	}
	if used := scenarioNamed(t, got, "used"); used.Status != "ok" || used.Skill == nil || !used.Skill.Exercised {
		t.Fatalf("used = status %q skill %+v", used.Status, used.Skill)
	}
	if got.Totals.SkillNotExercised != 1 {
		t.Fatalf("skill_not_exercised = %d", got.Totals.SkillNotExercised)
	}
	if got.Totals.With == nil || *got.Totals.With != 100 || got.Totals.Delta == nil || *got.Totals.Delta != 50 {
		t.Fatalf("headline must use only the exercised scenario: with %v delta %v", got.Totals.With, got.Totals.Delta)
	}
	if warning := findWarning(got, model.WarnSkillNotExercised); warning == nil || warning.Scenario != "demo/idle" {
		t.Fatalf("missing skill-not-exercised warning: %+v", got.Warnings)
	}
	if !strings.Contains(got.Verdict.Reason, "excluded because the skill was never invoked") {
		t.Fatalf("verdict reason = %q", got.Verdict.Reason)
	}
	for _, sc := range got.Scenarios {
		for _, r := range sc.Runs {
			if r.Arm == "with" && r.SkillUse == nil {
				t.Fatalf("run row lost its skill use: %+v", r)
			}
		}
	}
}

func TestUnknownSkillUseNeverExcludesAScenario(t *testing.T) {
	got := report.Build(twoScenarios(model.SkillUnknown, 3))
	idle := scenarioNamed(t, got, "idle")
	if idle.Status != "ok" || idle.Skill == nil || !idle.Skill.Exercised || idle.Skill.Unknown != 3 {
		t.Fatalf("idle = status %q skill %+v", idle.Status, idle.Skill)
	}
	if got.Totals.SkillNotExercised != 0 {
		t.Fatalf("skill_not_exercised = %d", got.Totals.SkillNotExercised)
	}
	if findWarning(got, model.WarnSkillNotExercised) != nil {
		t.Fatal("unknown must not warn as not exercised")
	}
	if findWarning(got, model.WarnSkillUnknown) == nil {
		t.Fatalf("missing skill-use-unknown warning: %+v", got.Warnings)
	}
}

func TestPartiallyExercisedSkillIsFlaggedButKept(t *testing.T) {
	in := twoScenarios(model.SkillInvoked, 3)
	in.Scenarios[1].Runs[0] = scoredRun("with", 1, 100, model.SkillNotInvoked)
	got := report.Build(in)
	idle := scenarioNamed(t, got, "idle")
	if idle.Status != "ok" || idle.Skill.Invoked != 2 || idle.Skill.NotInvoked != 1 {
		t.Fatalf("idle = %q %+v", idle.Status, idle.Skill)
	}
	if findWarning(got, report.WarnSkillPartial) == nil {
		t.Fatalf("missing partial warning: %+v", got.Warnings)
	}
}

func TestSkillNeverUsedAnywhereHasNoVerdict(t *testing.T) {
	in := twoScenarios(model.SkillNotInvoked, 3)
	for i := range in.Scenarios[0].Runs {
		if in.Scenarios[0].Runs[i].Arm == "with" {
			in.Scenarios[0].Runs[i] = scoredRun("with", in.Scenarios[0].Runs[i].Run, 100, model.SkillNotInvoked)
		}
	}
	got := report.Build(in)
	if got.Verdict.Status != "incomplete" || got.Totals.Delta != nil {
		t.Fatalf("verdict %q delta %v", got.Verdict.Status, got.Totals.Delta)
	}
}

func TestSuitesWithoutSkillUseKeepTheirBehaviour(t *testing.T) {
	got := report.Build(twoScenarios("", 3))
	for _, sc := range got.Scenarios {
		if sc.Status != "ok" {
			t.Fatalf("%s status = %q", sc.Scenario, sc.Status)
		}
	}
	if got.Totals.SkillNotExercised != 0 || findWarning(got, model.WarnSkillNotExercised) != nil {
		t.Fatalf("unexpected skill warning: %+v", got.Warnings)
	}
}

func TestSameModelJudgeWarning(t *testing.T) {
	in := twoScenarios(model.SkillInvoked, 3)
	in.Manifest.Config.AgentCmd = `opencode run -m opencode-go/glm "$(cat)"`
	in.Manifest.Config.Judge.Cmd = `opencode run --auto -m opencode-go/glm "$(cat)"`
	warning := findWarning(report.Build(in), model.WarnSameModelJudge)
	if warning == nil || !strings.Contains(warning.Message, "opencode-go/glm") ||
		!strings.Contains(warning.Message, "lenient") {
		t.Fatalf("same-model warning = %+v", warning)
	}

	in.Manifest.Config.Judge.Cmd = `opencode run -m opencode-go/other "$(cat)"`
	if warning := findWarning(report.Build(in), model.WarnSameModelJudge); warning != nil {
		t.Fatalf("different judge model warned: %+v", warning)
	}
}

func TestAttemptFailuresAreSurfaced(t *testing.T) {
	in := twoScenarios(model.SkillInvoked, 3)
	in.Attempts = []model.AttemptInput{
		knownAttempt(0.01, 10, 10, model.KindAgent),
		{Kind: model.KindJudge, Usage: usage(0, 0, 0), Status: model.StatusAgentFailed,
			Error: "judge invocation failed:\n  Unexpected server error"},
		{Kind: model.KindJudge, Usage: usage(0, 0, 0), Status: model.StatusAgentFailed, Error: "second"},
		{Kind: model.KindJudge, Usage: usage(0.01, 10, 10), Status: model.StatusOK},
		{Kind: model.KindAgent, Usage: usage(0, 0, 0), Status: model.StatusInterrupted},
	}
	got := report.Build(in)
	failures := got.AttemptFailures
	if failures == nil || failures.Failed != 3 || len(failures.Groups) != 2 {
		t.Fatalf("failures = %+v", failures)
	}
	agent, judge := failures.Groups[0], failures.Groups[1]
	if agent.Kind != model.KindAgent || agent.Failed != 1 || agent.Total != 2 || agent.FirstError != "status interrupted" {
		t.Fatalf("agent group = %+v", agent)
	}
	if judge.Kind != model.KindJudge || judge.Failed != 2 || judge.Total != 3 ||
		judge.FirstError != "judge invocation failed: Unexpected server error" {
		t.Fatalf("judge group = %+v", judge)
	}
	if findWarning(got, model.WarnAttemptFailures) == nil {
		t.Fatalf("missing attempt-failures warning: %+v", got.Warnings)
	}

	for name, rendered := range map[string]string{
		"md":   report.RenderMarkdown(got),
		"html": report.RenderHTML(got),
		"cli":  report.RenderCLI(got, false),
	} {
		if !strings.Contains(rendered, "Unexpected server error") || !strings.Contains(rendered, "Failed attempts") {
			t.Errorf("%s output hides the failed attempts:\n%s", name, rendered)
		}
	}
}

func TestNoAttemptFailuresMeansNoSection(t *testing.T) {
	in := twoScenarios(model.SkillInvoked, 3)
	in.Attempts = []model.AttemptInput{knownAttempt(0.01, 10, 10, model.KindAgent)}
	got := report.Build(in)
	if got.AttemptFailures != nil || findWarning(got, model.WarnAttemptFailures) != nil {
		t.Fatalf("clean run reported failures: %+v", got.AttemptFailures)
	}
	if strings.Contains(report.RenderMarkdown(got), "Failed attempts") {
		t.Fatal("clean run renders a failed-attempts section")
	}
}

func TestLowNIsStatedProminently(t *testing.T) {
	one := report.Build(twoScenarios(model.SkillInvoked, 1))
	if one.Verdict.Status != "indicative" || one.Verdict.Reason != "one run per arm; variance cannot be estimated" {
		t.Fatalf("n=1 verdict = %+v", one.Verdict)
	}
	if warning := findWarning(one, model.WarnLowN); warning == nil || !strings.Contains(warning.Message, "n=1") {
		t.Fatalf("n=1 warning = %+v", warning)
	}

	two := report.Build(twoScenarios(model.SkillInvoked, 2))
	if !strings.Contains(two.Verdict.Reason, "only two runs per arm") {
		t.Fatalf("n=2 verdict reason = %q", two.Verdict.Reason)
	}
	if findWarning(two, model.WarnLowN) == nil {
		t.Fatal("n=2 must warn")
	}

	three := report.Build(twoScenarios(model.SkillInvoked, 3))
	if findWarning(three, model.WarnLowN) != nil || strings.Contains(three.Verdict.Reason, "only two") {
		t.Fatalf("n=3 flagged as low: %+v / %q", three.Warnings, three.Verdict.Reason)
	}
}

func TestRenderersShowWarningsAndSkillUse(t *testing.T) {
	in := twoScenarios(model.SkillNotInvoked, 1)
	got := report.Build(in)
	for name, rendered := range map[string]string{
		"md":   report.RenderMarkdown(got),
		"html": report.RenderHTML(got),
		"cli":  report.RenderCLI(got, false),
	} {
		for _, want := range []string{"skill-not-exercised", "low-n", "not-invoked"} {
			if !strings.Contains(rendered, want) {
				t.Errorf("%s output lacks %q", name, want)
			}
		}
	}
	md := report.RenderMarkdown(got)
	if strings.Index(md, "## Warnings") > strings.Index(md, "| With | Without |") {
		t.Error("warnings must precede the headline table")
	}
}
