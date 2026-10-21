package report

import (
	"fmt"
	"strings"

	"github.com/rikdc/ai-skills/internal/model"
)

// RenderMarkdown renders a PR-comment-friendly report from the analysis model.
func RenderMarkdown(report model.Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Eval report: %s\n\n", mdEscape(report.Metadata.Label))
	fmt.Fprintf(&b, "- Suite: `%s`\n", mdEscape(report.Metadata.SuiteDir))
	fmt.Fprintf(&b, "- Agent: `%s`\n", mdEscape(report.Metadata.AgentCmd))
	if report.Metadata.Model != "" {
		fmt.Fprintf(&b, "- Model: `%s`\n", mdEscape(report.Metadata.Model))
	}
	dirty := ""
	if report.Metadata.GitDirty {
		dirty = " (dirty)"
	}
	fmt.Fprintf(&b, "- Git: `%s`%s  ·  Created: %s  ·  n: %d\n",
		mdEscape(report.Metadata.GitSHA), dirty, mdEscape(report.Metadata.CreatedAt), report.Metadata.N)
	if report.Metadata.AgentInvocations > 0 {
		fmt.Fprintf(&b, "- Agent invocations: %d\n", report.Metadata.AgentInvocations)
	}
	b.WriteString("- Fixed denominator: the rubric sum; unjudged criteria remain pending and are never renormalised.\n")
	fmt.Fprintf(&b, "\n## Verdict: %s\n\n%s\n\n", mdEscape(report.Verdict.Status), mdEscape(report.Verdict.Reason))
	b.WriteString(renderCaveatsMarkdown(report))
	b.WriteString("| With | Without | Delta | Won | Lost | Tied | Pending | Coverage | Failures |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	fmt.Fprintf(&b, "| %s | %s | %s | %d | %d | %d | %s | %s%% | %d |\n",
		fmtScore(report.Totals.With), fmtScore(report.Totals.Without), fmtDelta(report.Totals.Delta),
		report.Totals.Won, report.Totals.Lost, report.Totals.Tied,
		trimFloat(report.Totals.PendingPoints),
		strconv1(report.Totals.CoveragePercent), report.Totals.RunnerFailures)

	b.WriteString("\n## Regressions\n\n")
	regs := regressions(report.Criteria)
	if len(regs) == 0 {
		b.WriteString("None.\n")
	} else {
		b.WriteString("| Scenario | Criterion | With | Without | Delta |\n")
		b.WriteString("| --- | --- | --- | --- | --- |\n")
		for _, criterion := range regs {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
				mdEscape(criterionLabel(criterion)), mdEscape(criterion.Name),
				fmtScore(criterion.With), fmtScore(criterion.Without), fmtDelta(criterion.Delta))
		}
	}

	b.WriteString("\n## Scenarios\n\n")
	b.WriteString("| Scenario | With | Without | Delta | Pending | Status |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, sc := range sortScenarios(report.Scenarios) {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n",
			mdEscape(scenarioLabel(sc)), fmtScore(sc.Arms.With.PercentMean),
			fmtScore(sc.Arms.Without.PercentMean), fmtDelta(sc.Delta),
			trimFloat(sc.PendingPoints)+" pts", mdEscape(sc.Status))
	}

	b.WriteString("\n## Criteria\n\n")
	b.WriteString("| Scenario | Criterion | With | Without | Delta | Status |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, criterion := range sortCriteria(report.Criteria) {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s %s | %s |\n",
			mdEscape(criterionLabel(criterion)), mdEscape(criterion.Name),
			fmtScore(criterion.With), fmtScore(criterion.Without),
			deltaArrow(criterion.Delta), fmtDelta(criterion.Delta), mdEscape(criterion.Status))
	}

	b.WriteString("\n## Cost\n\n")
	if !hasKnownCost(report) {
		fmt.Fprintf(&b, "Cost: not reported by the agent command (0/%d runs); add `--format json`, `--output-format json`, or `--usage-cmd`.\n",
			totalRuns(report))
	} else {
		b.WriteString("| Cost | With | Without | Delta |\n| --- | --- | --- | --- |\n")
		for _, row := range costTableRows(report) {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", row.label, row.with, row.without, row.delta)
		}
		agentKnown := report.Cost.With.Agent.Cost.RunsKnown + report.Cost.Without.Agent.Cost.RunsKnown
		judgeKnown := report.Cost.With.Judge.Cost.RunsKnown + report.Cost.Without.Judge.Cost.RunsKnown
		combinedKnown := report.Cost.With.Combined.Cost.RunsKnown + report.Cost.Without.Combined.Cost.RunsKnown
		total := report.Cost.With.RunsTotal + report.Cost.Without.RunsTotal
		partial := ""
		if combinedKnown < total {
			partial = " (partial)"
		}
		fmt.Fprintf(&b, "\nCoverage: agent %d/%d, judge %d/%d, total %d/%d%s. Unknown usage is never counted as zero.\n",
			agentKnown, total, judgeKnown, total, combinedKnown, total, partial)
		if report.Cost.CostPerDeltaPoint != nil {
			fmt.Fprintf(&b, "Efficiency: %s pts for %s per run (%s per point).\n",
				fmtDelta(report.Totals.Delta), fmtMoneyDelta(deltaCost(report)),
				fmtCostPerPoint(report.Cost.CostPerDeltaPoint))
		} else {
			b.WriteString("Efficiency: — (partial cost coverage or no positive score delta).\n")
		}
		if report.Totals.Functional > 0 {
			b.WriteString("\nCost totals include functional runs; delta and efficiency use comparison scenarios only.\n")
		}
	}
	b.WriteString(renderDetailsMarkdown(report))
	b.WriteString(renderExpenditureMarkdown(report))
	b.WriteString(renderComparisonMarkdown(report))

	b.WriteString("\n## Timing\n\n")
	fmt.Fprintf(&b, "- Wall clock: %s\n", fmtSeconds(report.Timing.WallSeconds))
	fmt.Fprintf(&b, "- Agent: %s\n", fmtSeconds(report.Timing.AgentSeconds))
	fmt.Fprintf(&b, "- Judge: %s\n", fmtSeconds(report.Timing.JudgeSeconds))
	fmt.Fprintf(&b, "- Runs: %d, timed out: %d\n", report.Timing.Runs, report.Timing.TimedOutRuns)
	b.WriteString("\nRaw JSON: `report.json`  ·  HTML: `report.html`\n")
	return b.String()
}

type costRow struct {
	label   string
	with    string
	without string
	delta   string
}

func costTableRows(report model.Report) []costRow {
	d := deltaTotals(report)
	return []costRow{
		{"agent", fmtMoney(report.Cost.With.Agent.Cost.Total), fmtMoney(report.Cost.Without.Agent.Cost.Total), fmtMoneyDelta(d.AgentCost)},
		{"judge", fmtMoney(report.Cost.With.Judge.Cost.Total), fmtMoney(report.Cost.Without.Judge.Cost.Total), fmtMoneyDelta(d.JudgeCost)},
		{"total", fmtMoney(report.Cost.With.Combined.Cost.Total), fmtMoney(report.Cost.Without.Combined.Cost.Total), fmtMoneyDelta(d.CombinedCost)},
		{"input tokens", fmtTokens(report.Cost.With.Combined.Tokens.Input.Total), fmtTokens(report.Cost.Without.Combined.Tokens.Input.Total), fmtTokensDelta(d.CombinedInputTokens)},
		{"output tokens", fmtTokens(report.Cost.With.Combined.Tokens.Output.Total), fmtTokens(report.Cost.Without.Combined.Tokens.Output.Total), fmtTokensDelta(d.CombinedOutputTokens)},
		{"reasoning tokens", fmtTokens(report.Cost.With.Combined.Tokens.Reasoning.Total), fmtTokens(report.Cost.Without.Combined.Tokens.Reasoning.Total), fmtTokensDelta(d.CombinedReasoningTokens)},
		{"cache read", fmtTokens(report.Cost.With.Combined.Tokens.CacheRead.Total), fmtTokens(report.Cost.Without.Combined.Tokens.CacheRead.Total), fmtTokensDelta(d.CombinedCacheReadTokens)},
		{"total tokens", fmtTokens(report.Cost.With.Combined.Tokens.Total), fmtTokens(report.Cost.Without.Combined.Tokens.Total), fmtTokensDelta(d.CombinedTokens)},
	}
}

func renderExpenditureMarkdown(report model.Report) string {
	e := report.Expenditure
	if e.TotalAttempts == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Expenditure\n\n")
	b.WriteString("| Attempts | Agent | Hook | Judge | Known | Unknown | Not applicable | Known total |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- |\n")
	fmt.Fprintf(&b, "| %d | %d | %d | %d | %d | %d | %d | %s |\n",
		e.TotalAttempts, e.AgentAttempts, e.HookAttempts, e.JudgeAttempts,
		e.KnownAttempts, e.UnknownAttempts, e.NotApplicableAttempts, fmtMoney(e.KnownTotal.CostUSD))
	if e.Complete {
		b.WriteString("\nEvery applicable attempt has known usage. Expenditure includes superseded, failed and regraded attempts.\n")
	} else {
		b.WriteString("\nThe known total is a lower bound: at least one applicable attempt has unknown usage. Unknown is never counted as zero.\n")
	}
	return b.String()
}

func renderComparisonMarkdown(report model.Report) string {
	comparison := report.Comparison
	if comparison == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Change since baseline\n\n")
	fmt.Fprintf(&b, "- Baseline: `%s`\n", mdEscape(sideDir(comparison.Baseline)))
	fmt.Fprintf(&b, "- Current: `%s`\n", mdEscape(sideDir(comparison.Current)))
	fmt.Fprintf(&b, "- Comparable scenarios: %d of %d current / %d baseline\n",
		comparison.ComparableScenarios, comparison.CurrentScenarios, comparison.BaselineScenarios)
	fmt.Fprintf(&b, "- Status: **%s**\n", mdEscape(comparison.Status))
	if !comparison.Comparable {
		b.WriteString("\nNo scenario with the same rubric ran in both suites; deltas are withheld.\n")
	}
	if len(comparison.Excluded) > 0 {
		b.WriteString("\nExcluded:\n\n")
		for _, excluded := range comparison.Excluded {
			fmt.Fprintf(&b, "- `%s/%s` (%s) — %s\n",
				mdEscape(excluded.Plugin), mdEscape(excluded.Scenario),
				mdEscape(excluded.Side), mdEscape(excluded.Reason))
		}
	}
	if !comparison.Comparable {
		return b.String()
	}
	b.WriteString("\n| Metric | Baseline | Current | Delta |\n| --- | --- | --- | --- |\n")
	for _, row := range comparisonRows(comparison) {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", row.label, row.baseline, row.current, row.delta)
	}
	return b.String()
}

func mdEscape(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func strconv1(value float64) string {
	return fmt.Sprintf("%.1f", value)
}
