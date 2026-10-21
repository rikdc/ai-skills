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
	if compWith.Combined.Cost.RunsKnown == compWith.RunsTotal &&
		compWithout.Combined.Cost.RunsKnown == compWithout.RunsTotal &&
		compWith.RunsTotal > 0 && compWithout.RunsTotal > 0 &&
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
	for _, run := range runs {
		usage := get(run)
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
	stats.Cost = metricStats(costs, len(runs))
	stats.Tokens = tokenStats(totals, inputs, outputs, reasonings, reads, writes, len(runs))
	return stats
}

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
	out.Total = &total
	out.Mean = &mean
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

func combinedStats(runs []model.RunRow) model.CombinedStats {
	stats := model.CombinedStats{RunsTotal: len(runs)}
	var costs, totals, inputs, outputs, reasonings, reads, writes []float64
	for _, run := range runs {
		agent, judge := run.AgentUsage, run.JudgeUsage
		if cost := combinedMetric(agent, judge, func(u model.Usage) *float64 { return u.CostUSD }); cost != nil {
			costs = append(costs, *cost)
		}
		if value := combinedTokenMetric(agent, judge, func(u model.Usage) *int64 { return u.TotalTokens }); value != nil {
			totals = append(totals, *value)
		}
		if value := combinedTokenMetric(agent, judge, func(u model.Usage) *int64 { return u.InputTokens }); value != nil {
			inputs = append(inputs, *value)
		}
		if value := combinedTokenMetric(agent, judge, func(u model.Usage) *int64 { return u.OutputTokens }); value != nil {
			outputs = append(outputs, *value)
		}
		if value := combinedTokenMetric(agent, judge, func(u model.Usage) *int64 { return u.ReasoningTokens }); value != nil {
			reasonings = append(reasonings, *value)
		}
		if value := combinedTokenMetric(agent, judge, func(u model.Usage) *int64 { return u.CacheReadTokens }); value != nil {
			reads = append(reads, *value)
		}
		if value := combinedTokenMetric(agent, judge, func(u model.Usage) *int64 { return u.CacheWriteTokens }); value != nil {
			writes = append(writes, *value)
		}
	}
	stats.RunsKnown = len(costs)
	stats.Cost = metricStats(costs, len(runs))
	stats.Tokens = tokenStats(totals, inputs, outputs, reasonings, reads, writes, len(runs))
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

func combinedMetric(agent, judge model.Usage, get func(model.Usage) *float64) *float64 {
	if judge.State == model.UsageNotApplicable {
		return usageMetric(agent, get(agent))
	}
	if agent.State == model.UsageNotApplicable {
		return usageMetric(judge, get(judge))
	}
	a := usageMetric(agent, get(agent))
	j := usageMetric(judge, get(judge))
	if a == nil || j == nil {
		return nil
	}
	value := *a + *j
	return &value
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
	known := model.Usage{State: model.UsageUnknown, Known: false, Source: "expenditure"}
	known.Currency = "USD"
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
		switch usage.State {
		case model.UsageNotApplicable:
			out.NotApplicableAttempts++
			continue
		case model.UsageKnown:
			out.KnownAttempts++
		default:
			out.UnknownAttempts++
		}
		// Known totals include every numeric component, including a partial
		// component carried by an otherwise unknown attempt.
		if usage.CostUSD != nil {
			known.CostUSD = addFloat(known.CostUSD, *usage.CostUSD)
		}
		if usage.InputTokens != nil {
			known.InputTokens = addInt(known.InputTokens, *usage.InputTokens)
		}
		if usage.OutputTokens != nil {
			known.OutputTokens = addInt(known.OutputTokens, *usage.OutputTokens)
		}
		if usage.ReasoningTokens != nil {
			known.ReasoningTokens = addInt(known.ReasoningTokens, *usage.ReasoningTokens)
		}
		if usage.CacheReadTokens != nil {
			known.CacheReadTokens = addInt(known.CacheReadTokens, *usage.CacheReadTokens)
		}
		if usage.CacheWriteTokens != nil {
			known.CacheWriteTokens = addInt(known.CacheWriteTokens, *usage.CacheWriteTokens)
		}
		if usage.TotalTokens != nil {
			known.TotalTokens = addInt(known.TotalTokens, *usage.TotalTokens)
		}
	}
	out.Complete = out.UnknownAttempts == 0
	if out.Complete {
		known = known.WithState(model.UsageKnown)
	} else {
		known = known.WithState(model.UsageUnknown)
	}
	out.KnownTotal = known
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
