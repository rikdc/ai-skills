package report

import (
	"sort"

	"github.com/rikdc/ai-skills/internal/model"
)

// Compare builds the baseline comparison block. Only scenarios with the same
// plugin, name, and rubric (sorted name/max pairs) that are ok on both sides
// contribute. Every other scenario is listed with a reason.
func Compare(current, baseline model.Report) *model.Comparison {
	type match struct {
		current  model.ScenarioResult
		baseline model.ScenarioResult
	}
	var matches []match
	var excluded []model.ComparisonExclusion

	findBaseline := func(plugin, scenario string) *model.ScenarioResult {
		for i := range baseline.Scenarios {
			if baseline.Scenarios[i].Plugin == plugin && baseline.Scenarios[i].Scenario == scenario {
				return &baseline.Scenarios[i]
			}
		}
		return nil
	}

	for _, sc := range current.Scenarios {
		old := findBaseline(sc.Plugin, sc.Scenario)
		switch {
		case old == nil:
			excluded = append(excluded, exclusion("current", sc, "missing from baseline"))
		case !sameRubric(sc, *old):
			excluded = append(excluded, exclusion("current", sc, "rubric changed"))
		case sc.Status != "ok":
			excluded = append(excluded, exclusion("current", sc, "not comparable in current"))
		case old.Status != "ok":
			excluded = append(excluded, exclusion("current", sc, "not comparable in baseline"))
		default:
			matches = append(matches, match{current: sc, baseline: *old})
		}
	}
	for _, old := range baseline.Scenarios {
		found := false
		for _, sc := range current.Scenarios {
			if sc.Plugin == old.Plugin && sc.Scenario == old.Scenario {
				found = true
				break
			}
		}
		if !found {
			excluded = append(excluded, exclusion("baseline", old, "missing from current"))
		}
	}

	matchedCurrent := make([]model.ScenarioResult, 0, len(matches))
	matchedBaseline := make([]model.ScenarioResult, 0, len(matches))
	for _, m := range matches {
		matchedCurrent = append(matchedCurrent, m.current)
		matchedBaseline = append(matchedBaseline, m.baseline)
	}
	comparable := len(matches) > 0

	currentSide := comparisonSide(current, matchedCurrent, len(current.Scenarios))
	baselineSide := comparisonSide(baseline, matchedBaseline, len(baseline.Scenarios))
	comparison := &model.Comparison{
		Status:              "not comparable",
		Comparable:          comparable,
		ComparableScenarios: len(matches),
		CurrentScenarios:    len(current.Scenarios),
		BaselineScenarios:   len(baseline.Scenarios),
		ExcludedScenarios:   len(excluded),
		Excluded:            excluded,
		Baseline:            baselineSide,
		Current:             currentSide,
		Delta:               &model.ComparisonDelta{},
		Scenarios:           []model.ComparisonScenario{},
	}
	if comparable {
		comparison.Status = "comparable"
	}
	delta := comparison.Delta
	delta.WithScore = diff(currentSide.With, baselineSide.With)
	delta.WithoutScore = diff(currentSide.Without, baselineSide.Without)
	delta.Effect = diff(currentSide.Delta, baselineSide.Delta)
	delta.AgentCostPerRun = diff(currentSide.Cost.AgentPerRun, baselineSide.Cost.AgentPerRun)
	delta.JudgeCostPerRun = diff(currentSide.Cost.JudgePerRun, baselineSide.Cost.JudgePerRun)
	delta.WithCostPerRun = diff(currentSide.Cost.WithPerRun, baselineSide.Cost.WithPerRun)
	delta.WithoutCostPerRun = diff(currentSide.Cost.WithoutPerRun, baselineSide.Cost.WithoutPerRun)
	delta.PairCostPerRun = diff(currentSide.Cost.PairPerRun, baselineSide.Cost.PairPerRun)
	delta.WithInputTokensPerRun = diff(currentSide.Tokens.WithInputPerRun, baselineSide.Tokens.WithInputPerRun)
	delta.WithOutputTokensPerRun = diff(currentSide.Tokens.WithOutputPerRun, baselineSide.Tokens.WithOutputPerRun)
	delta.WithTotalTokensPerRun = diff(currentSide.Tokens.WithTotalPerRun, baselineSide.Tokens.WithTotalPerRun)
	delta.WithoutInputTokensPerRun = diff(currentSide.Tokens.WithoutInputPerRun, baselineSide.Tokens.WithoutInputPerRun)
	delta.WithoutOutputTokensPerRun = diff(currentSide.Tokens.WithoutOutputPerRun, baselineSide.Tokens.WithoutOutputPerRun)
	delta.WithoutTotalTokensPerRun = diff(currentSide.Tokens.WithoutTotalPerRun, baselineSide.Tokens.WithoutTotalPerRun)
	delta.WallSeconds = diff(currentSide.Timing.WallSeconds, baselineSide.Timing.WallSeconds)
	delta.AgentSeconds = diff(currentSide.Timing.AgentSeconds, baselineSide.Timing.AgentSeconds)
	delta.JudgeSeconds = diff(currentSide.Timing.JudgeSeconds, baselineSide.Timing.JudgeSeconds)

	for _, m := range matches {
		cur := scenarioComparisonSide(m.current)
		old := scenarioComparisonSide(m.baseline)
		deltaSide := model.ComparisonMetric{
			With:                diff(cur.With, old.With),
			Without:             diff(cur.Without, old.Without),
			Delta:               diff(cur.Delta, old.Delta),
			CostWithPerRun:      diff(cur.CostWithPerRun, old.CostWithPerRun),
			CostWithoutPerRun:   diff(cur.CostWithoutPerRun, old.CostWithoutPerRun),
			InputWithPerRun:     diff(cur.InputWithPerRun, old.InputWithPerRun),
			OutputWithPerRun:    diff(cur.OutputWithPerRun, old.OutputWithPerRun),
			InputWithoutPerRun:  diff(cur.InputWithoutPerRun, old.InputWithoutPerRun),
			OutputWithoutPerRun: diff(cur.OutputWithoutPerRun, old.OutputWithoutPerRun),
		}
		comparison.Scenarios = append(comparison.Scenarios, model.ComparisonScenario{
			Plugin:   m.current.Plugin,
			Scenario: m.current.Scenario,
			Current:  cur,
			Baseline: old,
			Delta:    deltaSide,
		})
	}

	if current.Metadata.Backend != baseline.Metadata.Backend ||
		current.Metadata.Model != baseline.Metadata.Model {
		comparison.MeasurementWarning = "backend or model changed; deltas are not a controlled measurement"
	}
	return comparison
}

func exclusion(side string, sc model.ScenarioResult, reason string) model.ComparisonExclusion {
	return model.ComparisonExclusion{
		Side: side, Plugin: sc.Plugin, Scenario: sc.Scenario, Reason: reason,
	}
}

type rubricPair struct {
	name string
	max  float64
}

func sameRubric(a, b model.ScenarioResult) bool {
	ar, br := rubricPairs(a), rubricPairs(b)
	if len(ar) != len(br) {
		return false
	}
	for i := range ar {
		if ar[i] != br[i] {
			return false
		}
	}
	return true
}

func rubricPairs(sc model.ScenarioResult) []rubricPair {
	pairs := make([]rubricPair, 0, len(sc.Criteria))
	for _, criterion := range sc.Criteria {
		pairs = append(pairs, rubricPair{name: criterion.Name, max: criterion.Max})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].name != pairs[j].name {
			return pairs[i].name < pairs[j].name
		}
		return pairs[i].max < pairs[j].max
	})
	return pairs
}

func comparisonSide(report model.Report, scenarios []model.ScenarioResult, totalScenarios int) *model.ComparisonSide {
	var withs, withouts, deltas []float64
	for _, sc := range scenarios {
		if sc.Arms.With.PercentMean != nil {
			withs = append(withs, *sc.Arms.With.PercentMean)
		}
		if sc.Arms.Without.PercentMean != nil {
			withouts = append(withouts, *sc.Arms.Without.PercentMean)
		}
		if sc.Delta != nil {
			deltas = append(deltas, *sc.Delta)
		}
	}
	side := &model.ComparisonSide{
		SuiteDir:  report.Metadata.SuiteDir,
		CreatedAt: report.Metadata.CreatedAt,
		GitSHA:    report.Metadata.GitSHA,
		N:         report.Metadata.N,
		Verdict:   report.Verdict.Status,
		With:      mean(withs),
		Without:   mean(withouts),
		Delta:     mean(deltas),
	}
	side.Cost.AgentPerRun = matchedArmCost(scenarios, "with", func(run model.RunRow) *float64 {
		return usageMetric(run.AgentUsage, run.AgentUsage.CostUSD)
	})
	side.Cost.JudgePerRun = matchedArmCost(scenarios, "with", func(run model.RunRow) *float64 {
		return usageMetric(run.JudgeUsage, run.JudgeUsage.CostUSD)
	})
	side.Cost.WithPerRun = matchedArmCost(scenarios, "with", runCost)
	side.Cost.WithoutPerRun = matchedArmCost(scenarios, "without", runCost)
	if side.Cost.WithPerRun != nil && side.Cost.WithoutPerRun != nil {
		value := *side.Cost.WithPerRun + *side.Cost.WithoutPerRun
		side.Cost.PairPerRun = &value
	}
	side.Tokens.WithInputPerRun = matchedArmToken(scenarios, "with", func(u model.Usage) *int64 { return u.InputTokens })
	side.Tokens.WithOutputPerRun = matchedArmToken(scenarios, "with", func(u model.Usage) *int64 { return u.OutputTokens })
	side.Tokens.WithTotalPerRun = matchedArmToken(scenarios, "with", func(u model.Usage) *int64 { return u.TotalTokens })
	side.Tokens.WithoutInputPerRun = matchedArmToken(scenarios, "without", func(u model.Usage) *int64 { return u.InputTokens })
	side.Tokens.WithoutOutputPerRun = matchedArmToken(scenarios, "without", func(u model.Usage) *int64 { return u.OutputTokens })
	side.Tokens.WithoutTotalPerRun = matchedArmToken(scenarios, "without", func(u model.Usage) *int64 { return u.TotalTokens })
	if side.Tokens.WithInputPerRun != nil && side.Tokens.WithoutInputPerRun != nil {
		v := *side.Tokens.WithInputPerRun + *side.Tokens.WithoutInputPerRun
		side.Tokens.PairInputPerRun = &v
	}
	if side.Tokens.WithOutputPerRun != nil && side.Tokens.WithoutOutputPerRun != nil {
		v := *side.Tokens.WithOutputPerRun + *side.Tokens.WithoutOutputPerRun
		side.Tokens.PairOutputPerRun = &v
	}
	if side.Tokens.WithTotalPerRun != nil && side.Tokens.WithoutTotalPerRun != nil {
		v := *side.Tokens.WithTotalPerRun + *side.Tokens.WithoutTotalPerRun
		side.Tokens.PairTotalPerRun = &v
	}
	if len(scenarios) == totalScenarios {
		side.Timing.WallSeconds = report.Timing.WallSeconds
	}
	side.Timing.AgentSeconds = timingTotal(scenarios, func(stats model.ArmStats) *float64 {
		return stats.Timing.AgentSeconds
	})
	side.Timing.JudgeSeconds = timingTotal(scenarios, func(stats model.ArmStats) *float64 {
		return stats.Timing.JudgeSeconds
	})
	return side
}

func matchedArmCost(scenarios []model.ScenarioResult, arm string, get func(model.RunRow) *float64) *float64 {
	var values []float64
	for _, sc := range scenarios {
		for _, run := range sc.Runs {
			if run.Arm != arm {
				continue
			}
			if value := get(run); value != nil {
				values = append(values, *value)
			}
		}
	}
	return mean(values)
}

func matchedArmToken(scenarios []model.ScenarioResult, arm string, get func(model.Usage) *int64) *float64 {
	return matchedArmCost(scenarios, arm, func(run model.RunRow) *float64 {
		return combinedTokenMetric(run.AgentUsage, run.JudgeUsage, get)
	})
}

func runCost(run model.RunRow) *float64 {
	a := usageMetric(run.AgentUsage, run.AgentUsage.CostUSD)
	j := usageMetric(run.JudgeUsage, run.JudgeUsage.CostUSD)
	if run.JudgeUsage.State == model.UsageNotApplicable {
		return a
	}
	if run.AgentUsage.State == model.UsageNotApplicable {
		return j
	}
	if a == nil || j == nil {
		return nil
	}
	value := *a + *j
	return &value
}

func timingTotal(scenarios []model.ScenarioResult, get func(model.ArmStats) *float64) *float64 {
	var values []float64
	for _, sc := range scenarios {
		for _, arm := range []model.ArmStats{sc.Arms.With, sc.Arms.Without} {
			if value := get(arm); value != nil {
				values = append(values, *value)
			}
		}
	}
	if len(values) == 0 {
		return nil
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	return &total
}

func scenarioComparisonSide(sc model.ScenarioResult) model.ComparisonMetric {
	out := model.ComparisonMetric{
		With:    sc.Arms.With.PercentMean,
		Without: sc.Arms.Without.PercentMean,
		Delta:   sc.Delta,
	}
	out.CostWithPerRun = armCostFor(sc, "with")
	out.CostWithoutPerRun = armCostFor(sc, "without")
	out.InputWithPerRun = armTokenFor(sc, "with", func(u model.Usage) *int64 { return u.InputTokens })
	out.OutputWithPerRun = armTokenFor(sc, "with", func(u model.Usage) *int64 { return u.OutputTokens })
	out.InputWithoutPerRun = armTokenFor(sc, "without", func(u model.Usage) *int64 { return u.InputTokens })
	out.OutputWithoutPerRun = armTokenFor(sc, "without", func(u model.Usage) *int64 { return u.OutputTokens })
	return out
}

func armCostFor(sc model.ScenarioResult, arm string) *float64 {
	var values []float64
	for _, run := range sc.Runs {
		if run.Arm != arm {
			continue
		}
		if value := runCost(run); value != nil {
			values = append(values, *value)
		}
	}
	return mean(values)
}

func armTokenFor(sc model.ScenarioResult, arm string, get func(model.Usage) *int64) *float64 {
	var values []float64
	for _, run := range sc.Runs {
		if run.Arm != arm {
			continue
		}
		if value := combinedTokenMetric(run.AgentUsage, run.JudgeUsage, get); value != nil {
			values = append(values, *value)
		}
	}
	return mean(values)
}
