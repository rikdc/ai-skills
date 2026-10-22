package report

import (
	"github.com/rikdc/ai-skills/internal/model"
)

func extractModel(agentCmd string) string { return model.ExtractModel(agentCmd) }

func buildCost(scenarios []model.ScenarioResult, totals model.Totals) model.CostReport {
	allWith := runsFor(scenarios, nil, "with")
	allWithout := runsFor(scenarios, nil, "without")
	comparisonWith := runsFor(scenarios, func(sc model.ScenarioResult) bool { return !sc.Functional }, "with")
	comparisonWithout := runsFor(scenarios, func(sc model.ScenarioResult) bool { return !sc.Functional }, "without")

	costWith := costStats(allWith)
	costWithout := costStats(allWithout)
	compWith := costStats(comparisonWith)
	compWithout := costStats(comparisonWithout)

	deltaTotals := &model.CostDeltaTotals{
		AgentCost:               diff(compWith.Agent.Cost.Total, compWithout.Agent.Cost.Total),
		JudgeCost:               diff(compWith.Judge.Cost.Total, compWithout.Judge.Cost.Total),
		CombinedCost:            diff(compWith.Combined.Cost.Total, compWithout.Combined.Cost.Total),
		CombinedInputTokens:     diff(compWith.Combined.Tokens.Input.Total, compWithout.Combined.Tokens.Input.Total),
		CombinedOutputTokens:    diff(compWith.Combined.Tokens.Output.Total, compWithout.Combined.Tokens.Output.Total),
		CombinedReasoningTokens: diff(compWith.Combined.Tokens.Reasoning.Total, compWithout.Combined.Tokens.Reasoning.Total),
		CombinedCacheReadTokens: diff(compWith.Combined.Tokens.CacheRead.Total, compWithout.Combined.Tokens.CacheRead.Total),
		CombinedTokens:          diff(compWith.Combined.Tokens.Total, compWithout.Combined.Tokens.Total),
	}
	delta := &model.CostDelta{
		AgentCost:               diff(compWith.Agent.Cost.Mean, compWithout.Agent.Cost.Mean),
		JudgeCost:               diff(compWith.Judge.Cost.Mean, compWithout.Judge.Cost.Mean),
		CombinedCost:            diff(compWith.Combined.Cost.Mean, compWithout.Combined.Cost.Mean),
		AgentTokens:             diff(compWith.Agent.Tokens.Mean, compWithout.Agent.Tokens.Mean),
		JudgeTokens:             diff(compWith.Judge.Tokens.Mean, compWithout.Judge.Tokens.Mean),
		CombinedTokens:          diff(compWith.Combined.Tokens.Mean, compWithout.Combined.Tokens.Mean),
		AgentInputTokens:        diff(compWith.Agent.Tokens.Input.Mean, compWithout.Agent.Tokens.Input.Mean),
		AgentOutputTokens:       diff(compWith.Agent.Tokens.Output.Mean, compWithout.Agent.Tokens.Output.Mean),
		AgentReasoningTokens:    diff(compWith.Agent.Tokens.Reasoning.Mean, compWithout.Agent.Tokens.Reasoning.Mean),
		AgentCacheReadTokens:    diff(compWith.Agent.Tokens.CacheRead.Mean, compWithout.Agent.Tokens.CacheRead.Mean),
		JudgeInputTokens:        diff(compWith.Judge.Tokens.Input.Mean, compWithout.Judge.Tokens.Input.Mean),
		JudgeOutputTokens:       diff(compWith.Judge.Tokens.Output.Mean, compWithout.Judge.Tokens.Output.Mean),
		JudgeReasoningTokens:    diff(compWith.Judge.Tokens.Reasoning.Mean, compWithout.Judge.Tokens.Reasoning.Mean),
		JudgeCacheReadTokens:    diff(compWith.Judge.Tokens.CacheRead.Mean, compWithout.Judge.Tokens.CacheRead.Mean),
		CombinedInputTokens:     diff(compWith.Combined.Tokens.Input.Mean, compWithout.Combined.Tokens.Input.Mean),
		CombinedOutputTokens:    diff(compWith.Combined.Tokens.Output.Mean, compWithout.Combined.Tokens.Output.Mean),
		CombinedReasoningTokens: diff(compWith.Combined.Tokens.Reasoning.Mean, compWithout.Combined.Tokens.Reasoning.Mean),
		CombinedCacheReadTokens: diff(compWith.Combined.Tokens.CacheRead.Mean, compWithout.Combined.Tokens.CacheRead.Mean),
	}

	var costPerPoint *float64
	if compWith.Combined.Cost.Total != nil && compWithout.Combined.Cost.Total != nil &&
		totals.Delta != nil && *totals.Delta > Epsilon &&
		delta.CombinedCost != nil && *delta.CombinedCost > 0 {
		value := *delta.CombinedCost / *totals.Delta
		costPerPoint = &value
	}

	return model.CostReport{
		Currency: "USD",
		With:     costWith,
		Without:  costWithout,
		Comparison: &model.CostComparison{
			With:    compWith,
			Without: compWithout,
		},
		DeltaTotals:       deltaTotals,
		Delta:             delta,
		CostPerDeltaPoint: costPerPoint,
	}
}

func runsFor(scenarios []model.ScenarioResult, include func(model.ScenarioResult) bool, arm string) []model.RunRow {
	var out []model.RunRow
	for _, sc := range scenarios {
		if include != nil && !include(sc) {
			continue
		}
		for _, run := range sc.Runs {
			if run.Arm == arm {
				out = append(out, run)
			}
		}
	}
	return out
}

func costStats(runs []model.RunRow) model.CostStats {
	stats := model.CostStats{RunsTotal: len(runs)}
	stats.Agent = fieldStats(runs, func(run model.RunRow) model.Usage { return run.AgentUsage })
	stats.Judge = fieldStats(runs, func(run model.RunRow) model.Usage { return run.JudgeUsage })
	stats.Combined = combinedStats(runs)
	return stats
}

func fieldStats(runs []model.RunRow, get func(model.RunRow) model.Usage) model.FieldStats {
	stats := model.FieldStats{RunsTotal: len(runs)}
	var costs, totals []float64
	var inputs, outputs, reasonings, reads, writes []float64
	applicable := 0
	for _, run := range runs {
		usage := get(run)
		if usage.NormalizeState().State == model.UsageNotApplicable {
			continue
		}
		applicable++
		if usageKnown(usage) {
			stats.RunsKnown++
		}
		if cost := usageMetric(usage, usage.CostUSD); cost != nil {
			costs = append(costs, *cost)
		}
		if value := usageIntMetric(usage, usage.TotalTokens); value != nil {
			totals = append(totals, *value)
		}
		if value := usageIntMetric(usage, usage.InputTokens); value != nil {
			inputs = append(inputs, *value)
		}
		if value := usageIntMetric(usage, usage.OutputTokens); value != nil {
			outputs = append(outputs, *value)
		}
		if value := usageIntMetric(usage, usage.ReasoningTokens); value != nil {
			reasonings = append(reasonings, *value)
		}
		if value := usageIntMetric(usage, usage.CacheReadTokens); value != nil {
			reads = append(reads, *value)
		}
		if value := usageIntMetric(usage, usage.CacheWriteTokens); value != nil {
			writes = append(writes, *value)
		}
	}
	stats.RunsTotal = applicable
	stats.Cost = metricStats(costs, applicable)
	stats.Tokens = tokenStats(totals, inputs, outputs, reasonings, reads, writes, applicable)
	return stats
}

// metricStats summarises one metric over the runs where the usage applies (a
// failed run or one that needed no judge has none). The mean covers the runs
// that reported it (runs_known of runs_total); the total is present only when
// every applicable run reported it, so a sum over a subset never reads as a
// total.
func metricStats(values []float64, runsTotal int) model.MetricStats {
	out := model.MetricStats{RunsKnown: len(values), RunsTotal: runsTotal}
	if len(values) == 0 {
		return out
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	mean := total / float64(len(values))
	out.Mean = &mean
	if len(values) == runsTotal {
		out.Total = &total
	}
	return out
}

func tokenStats(totals, inputs, outputs, reasonings, reads, writes []float64, runsTotal int) model.TokenStats {
	return model.TokenStats{
		MetricStats: metricStats(totals, runsTotal),
		Input:       metricStats(inputs, runsTotal),
		Output:      metricStats(outputs, runsTotal),
		Reasoning:   metricStats(reasonings, runsTotal),
		CacheRead:   metricStats(reads, runsTotal),
		CacheWrite:  metricStats(writes, runsTotal),
	}
}

// combinedStats summarises agent plus judge usage per run through
// model.Combine, so a run whose invocations do not report the same components
// is unknown here exactly as in the suite summary.
func combinedStats(runs []model.RunRow) model.CombinedStats {
	stats := model.CombinedStats{RunsTotal: len(runs)}
	var costs, totals, inputs, outputs, reasonings, reads, writes []float64
	applicable := 0
	add := func(dst *[]float64, value *int64) {
		if value != nil {
			*dst = append(*dst, float64(*value))
		}
	}
	for _, run := range runs {
		combined := model.Combine(run.AgentUsage, run.JudgeUsage)
		if combined.State == model.UsageNotApplicable {
			continue
		}
		applicable++
		if combined.State != model.UsageKnown {
			continue
		}
		if combined.CostUSD != nil {
			costs = append(costs, *combined.CostUSD)
		}
		add(&totals, combined.TotalTokens)
		add(&inputs, combined.InputTokens)
		add(&outputs, combined.OutputTokens)
		add(&reasonings, combined.ReasoningTokens)
		add(&reads, combined.CacheReadTokens)
		add(&writes, combined.CacheWriteTokens)
	}
	stats.RunsKnown = len(costs)
	stats.RunsTotal = applicable
	stats.Cost = metricStats(costs, applicable)
	stats.Tokens = tokenStats(totals, inputs, outputs, reasonings, reads, writes, applicable)
	return stats
}

func combinedTokenMetric(agent, judge model.Usage, get func(model.Usage) *int64) *float64 {
	convert := func(u model.Usage) *float64 {
		value := get(u)
		if value == nil {
			return nil
		}
		f := float64(*value)
		return &f
	}
	return combinedMetric(agent, judge, convert)
}

func usageIntMetric(usage model.Usage, value *int64) *float64 {
	if !usageKnown(usage) || value == nil {
		return nil
	}
	f := float64(*value)
	return &f
}

// combinedMetric reads one metric of agent plus judge usage combined by
// model.Combine; it is nil unless the combination is known.
func combinedMetric(agent, judge model.Usage, get func(model.Usage) *float64) *float64 {
	combined := model.Combine(agent, judge)
	return usageMetric(combined, get(combined))
}

func usageKnown(usage model.Usage) bool {
	return usage.NormalizeState().State == model.UsageKnown
}

func usageMetric(usage model.Usage, value *float64) *float64 {
	if !usageKnown(usage) {
		return nil
	}
	return value
}

func buildExpenditure(attempts []model.AttemptInput) model.Expenditure {
	out := model.Expenditure{Currency: "USD"}
	usages := make([]model.Usage, 0, len(attempts))
	for _, attempt := range attempts {
		out.TotalAttempts++
		switch attempt.Kind {
		case model.KindAgent:
			out.AgentAttempts++
		case model.KindHook:
			out.HookAttempts++
		case model.KindJudge:
			out.JudgeAttempts++
		}
		usage := attempt.Usage.NormalizeState()
		usages = append(usages, usage)
		switch usage.State {
		case model.UsageNotApplicable:
			out.NotApplicableAttempts++
		case model.UsageKnown:
			out.KnownAttempts++
		default:
			out.UnknownAttempts++
		}
	}
	// The known total includes partial components of unknown attempts; it is
	// complete only when every applicable attempt reports every component.
	known, complete := model.SumLowerBound("expenditure", usages)
	out.Complete = complete
	if complete {
		out.KnownTotal = known
	} else {
		out.KnownTotal = known.WithState(model.UsageUnknown)
	}
	out.KnownTotal.Currency = "USD"
	return out
}

func addFloat(current *float64, value float64) *float64 {
	if current == nil {
		v := value
		return &v
	}
	v := *current + value
	return &v
}

func addInt(current *int64, value int64) *int64 {
	if current == nil {
		v := value
		return &v
	}
	v := *current + value
	return &v
}
