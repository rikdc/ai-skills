package report

import (
	"fmt"
	"strings"

	"github.com/rikdc/ai-skills/internal/model"
)

// StatusSkillNotExercised is the scenario status when every with-arm run
// provably left the skill unused.
const StatusSkillNotExercised = "skill-not-exercised"

// WarnSkillPartial flags a scenario whose skill was used in only some of its
// with-arm runs.
const WarnSkillPartial = "skill-partially-exercised"

const maxErrorLength = 400

// skillCoverage summarises skill use over a scenario's with-arm runs. It is
// nil when no run recorded skill use (no skills, or an older suite).
func skillCoverage(runs []model.RunRow) *model.SkillCoverage {
	var coverage model.SkillCoverage
	seen := false
	for _, run := range runs {
		if run.Arm != "with" || run.SkillUse == nil {
			continue
		}
		seen = true
		switch run.SkillUse.State {
		case model.SkillInvoked:
			coverage.Invoked++
		case model.SkillNotInvoked:
			coverage.NotInvoked++
		default:
			coverage.Unknown++
		}
	}
	if !seen {
		return nil
	}
	// Unknown never counts against a scenario: only proof of non-use does.
	coverage.Exercised = coverage.Invoked > 0 || coverage.Unknown > 0
	return &coverage
}

func skillNotExercised(sc model.ScenarioResult) bool {
	return !sc.Functional && sc.Skill != nil && !sc.Skill.Exercised
}

func scenarioLabelOf(sc model.ScenarioResult) string {
	return sc.Plugin + "/" + sc.Scenario
}

func buildAttemptFailures(attempts []model.AttemptInput) *model.AttemptFailures {
	order := []model.AttemptKind{model.KindAgent, model.KindHook, model.KindJudge}
	groups := map[model.AttemptKind]*model.AttemptFailureGroup{}
	for _, kind := range order {
		groups[kind] = &model.AttemptFailureGroup{Kind: kind}
	}
	for _, attempt := range attempts {
		group, ok := groups[attempt.Kind]
		if !ok {
			continue
		}
		group.Total++
		if attempt.Status == "" || attempt.Status == model.StatusOK {
			continue
		}
		group.Failed++
		if group.FirstError == "" {
			group.FirstError = describeFailure(attempt)
		}
	}
	failures := &model.AttemptFailures{Groups: []model.AttemptFailureGroup{}}
	for _, kind := range order {
		if group := groups[kind]; group.Failed > 0 {
			failures.Failed += group.Failed
			failures.Groups = append(failures.Groups, *group)
		}
	}
	if failures.Failed == 0 {
		return nil
	}
	return failures
}

func describeFailure(attempt model.AttemptInput) string {
	text := strings.Join(strings.Fields(attempt.Error), " ")
	if text == "" {
		text = "status " + string(attempt.Status)
	}
	if runes := []rune(text); len(runes) > maxErrorLength {
		text = string(runes[:maxErrorLength]) + "…"
	}
	return text
}

func buildWarnings(report model.Report, in *model.ReportInput) []model.Warning {
	var warnings []model.Warning
	for _, sc := range report.Scenarios {
		if sc.Skill == nil || sc.Functional {
			continue
		}
		total := sc.Skill.Invoked + sc.Skill.NotInvoked + sc.Skill.Unknown
		label := scenarioLabelOf(sc)
		switch {
		case !sc.Skill.Exercised:
			warnings = append(warnings, model.Warning{
				Code: model.WarnSkillNotExercised, Scenario: label,
				Message: fmt.Sprintf("%s: the skill was not invoked in any with-arm run (0 of %d); the scenario compared the agent with itself and is excluded from the headline With, Without and Delta.", label, total),
			})
		case sc.Skill.NotInvoked > 0 && sc.Skill.Invoked > 0:
			warnings = append(warnings, model.Warning{
				Code: WarnSkillPartial, Scenario: label,
				Message: fmt.Sprintf("%s: the skill was invoked in only %d of %d with-arm runs; the with score mixes runs that never saw it.", label, sc.Skill.Invoked, total),
			})
		}
		if sc.Skill.Unknown > 0 {
			warnings = append(warnings, model.Warning{
				Code: model.WarnSkillUnknown, Scenario: label,
				Message: fmt.Sprintf("%s: skill use could not be determined for %d of %d with-arm runs (agent log missing or in an unrecognised format).", label, sc.Skill.Unknown, total),
			})
		}
	}

	if name, same := model.SameJudgeModel(
		report.Metadata.AgentCmd, report.Metadata.Model,
		report.Metadata.JudgeCmd, report.Metadata.JudgeModel); same {
		warnings = append(warnings, model.Warning{
			Code: model.WarnSameModelJudge, Message: model.SameModelWarning(name),
		})
	}

	if failures := report.AttemptFailures; failures != nil {
		parts := make([]string, 0, len(failures.Groups))
		for _, group := range failures.Groups {
			parts = append(parts, fmt.Sprintf("%d of %d %s attempts failed (first error: %s)",
				group.Failed, group.Total, group.Kind, group.FirstError))
		}
		warnings = append(warnings, model.Warning{
			Code:    model.WarnAttemptFailures,
			Message: "Failed attempts were retried or superseded and are not visible in the scores: " + strings.Join(parts, "; ") + ".",
		})
	}

	if report.Totals.N > 0 && report.Totals.N < 3 && hasComparison(report.Scenarios) {
		warnings = append(warnings, model.Warning{
			Code: model.WarnLowN,
			Message: fmt.Sprintf("n=%d: fewer than 3 runs per arm, so run-to-run variance cannot be estimated reliably; treat the delta as indicative, not conclusive.",
				report.Totals.N),
		})
	}
	return warnings
}

func hasComparison(scenarios []model.ScenarioResult) bool {
	for _, sc := range scenarios {
		if !sc.Functional {
			return true
		}
	}
	return false
}
