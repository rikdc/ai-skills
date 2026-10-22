// Package report performs the pure analysis and rendering of a loaded eval
// suite. It imports only internal/model: all filesystem reads live in
// internal/reportio, and no renderer recomputes scores, deltas, or verdicts.
package report

import (
	"math"
	"sort"

	"github.com/rikdc/ai-skills/internal/model"
)

// Epsilon is the report comparison threshold.
const Epsilon = 0.0001

// Build analyses a loaded suite into the single report document.
func Build(in *model.ReportInput) model.Report {
	report := model.Report{
		Version: model.SchemaVersion,
		Metadata: model.ReportMetadata{
			SuiteDir:         in.SuiteDir,
			AgentCmd:         in.Manifest.Config.AgentCmd,
			JudgeCmd:         firstNonEmpty(in.Manifest.Config.Judge.Cmd, ""),
			Model:            firstNonEmpty(in.Manifest.Config.Model, extractModel(in.Manifest.Config.AgentCmd)),
			Backend:          in.Manifest.Config.Backend,
			JudgeModel:       in.Manifest.Config.Judge.Model,
			GitSHA:           in.Manifest.Repo.GitSHA,
			GitDirty:         in.Manifest.Repo.GitDirty,
			CreatedAt:        in.Manifest.CreatedAt,
			Arm:              in.Manifest.Config.Arm,
			N:                in.Manifest.Config.Runs,
			AgentInvocations: in.State.Counts["agent_invocations"],
			Counts:           in.State.Counts,
		},
		Links: model.Links{
			JSON:     "report.json",
			Markdown: "report.md",
			HTML:     "report.html",
		},
	}

	var units []string
	plugins := map[string]bool{}
	for _, sc := range in.Scenarios {
		plugins[sc.Plugin] = true
		units = append(units, sc.Units...)
		report.Scenarios = append(report.Scenarios, buildScenario(sc, in.Manifest.Config.Runs, in.Manifest.Config.Arm))
	}
	report.Metadata.Units = uniqueSorted(units)
	if len(in.Manifest.Config.Units) > 0 {
		report.Metadata.Label = join(in.Manifest.Config.Units, ", ")
	} else {
		var names []string
		for plugin := range plugins {
			names = append(names, plugin)
		}
		sort.Strings(names)
		report.Metadata.Label = join(names, ", ")
	}
	if report.Metadata.Label == "" {
		report.Metadata.Label = "all plugins"
	}

	report.Totals = buildTotals(report.Scenarios, in)
	report.Timing = buildTiming(report.Scenarios, in)
	report.Cost = buildCost(report.Scenarios, report.Totals)
	report.Usage = model.CostReport{
		Currency: report.Cost.Currency,
		With:     report.Cost.With,
		Without:  report.Cost.Without,
		Delta:    report.Cost.Delta,
	}
	report.Criteria = flattenCriteria(report.Scenarios)
	report.Expenditure = buildExpenditure(in.Attempts)
	report.Verdict = buildVerdict(report.Totals)
	for _, sc := range report.Scenarios {
		for _, run := range sc.Runs {
			if run.Evidence != "" {
				report.Links.Evidence = append(report.Links.Evidence, model.EvidenceLink{
					Plugin: sc.Plugin, Scenario: sc.Scenario, Path: run.Evidence,
				})
				break
			}
		}
	}
	return report
}

func buildScenario(sc model.ScenarioInput, configuredRuns int, arm string) model.ScenarioResult {
	functional := sc.Mode == string(model.ModeFunctional)
	out := model.ScenarioResult{
		Plugin:         sc.Plugin,
		Scenario:       sc.Scenario,
		Units:          append([]string(nil), sc.Units...),
		Mode:           sc.Mode,
		Functional:     functional,
		Denominator:    denominator(sc.Rubric),
		CriteriaCount:  len(sc.Rubric),
		ConfiguredRuns: configuredRuns,
	}
	if out.Units == nil {
		out.Units = []string{}
	}
	out.ArmCount = 2
	if functional || (arm != "" && arm != "both") {
		out.ArmCount = 1
	}
	names := rubricNames(sc.Rubric)
	runs := make([]model.RunRow, 0, len(sc.Runs))
	for _, ri := range sc.Runs {
		runs = append(runs, buildRun(ri, sc.Rubric, names, out.Denominator))
	}
	out.Runs = runs
	out.Arms.With = armStats(runs, "with")
	out.Arms.Without = armStats(runs, "without")
	if !functional && out.Arms.With.Runs > 0 && out.Arms.Without.Runs > 0 {
		out.Delta = diff(out.Arms.With.PercentMean, out.Arms.Without.PercentMean)
	}
	for _, run := range runs {
		out.PendingPoints += run.PendingPoints
		out.PendingCriteria += run.PendingCount
	}
	out.Criteria = buildCriteria(sc.Rubric, runs, functional)
	for _, criterion := range out.Criteria {
		if criterion.Delta != nil && *criterion.Delta < -Epsilon {
			out.Regressions = append(out.Regressions, criterion)
		}
	}
	if out.Regressions == nil {
		out.Regressions = []model.CriterionResult{}
	}
	switch {
	case anyRunFailed(runs):
		out.Status = "runner-failed"
	case functional:
		out.Status = "functional"
	case out.Arms.With.Runs == 0 || out.Arms.Without.Runs == 0:
		out.Status = "not-comparable"
	default:
		out.Status = "ok"
	}
	return out
}

func buildRun(ri model.RunInput, rubric []model.Criterion, names map[string]bool, denominator float64) model.RunRow {
	scored := map[string]float64{}
	for _, item := range ri.MechanicalItems {
		if names[item.Name] {
			scored[item.Name] = item.Score
		}
	}
	for _, item := range ri.JudgedItems {
		if names[item.Name] {
			scored[item.Name] = item.Score
		}
	}
	row := model.RunRow{
		Arm:                  ri.Arm,
		Run:                  ri.Run,
		Status:               ri.Status,
		StopReason:           ri.StopReason,
		AgentAttempt:         ri.AgentAttempt,
		JudgeAttempt:         ri.JudgeAttempt,
		Complete:             ri.Complete,
		AgentExit:            ri.AgentExit,
		TimedOut:             ri.TimedOut,
		DurationSeconds:      ri.DurationSeconds,
		JudgeDurationSeconds: ri.JudgeDurationSeconds,
		Mechanical:           model.ItemList{Items: nonNilItems(ri.MechanicalItems)},
		Usage:                ri.Usage,
		AgentUsage:           ri.AgentUsage,
		JudgeUsage:           ri.JudgeUsage,
		CombinedUsage:        ri.CombinedUsage,
		Evidence:             ri.Evidence,
		Max:                  denominator,
		Scored:               scored,
	}
	if ri.JudgedItems != nil {
		row.Judged = &model.ItemList{Items: ri.JudgedItems}
	}
	pendingPoints := 0.0
	pendingCount := 0
	for _, criterion := range rubric {
		if _, ok := scored[criterion.Name]; !ok {
			pendingPoints += criterion.MaxScore
			pendingCount++
		}
	}
	row.PendingPoints = pendingPoints
	row.PendingCount = pendingCount
	if ri.Status == string(model.StatusOK) {
		score := 0.0
		for _, value := range scored {
			score += value
		}
		row.Score = &score
		if denominator > 0 {
			percent := score * 100 / denominator
			row.Percent = &percent
		}
	}
	return row
}

func buildCriteria(rubric []model.Criterion, runs []model.RunRow, functional bool) []model.CriterionResult {
	withRuns := filterRuns(runs, "with")
	withoutRuns := filterRuns(runs, "without")
	out := make([]model.CriterionResult, 0, len(rubric))
	for _, item := range rubric {
		result := model.CriterionResult{Name: item.Name, Max: item.MaxScore}
		if functional {
			var values []float64
			pending := false
			for _, run := range withRuns {
				value, ok := run.Scored[item.Name]
				if !ok {
					pending = true
					continue
				}
				values = append(values, value)
			}
			result.With = mean(values)
			result.PairedRuns = len(withRuns)
			if pending {
				result.Status = "pending"
			} else {
				result.Status = "scored"
			}
			out = append(out, result)
			continue
		}
		var withValues, withoutValues, deltas []float64
		pending := false
		for _, run := range runs {
			if _, ok := run.Scored[item.Name]; !ok {
				pending = true
			}
		}
		for _, withRun := range withRuns {
			withoutRun := runByNumber(withoutRuns, withRun.Run)
			if withoutRun == nil {
				continue
			}
			w, okWith := withRun.Scored[item.Name]
			o, okWithout := withoutRun.Scored[item.Name]
			if !okWith || !okWithout {
				continue
			}
			withValues = append(withValues, w)
			withoutValues = append(withoutValues, o)
			deltas = append(deltas, w-o)
		}
		result.With = mean(withValues)
		result.Without = mean(withoutValues)
		result.Delta = mean(deltas)
		result.PairedRuns = len(deltas)
		switch {
		case pending:
			result.Status = "pending"
		case len(deltas) > 0:
			result.Status = "comparable"
		default:
			result.Status = "not comparable"
		}
		out = append(out, result)
	}
	return out
}

func armStats(runs []model.RunRow, arm string) model.ArmStats {
	filtered := filterRuns(runs, arm)
	stats := model.ArmStats{Runs: len(filtered)}
	var scores, percents []float64
	for _, run := range filtered {
		if run.Score != nil {
			scores = append(scores, *run.Score)
		}
		if run.Percent != nil {
			percents = append(percents, *run.Percent)
		}
		if run.Status != string(model.StatusOK) {
			stats.Failures++
		}
		stats.PendingPoints += run.PendingPoints
		if run.TimedOut {
			stats.Timing.TimedOutRuns++
		}
	}
	stats.ScoreMean = mean(scores)
	stats.PercentMean = mean(percents)
	stats.PercentMin = minValue(percents)
	stats.PercentMax = maxValue(percents)
	stats.Timing.AgentSeconds = sumPresent(filtered, func(r model.RunRow) *float64 { return r.DurationSeconds })
	stats.Timing.JudgeSeconds = sumPresent(filtered, func(r model.RunRow) *float64 { return r.JudgeDurationSeconds })
	return stats
}

func buildTotals(scenarios []model.ScenarioResult, in *model.ReportInput) model.Totals {
	totals := model.Totals{N: in.Manifest.Config.Runs}
	var withValues, withoutValues, deltas []float64
	var functionalWith []float64
	var withMins, withMaxs, withoutMins, withoutMaxs []float64
	for _, sc := range scenarios {
		totals.PendingPoints += sc.PendingPoints
		totals.PendingCriteria += sc.PendingCriteria
		if sc.Functional {
			totals.Functional++
			if sc.Arms.With.PercentMean != nil {
				functionalWith = append(functionalWith, *sc.Arms.With.PercentMean)
			}
			continue
		}
		if sc.Arms.With.PercentMean != nil {
			withValues = append(withValues, *sc.Arms.With.PercentMean)
		}
		if sc.Arms.Without.PercentMean != nil {
			withoutValues = append(withoutValues, *sc.Arms.Without.PercentMean)
		}
		if sc.Delta != nil {
			deltas = append(deltas, *sc.Delta)
		}
		if sc.Arms.With.PercentMin != nil {
			withMins = append(withMins, *sc.Arms.With.PercentMin)
		}
		if sc.Arms.With.PercentMax != nil {
			withMaxs = append(withMaxs, *sc.Arms.With.PercentMax)
		}
		if sc.Arms.Without.PercentMin != nil {
			withoutMins = append(withoutMins, *sc.Arms.Without.PercentMin)
		}
		if sc.Arms.Without.PercentMax != nil {
			withoutMaxs = append(withoutMaxs, *sc.Arms.Without.PercentMax)
		}
	}
	totals.With = mean(withValues)
	totals.Without = mean(withoutValues)
	totals.Delta = mean(deltas)
	totals.FunctionalWith = mean(functionalWith)
	totals.WithMin = minValue(withMins)
	totals.WithMax = maxValue(withMaxs)
	totals.WithoutMin = minValue(withoutMins)
	totals.WithoutMax = maxValue(withoutMaxs)

	var scoredCells, expectedCells, failedRuns, failedScenarios, notComparable int
	for _, sc := range scenarios {
		if sc.Status == "runner-failed" {
			failedScenarios++
		}
		if sc.Status == "not-comparable" {
			notComparable++
		}
		expectedCells += sc.CriteriaCount * sc.ConfiguredRuns * sc.ArmCount
		for _, run := range sc.Runs {
			if run.Status != string(model.StatusOK) {
				failedRuns++
			}
			scoredCells += len(run.Scored)
		}
	}
	totals.ScoredCriteriaCells = scoredCells
	totals.ExpectedCriteriaCells = expectedCells
	if expectedCells > 0 {
		totals.CoveragePercent = float64(scoredCells) * 100 / float64(expectedCells)
	}
	totals.FailedRuns = failedRuns
	totals.FailedScenarios = failedScenarios
	totals.NotComparable = notComparable
	for _, row := range in.Results {
		if row.Status != "" && row.Status != string(model.StatusOK) {
			totals.ResultsFailures++
		}
	}
	totals.RunnerFailures = maxInt(in.State.Counts["runner_failures"], failedRuns)
	totals.RunnerFailures = maxInt(totals.RunnerFailures, totals.ResultsFailures)
	totals.Failures = totals.RunnerFailures

	for _, sc := range scenarios {
		for _, criterion := range sc.Criteria {
			if criterion.Delta == nil {
				continue
			}
			switch {
			case *criterion.Delta > Epsilon:
				totals.Won++
			case *criterion.Delta < -Epsilon:
				totals.Lost++
			default:
				totals.Tied++
			}
		}
		for range sc.Regressions {
			totals.Regressions++
		}
	}
	return totals
}

func buildTiming(scenarios []model.ScenarioResult, in *model.ReportInput) model.Timing {
	timing := model.Timing{WallSeconds: in.State.DurationSeconds}
	if timing.WallSeconds != nil {
		timing.TotalSeconds = *timing.WallSeconds
	}
	var agentValues, judgeValues []*float64
	for _, sc := range scenarios {
		for _, run := range sc.Runs {
			timing.Runs++
			if run.TimedOut {
				timing.TimedOutRuns++
			}
			if run.DurationSeconds != nil {
				agentValues = append(agentValues, run.DurationSeconds)
			}
			if run.JudgeDurationSeconds != nil {
				judgeValues = append(judgeValues, run.JudgeDurationSeconds)
			}
		}
	}
	timing.AgentSeconds = sumPointers(agentValues)
	timing.JudgeSeconds = sumPointers(judgeValues)
	return timing
}

func buildVerdict(totals model.Totals) model.Verdict {
	if totals.Delta == nil {
		switch {
		case totals.Functional > 0 && totals.RunnerFailures == 0 && totals.FailedRuns == 0 &&
			totals.PendingPoints == 0 && totals.NotComparable == 0:
			return model.Verdict{
				Status: "functional",
				Reason: "functional scenarios report absolute scores; there is no with/without delta",
			}
		case totals.Functional > 0:
			return model.Verdict{
				Status: "incomplete",
				Reason: "functional scenarios still have failures or unjudged criteria",
			}
		default:
			return model.Verdict{
				Status: "incomplete",
				Reason: "no scenario has a paired with/without result; both arms must run",
			}
		}
	}
	if totals.RunnerFailures > 0 || totals.FailedRuns > 0 || totals.PendingPoints > 0 || totals.NotComparable > 0 {
		return model.Verdict{
			Status: "incomplete",
			Reason: "runner failures or unjudged criteria remain; a verdict would be made up",
		}
	}
	if totals.N == 1 {
		return model.Verdict{
			Status: "indicative",
			Reason: "one run per arm; variance cannot be estimated",
		}
	}
	delta := *totals.Delta
	if math.Abs(delta) <= Epsilon {
		return model.Verdict{
			Status: "no measurable effect",
			Reason: "delta is approximately zero; likely a task leak or a non-discriminating scenario",
		}
	}
	if rangesOverlap(totals) {
		return model.Verdict{
			Status: "inconclusive",
			Reason: "delta falls inside the overlapping per-arm min/max ranges",
		}
	}
	if delta > 0 && totals.Regressions == 0 {
		return model.Verdict{
			Status: "helps",
			Reason: "positive mean delta with no regressions",
		}
	}
	if delta < 0 {
		return model.Verdict{
			Status: "hurts",
			Reason: "negative mean delta",
		}
	}
	return model.Verdict{
		Status: "inconclusive",
		Reason: "positive mean but at least one criterion regressed",
	}
}

func rangesOverlap(totals model.Totals) bool {
	if totals.WithMin == nil || totals.WithMax == nil || totals.WithoutMin == nil || totals.WithoutMax == nil {
		return false
	}
	return *totals.WithMin <= *totals.WithoutMax && *totals.WithoutMin <= *totals.WithMax
}

func flattenCriteria(scenarios []model.ScenarioResult) []model.CriterionResult {
	var out []model.CriterionResult
	for _, sc := range scenarios {
		for _, criterion := range sc.Criteria {
			criterion.Plugin = sc.Plugin
			criterion.Scenario = sc.Scenario
			out = append(out, criterion)
		}
	}
	return out
}

func denominator(rubric []model.Criterion) float64 {
	total := 0.0
	for _, item := range rubric {
		total += item.MaxScore
	}
	return total
}

func rubricNames(rubric []model.Criterion) map[string]bool {
	names := make(map[string]bool, len(rubric))
	for _, item := range rubric {
		names[item.Name] = true
	}
	return names
}

func filterRuns(runs []model.RunRow, arm string) []model.RunRow {
	var out []model.RunRow
	for _, run := range runs {
		if run.Arm == arm {
			out = append(out, run)
		}
	}
	return out
}

func runByNumber(runs []model.RunRow, number int) *model.RunRow {
	for i := range runs {
		if runs[i].Run == number {
			return &runs[i]
		}
	}
	return nil
}

func anyRunFailed(runs []model.RunRow) bool {
	for _, run := range runs {
		if run.Status != string(model.StatusOK) {
			return true
		}
	}
	return false
}

func mean(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	out := total / float64(len(values))
	return &out
}

func minValue(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	out := values[0]
	for _, value := range values[1:] {
		if value < out {
			out = value
		}
	}
	return &out
}

func maxValue(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	out := values[0]
	for _, value := range values[1:] {
		if value > out {
			out = value
		}
	}
	return &out
}

func sumPresent(runs []model.RunRow, get func(model.RunRow) *float64) *float64 {
	var values []*float64
	for _, run := range runs {
		if value := get(run); value != nil {
			values = append(values, value)
		}
	}
	return sumPointers(values)
}

func sumPointers(values []*float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	total := 0.0
	for _, value := range values {
		total += *value
	}
	return &total
}

func diff(a, b *float64) *float64 {
	if a == nil || b == nil {
		return nil
	}
	out := *a - *b
	return &out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func join(values []string, sep string) string {
	out := ""
	for i, value := range values {
		if i > 0 {
			out += sep
		}
		out += value
	}
	return out
}

func nonNilItems(items []model.JudgedItem) []model.JudgedItem {
	if items == nil {
		return []model.JudgedItem{}
	}
	return items
}
