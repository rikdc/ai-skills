package report

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rikdc/ai-skills/internal/model"
)

// RenderCLI renders the report for a terminal. Color is applied only when the
// caller has already decided stdout is a TTY and color is enabled.
func RenderCLI(report model.Report, color bool) string {
	var b strings.Builder
	bold := func(code, text string) string {
		if !color {
			return text
		}
		return "\x1b[" + code + "m" + text + "\x1b[0m"
	}

	fmt.Fprintf(&b, "%s\n", bold("1", "Eval report: "+report.Metadata.Label))
	fmt.Fprintf(&b, "Suite: %s\n", report.Metadata.SuiteDir)
	fmt.Fprintf(&b, "Agent: %s\n", report.Metadata.AgentCmd)
	if report.Metadata.Model != "" {
		fmt.Fprintf(&b, "Model: %s\n", report.Metadata.Model)
	}
	dirty := ""
	if report.Metadata.GitDirty {
		dirty = " (dirty)"
	}
	fmt.Fprintf(&b, "Git: %s%s   Created: %s   n: %d   Arm: %s\n",
		report.Metadata.GitSHA, dirty, report.Metadata.CreatedAt,
		report.Metadata.N, report.Metadata.Arm)
	if len(report.Metadata.Counts) > 0 {
		if invocations, ok := report.Metadata.Counts["agent_invocations"]; ok {
			fmt.Fprintf(&b, "Invocations: %d\n", invocations)
		}
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "%s\n", bold("1;33", "Verdict: "+report.Verdict.Status))
	fmt.Fprintf(&b, "  %s\n\n", report.Verdict.Reason)
	b.WriteString(renderCaveatsCLI(report, bold))
	fmt.Fprintf(&b, "With: %s/100    Without: %s/100    Delta: %s\n",
		fmtScore(report.Totals.With), fmtScore(report.Totals.Without), fmtDelta(report.Totals.Delta))
	fmt.Fprintf(&b, "Criteria: %d won, %d lost, %d tied    Coverage: %s%%    Pending: %s points (both arms)    Failures: %d\n",
		report.Totals.Won, report.Totals.Lost, report.Totals.Tied,
		strconv.FormatFloat(report.Totals.CoveragePercent, 'f', 1, 64),
		strconv.FormatFloat(report.Totals.PendingPoints, 'f', -1, 64),
		report.Totals.RunnerFailures)

	if !hasKnownCost(report) {
		fmt.Fprintf(&b, "Cost: not reported by the agent command (0/%d runs); add --format json, --output-format json, or --usage-cmd.\n",
			totalRuns(report))
	} else {
		b.WriteString("\n")
		fmt.Fprintf(&b, "%-18s %12s %12s %12s\n", "COST", "WITH", "WITHOUT", "DELTA")
		costRows := []struct {
			label string
			with  *float64
			wo    *float64
			delta *float64
			money bool
		}{
			{"agent", report.Cost.With.Agent.Cost.Total, report.Cost.Without.Agent.Cost.Total, deltaTotals(report).AgentCost, true},
			{"judge", report.Cost.With.Judge.Cost.Total, report.Cost.Without.Judge.Cost.Total, deltaTotals(report).JudgeCost, true},
			{"total", report.Cost.With.Combined.Cost.Total, report.Cost.Without.Combined.Cost.Total, deltaTotals(report).CombinedCost, true},
			{"input tokens", report.Cost.With.Combined.Tokens.Input.Total, report.Cost.Without.Combined.Tokens.Input.Total, deltaTotals(report).CombinedInputTokens, false},
			{"output tokens", report.Cost.With.Combined.Tokens.Output.Total, report.Cost.Without.Combined.Tokens.Output.Total, deltaTotals(report).CombinedOutputTokens, false},
			{"reasoning tokens", report.Cost.With.Combined.Tokens.Reasoning.Total, report.Cost.Without.Combined.Tokens.Reasoning.Total, deltaTotals(report).CombinedReasoningTokens, false},
			{"cache read", report.Cost.With.Combined.Tokens.CacheRead.Total, report.Cost.Without.Combined.Tokens.CacheRead.Total, deltaTotals(report).CombinedCacheReadTokens, false},
			{"total tokens", report.Cost.With.Combined.Tokens.Total, report.Cost.Without.Combined.Tokens.Total, deltaTotals(report).CombinedTokens, false},
		}
		for _, row := range costRows {
			if row.money {
				fmt.Fprintf(&b, "%-18s %12s %12s %12s\n", row.label,
					fmtMoney(row.with), fmtMoney(row.wo), fmtMoneyDelta(row.delta))
			} else {
				fmt.Fprintf(&b, "%-18s %12s %12s %12s\n", row.label,
					fmtTokens(row.with), fmtTokens(row.wo), fmtTokensDelta(row.delta))
			}
		}
		if report.Totals.Functional > 0 {
			b.WriteString("\nCost totals include functional runs; delta and efficiency use comparison scenarios only.\n")
		}
		agentKnown := report.Cost.With.Agent.Cost.RunsKnown + report.Cost.Without.Agent.Cost.RunsKnown
		judgeKnown := report.Cost.With.Judge.Cost.RunsKnown + report.Cost.Without.Judge.Cost.RunsKnown
		combinedKnown := report.Cost.With.Combined.Cost.RunsKnown + report.Cost.Without.Combined.Cost.RunsKnown
		// Each denominator counts the runs where that usage applies; a failed
		// run, or one that needed no judge, has none.
		agentTotal := report.Cost.With.Agent.Cost.RunsTotal + report.Cost.Without.Agent.Cost.RunsTotal
		judgeTotal := report.Cost.With.Judge.Cost.RunsTotal + report.Cost.Without.Judge.Cost.RunsTotal
		combinedTotal := report.Cost.With.Combined.Cost.RunsTotal + report.Cost.Without.Combined.Cost.RunsTotal
		partial := ""
		if combinedKnown < combinedTotal {
			partial = " (partial)"
		}
		fmt.Fprintf(&b, "  coverage: agent %d/%d, judge %d/%d, total %d/%d%s\n",
			agentKnown, agentTotal, judgeKnown, judgeTotal, combinedKnown, combinedTotal, partial)
		if report.Cost.CostPerDeltaPoint != nil {
			fmt.Fprintf(&b, "Efficiency: %s pts for %s per run  (%s per point)\n",
				fmtDelta(report.Totals.Delta), fmtMoneyDelta(deltaCost(report)),
				fmtCostPerPoint(report.Cost.CostPerDeltaPoint))
		} else {
			b.WriteString("Efficiency: — (cost coverage or score delta does not support a per-point claim)\n")
		}
	}

	if report.Expenditure.TotalAttempts > 0 {
		e := report.Expenditure
		fmt.Fprintf(&b, "\n%s\n", bold("1", "Expenditure (all immutable attempts)"))
		fmt.Fprintf(&b, "  attempts %d · agent %d · hook %d · judge %d\n",
			e.TotalAttempts, e.AgentAttempts, e.HookAttempts, e.JudgeAttempts)
		fmt.Fprintf(&b, "  known %d · unknown %d · not applicable %d · known total %s",
			e.KnownAttempts, e.UnknownAttempts, e.NotApplicableAttempts, fmtMoney(e.KnownTotal.CostUSD))
		if e.Complete {
			b.WriteString(" (complete)\n")
		} else {
			b.WriteString(" (lower bound; missing usage is never zero)\n")
		}
		if report.Totals.Functional > 0 {
			b.WriteString("  Includes functional runs and superseded/regraded attempts.\n")
		}
	}

	b.WriteString(renderDetailsCLI(report, bold))
	b.WriteString(renderComparisonCLI(report))

	fmt.Fprintf(&b, "\n%s\n", bold("1", "Timing"))
	fmt.Fprintf(&b, "  wall %s · agent %s · judge %s · %d run(s), %d timed out\n",
		fmtSeconds(report.Timing.WallSeconds), fmtSeconds(report.Timing.AgentSeconds),
		fmtSeconds(report.Timing.JudgeSeconds), report.Timing.Runs, report.Timing.TimedOutRuns)

	fmt.Fprintf(&b, "\n%s\n", bold("1", "Regressions"))
	regs := regressions(report.Criteria)
	if len(regs) == 0 {
		b.WriteString("  none\n")
	} else {
		for _, criterion := range regs {
			fmt.Fprintf(&b, "  %-36s %-42s %s -> %s (%s)\n",
				criterionLabel(criterion), criterion.Name,
				fmtScore(criterion.With), fmtScore(criterion.Without), fmtDelta(criterion.Delta))
		}
	}

	fmt.Fprintf(&b, "\n%s\n", bold("1", "Scenarios (delta descending)"))
	fmt.Fprintf(&b, "  %-44s %8s %8s %8s %10s  %s\n", "SCENARIO", "WITH", "WITHOUT", "DELTA", "PENDING", "STATUS")
	for _, sc := range sortScenarios(report.Scenarios) {
		fmt.Fprintf(&b, "  %-44s %8s %8s %8s %10s  %s\n",
			scenarioLabel(sc), fmtScore(sc.Arms.With.PercentMean), fmtScore(sc.Arms.Without.PercentMean),
			fmtDelta(sc.Delta), pendingPoints(sc.PendingPoints), sc.Status)
	}
	fmt.Fprintf(&b, "  arm range: with %s–%s, without %s–%s (min–max over runs)\n",
		fmtScore(report.Totals.WithMin), fmtScore(report.Totals.WithMax),
		fmtScore(report.Totals.WithoutMin), fmtScore(report.Totals.WithoutMax))

	fmt.Fprintf(&b, "\n%s\n", bold("1", "Criteria (regressions first)"))
	fmt.Fprintf(&b, "  %-36s %-34s %7s %7s %8s  %s\n", "SCENARIO", "CRITERION", "WITH", "WITHOUT", "DELTA", "STATUS")
	for _, criterion := range sortCriteria(report.Criteria) {
		fmt.Fprintf(&b, "  %-36s %-34s %7s %7s %8s  %s\n",
			criterionLabel(criterion), criterion.Name, fmtScore(criterion.With),
			fmtScore(criterion.Without), deltaArrow(criterion.Delta)+" "+fmtDelta(criterion.Delta),
			criterion.Status)
	}

	if report.Totals.FailedRuns != 0 || report.Totals.PendingPoints != 0 {
		fmt.Fprintf(&b, "\n%s\n", bold("1;31", "Incomplete runs"))
		for _, sc := range report.Scenarios {
			if sc.Status == "ok" && sc.PendingPoints == 0 {
				continue
			}
			fmt.Fprintf(&b, "  %s — %s pending point(s), status %s\n",
				scenarioLabel(sc), trimFloat(sc.PendingPoints), sc.Status)
		}
	}
	return b.String()
}

func renderComparisonCLI(report model.Report) string {
	comparison := report.Comparison
	if comparison == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nChange since baseline\n")
	fmt.Fprintf(&b, "  Baseline: %s\n", sideDir(comparison.Baseline))
	fmt.Fprintf(&b, "  Comparable scenarios: %d of %d current / %d baseline\n",
		comparison.ComparableScenarios, comparison.CurrentScenarios, comparison.BaselineScenarios)
	fmt.Fprintf(&b, "  Status: %s\n", comparison.Status)
	if !comparison.Comparable {
		b.WriteString("  No scenario with the same rubric ran in both suites; deltas are withheld.\n")
	}
	if len(comparison.Excluded) > 0 {
		b.WriteString("  Excluded:\n")
		for _, excluded := range comparison.Excluded {
			fmt.Fprintf(&b, "    - %s: %s/%s — %s\n",
				excluded.Side, excluded.Plugin, excluded.Scenario, excluded.Reason)
		}
	}
	if !comparison.Comparable {
		return b.String()
	}
	fmt.Fprintf(&b, "\n%-22s %12s %12s %12s\n", "METRIC", "BASELINE", "CURRENT", "DELTA")
	for _, row := range comparisonRows(comparison) {
		fmt.Fprintf(&b, "%-22s %12s %12s %12s\n", row.label, row.baseline, row.current, row.delta)
	}
	return b.String()
}

type comparisonRow struct {
	label    string
	baseline string
	current  string
	delta    string
}

func comparisonRows(c *model.Comparison) []comparisonRow {
	base, cur, d := c.Baseline, c.Current, c.Delta
	return []comparisonRow{
		{"Score with", fmtScore(base.With), fmtScore(cur.With), fmtDelta(d.WithScore)},
		{"Score without", fmtScore(base.Without), fmtScore(cur.Without), fmtDelta(d.WithoutScore)},
		{"Score effect", fmtScore(base.Delta), fmtScore(cur.Delta), fmtDelta(d.Effect)},
		{"Cost with / run", fmtMoney(base.Cost.WithPerRun), fmtMoney(cur.Cost.WithPerRun), fmtMoneyDelta(d.WithCostPerRun)},
		{"Cost without / run", fmtMoney(base.Cost.WithoutPerRun), fmtMoney(cur.Cost.WithoutPerRun), fmtMoneyDelta(d.WithoutCostPerRun)},
		{"Cost pair / run", fmtMoney(base.Cost.PairPerRun), fmtMoney(cur.Cost.PairPerRun), fmtMoneyDelta(d.PairCostPerRun)},
		{"Agent cost / run", fmtMoney(base.Cost.AgentPerRun), fmtMoney(cur.Cost.AgentPerRun), fmtMoneyDelta(d.AgentCostPerRun)},
		{"Judge cost / run", fmtMoney(base.Cost.JudgePerRun), fmtMoney(cur.Cost.JudgePerRun), fmtMoneyDelta(d.JudgeCostPerRun)},
		{"Input with / run", fmtTokens(base.Tokens.WithInputPerRun), fmtTokens(cur.Tokens.WithInputPerRun), fmtTokensDelta(d.WithInputTokensPerRun)},
		{"Output with / run", fmtTokens(base.Tokens.WithOutputPerRun), fmtTokens(cur.Tokens.WithOutputPerRun), fmtTokensDelta(d.WithOutputTokensPerRun)},
		{"Input without / run", fmtTokens(base.Tokens.WithoutInputPerRun), fmtTokens(cur.Tokens.WithoutInputPerRun), fmtTokensDelta(d.WithoutInputTokensPerRun)},
		{"Output without / run", fmtTokens(base.Tokens.WithoutOutputPerRun), fmtTokens(cur.Tokens.WithoutOutputPerRun), fmtTokensDelta(d.WithoutOutputTokensPerRun)},
		{"Total with / run", fmtTokens(base.Tokens.WithTotalPerRun), fmtTokens(cur.Tokens.WithTotalPerRun), fmtTokensDelta(d.WithTotalTokensPerRun)},
		{"Total without / run", fmtTokens(base.Tokens.WithoutTotalPerRun), fmtTokens(cur.Tokens.WithoutTotalPerRun), fmtTokensDelta(d.WithoutTotalTokensPerRun)},
		{"Wall time", fmtSeconds(base.Timing.WallSeconds), fmtSeconds(cur.Timing.WallSeconds), fmtSecondsDelta(d.WallSeconds)},
		{"Agent time", fmtSeconds(base.Timing.AgentSeconds), fmtSeconds(cur.Timing.AgentSeconds), fmtSecondsDelta(d.AgentSeconds)},
		{"Judge time", fmtSeconds(base.Timing.JudgeSeconds), fmtSeconds(cur.Timing.JudgeSeconds), fmtSecondsDelta(d.JudgeSeconds)},
	}
}

func deltaTotals(report model.Report) model.CostDeltaTotals {
	if report.Cost.DeltaTotals == nil {
		return model.CostDeltaTotals{}
	}
	return *report.Cost.DeltaTotals
}

func deltaCost(report model.Report) *float64 {
	if report.Cost.Delta == nil {
		return nil
	}
	return report.Cost.Delta.CombinedCost
}

func sideDir(side *model.ComparisonSide) string {
	if side == nil {
		return ""
	}
	return side.SuiteDir
}

func pendingPoints(value float64) string {
	return trimFloat(value) + " pts"
}

func trimFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
