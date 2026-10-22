package report

import (
	"fmt"
	"html"
	"strings"

	"github.com/rikdc/ai-skills/internal/model"
)

const reportCSS = `
:root{--bg:#fff;--fg:#1a1a1a;--muted:#555;--line:#d8d8d8;--card:#f5f5f5;--good:#0a7a3d;--bad:#b3261e;--warn:#8a5a00}
@media (prefers-color-scheme:dark){:root{--bg:#111417;--fg:#e8e8e8;--muted:#a0a6ad;--line:#333a40;--card:#1a1f24;--good:#5cd68a;--bad:#ff8a80;--warn:#ffd166}}
*{box-sizing:border-box}
body{margin:0 auto;max-width:1100px;padding:2rem 1.25rem 4rem;background:var(--bg);color:var(--fg);font:15px/1.55 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
h1,h2{line-height:1.2}h2{margin-top:2rem;border-bottom:1px solid var(--line);padding-bottom:.35rem}
.meta{color:var(--muted)}.note{color:var(--muted);font-size:.9rem}
.verdict{border:1px solid var(--line);border-left:6px solid var(--warn);background:var(--card);padding:1rem 1.25rem;border-radius:8px}
.verdict.incomplete,.verdict.hurts{border-left-color:var(--bad)}
.verdict.helps,.verdict.functional{border-left-color:var(--good)}
.verdict .state{font-size:1.4rem;font-weight:700;text-transform:uppercase;letter-spacing:.04em}
.verdict .reason{color:var(--muted);margin-top:.35rem}
.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:.75rem;margin-top:1rem}
.card{background:var(--card);border:1px solid var(--line);border-radius:8px;padding:.75rem .9rem}
.card .k{color:var(--muted);font-size:.8rem;text-transform:uppercase;letter-spacing:.05em}
.card .v{font-size:1.35rem;font-weight:600}.pos{color:var(--good)}.neg{color:var(--bad)}
table{width:100%;border-collapse:collapse;margin-top:.75rem;font-size:.92rem}
th,td{text-align:left;padding:.45rem .55rem;border-bottom:1px solid var(--line);vertical-align:top}
th{color:var(--muted);font-size:.78rem;text-transform:uppercase;letter-spacing:.05em}
td.num{font-variant-numeric:tabular-nums;white-space:nowrap}
ul.links{columns:2}
`

// RenderHTML renders one self-contained HTML document: inline CSS, no
// JavaScript, no external assets, and escaped scenario/criterion text.
func RenderHTML(report model.Report) string {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("<title>Eval report</title>\n<style>")
	b.WriteString(reportCSS)
	b.WriteString("</style>\n</head>\n<body>\n")

	fmt.Fprintf(&b, "<h1>Eval report: %s</h1>\n", esc(report.Metadata.Label))
	fmt.Fprintf(&b, "<p class=\"meta\">Suite: <code>%s</code><br>Agent: <code>%s</code>",
		esc(report.Metadata.SuiteDir), esc(report.Metadata.AgentCmd))
	if report.Metadata.Model != "" {
		fmt.Fprintf(&b, " · Model: <code>%s</code>", esc(report.Metadata.Model))
	}
	dirty := ""
	if report.Metadata.GitDirty {
		dirty = " (dirty)"
	}
	fmt.Fprintf(&b, "<br>Git: <code>%s</code>%s · Created: %s · n: %d", esc(report.Metadata.GitSHA), dirty,
		esc(report.Metadata.CreatedAt), report.Metadata.N)
	if report.Metadata.AgentInvocations > 0 {
		fmt.Fprintf(&b, " · Agent invocations: %d", report.Metadata.AgentInvocations)
	}
	b.WriteString("</p>\n")
	b.WriteString("<p class=\"note\"><strong>Fixed denominator.</strong> Scores use the full rubric max_score sum; " +
		"a run with only part of its checklist judged is reported as <code>52/100 + 48 pending</code>, " +
		"never renormalised to <code>52/52</code>.</p>\n")
	fmt.Fprintf(&b, "<section class=\"verdict %s\"><div class=\"state\">%s</div><div class=\"reason\">%s</div></section>\n",
		esc(strings.ReplaceAll(report.Verdict.Status, " ", "-")), esc(report.Verdict.Status), esc(report.Verdict.Reason))

	b.WriteString("<div class=\"cards\">\n")
	card(&b, "With", fmtScore(report.Totals.With), "")
	card(&b, "Without", fmtScore(report.Totals.Without), "")
	cardClass(&b, "Delta", fmtDelta(report.Totals.Delta), classFor(report.Totals.Delta), "")
	card(&b, "Criteria", fmt.Sprintf("%d / %d / %d", report.Totals.Won, report.Totals.Lost, report.Totals.Tied), "won / lost / tied")
	card(&b, "Pending", trimFloat(report.Totals.PendingPoints), "points unjudged")
	card(&b, "Coverage", strconv1(report.Totals.CoveragePercent)+"%", "")
	card(&b, "Failures", fmt.Sprintf("%d", report.Totals.RunnerFailures), "")
	card(&b, "Cost with", fmtMoney(report.Cost.With.Combined.Cost.Total),
		fmtCoverage(report.Cost.With.Combined.Cost)+" runs")
	card(&b, "Cost without", fmtMoney(report.Cost.Without.Combined.Cost.Total),
		fmtCoverage(report.Cost.Without.Combined.Cost)+" runs")
	cardClass(&b, "Cost delta", fmtMoneyDelta(deltaTotals(report).CombinedCost),
		classFor(deltaTotals(report).CombinedCost), "")
	card(&b, "Tokens", fmtTokens(report.Cost.With.Combined.Tokens.Total)+" / "+
		fmtTokens(report.Cost.Without.Combined.Tokens.Total), "with / without")
	b.WriteString("</div>\n")

	b.WriteString("<h2>Criteria</h2>\n<table><thead><tr><th>Scenario</th><th>Criterion</th><th>With</th><th>Without</th><th>Delta</th><th>Status</th></tr></thead><tbody>\n")
	for _, criterion := range sortCriteria(report.Criteria) {
		fmt.Fprintf(&b, "<tr><td>%s/%s</td><td>%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td class=\"num %s\">%s %s</td><td>%s</td></tr>\n",
			esc(criterion.Plugin), esc(criterion.Scenario), esc(criterion.Name),
			fmtScore(criterion.With), fmtScore(criterion.Without), classFor(criterion.Delta),
			deltaArrow(criterion.Delta), fmtDelta(criterion.Delta), esc(criterion.Status))
	}
	b.WriteString("</tbody></table>\n")

	b.WriteString("<h2>Scenarios</h2>\n<table><thead><tr><th>Scenario</th><th>With</th><th>Without</th><th>Delta</th><th>Pending</th><th>Status</th></tr></thead><tbody>\n")
	for _, sc := range sortScenarios(report.Scenarios) {
		fmt.Fprintf(&b, "<tr><td>%s/%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td class=\"num %s\">%s %s</td><td class=\"num\">%s pts</td><td>%s</td></tr>\n",
			esc(sc.Plugin), esc(sc.Scenario), fmtScore(sc.Arms.With.PercentMean),
			fmtScore(sc.Arms.Without.PercentMean), classFor(sc.Delta), deltaArrow(sc.Delta),
			fmtDelta(sc.Delta), trimFloat(sc.PendingPoints), esc(sc.Status))
	}
	b.WriteString("</tbody></table>\n")

	b.WriteString("<h2>Regressions</h2>\n")
	regs := regressions(report.Criteria)
	if len(regs) == 0 {
		b.WriteString("<p class=\"note\">None.</p>\n")
	} else {
		b.WriteString("<table><thead><tr><th>Scenario</th><th>Criterion</th><th>With</th><th>Without</th><th>Delta</th></tr></thead><tbody>\n")
		for _, criterion := range regs {
			fmt.Fprintf(&b, "<tr><td>%s/%s</td><td>%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td class=\"num neg\">%s</td></tr>\n",
				esc(criterion.Plugin), esc(criterion.Scenario), esc(criterion.Name),
				fmtScore(criterion.With), fmtScore(criterion.Without), fmtDelta(criterion.Delta))
		}
		b.WriteString("</tbody></table>\n")
	}

	b.WriteString("<h2>Cost</h2>\n")
	if !hasKnownCost(report) {
		fmt.Fprintf(&b, "<p class=\"note\">Cost: not reported by the agent command (0/%d runs); add <code>--format json</code>, <code>--output-format json</code>, or <code>--usage-cmd</code>.</p>\n",
			totalRuns(report))
	} else {
		b.WriteString("<table><thead><tr><th>Cost</th><th>With</th><th>Without</th><th>Delta</th><th>Coverage</th></tr></thead><tbody>\n")
		fmt.Fprintf(&b, "<tr><td>agent</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td>%s / %s</td></tr>\n",
			fmtMoney(report.Cost.With.Agent.Cost.Total), fmtMoney(report.Cost.Without.Agent.Cost.Total),
			fmtMoneyDelta(deltaTotals(report).AgentCost),
			fmtCoverage(report.Cost.With.Agent.Cost), fmtCoverage(report.Cost.Without.Agent.Cost))
		fmt.Fprintf(&b, "<tr><td>judge</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td>%s / %s</td></tr>\n",
			fmtMoney(report.Cost.With.Judge.Cost.Total), fmtMoney(report.Cost.Without.Judge.Cost.Total),
			fmtMoneyDelta(deltaTotals(report).JudgeCost),
			fmtCoverage(report.Cost.With.Judge.Cost), fmtCoverage(report.Cost.Without.Judge.Cost))
		fmt.Fprintf(&b, "<tr><td>total</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td>%s / %s</td></tr>\n",
			fmtMoney(report.Cost.With.Combined.Cost.Total), fmtMoney(report.Cost.Without.Combined.Cost.Total),
			fmtMoneyDelta(deltaTotals(report).CombinedCost),
			fmtCoverage(report.Cost.With.Combined.Cost), fmtCoverage(report.Cost.Without.Combined.Cost))
		for _, row := range costTableRows(report) {
			switch row.label {
			case "input tokens", "output tokens", "reasoning tokens", "cache read", "total tokens":
				fmt.Fprintf(&b, "<tr><td>%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td></td></tr>\n",
					row.label, row.with, row.without, row.delta)
			}
		}
		b.WriteString("</tbody></table>\n")
		b.WriteString("<p class=\"note\">Unknown usage is never counted as zero. Coverage is known runs / total runs.</p>\n")
		if report.Totals.Functional > 0 {
			b.WriteString("<p class=\"note\">Cost totals include functional runs; delta and efficiency use comparison scenarios only.</p>\n")
		}
		if report.Cost.CostPerDeltaPoint != nil {
			fmt.Fprintf(&b, "<p class=\"note\">Efficiency: %s pts for %s per run (%s per point).</p>\n",
				fmtDelta(report.Totals.Delta), fmtMoneyDelta(deltaCost(report)),
				esc(fmtCostPerPoint(report.Cost.CostPerDeltaPoint)))
		} else {
			b.WriteString("<p class=\"note\">Efficiency: — (partial cost coverage or no positive score delta).</p>\n")
		}
	}
	b.WriteString(renderExpenditureHTML(report))
	b.WriteString(renderComparisonHTML(report))

	b.WriteString("<h2>Timing</h2>\n")
	fmt.Fprintf(&b, "<p class=\"note\">Wall %s · agent %s · judge %s · %d run(s), %d timed out.</p>\n",
		fmtSeconds(report.Timing.WallSeconds), fmtSeconds(report.Timing.AgentSeconds),
		fmtSeconds(report.Timing.JudgeSeconds), report.Timing.Runs, report.Timing.TimedOutRuns)

	b.WriteString("<h2>Evidence</h2>\n")
	b.WriteString("<p class=\"note\">Relative links; open from the suite directory. The raw JSON is beside this file.</p>\n")
	b.WriteString("<ul class=\"links\">\n")
	for _, sc := range report.Scenarios {
		fmt.Fprintf(&b, "<li><a href=\"../runs/%s/%s\">%s/%s</a></li>\n",
			urlEscape(sc.Plugin), urlEscape(sc.Scenario), esc(sc.Plugin), esc(sc.Scenario))
	}
	b.WriteString("<li><a href=\"report.json\">report.json</a></li>\n</ul>\n</body>\n</html>\n")
	return b.String()
}

func renderExpenditureHTML(report model.Report) string {
	e := report.Expenditure
	if e.TotalAttempts == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<h2>Expenditure</h2>\n")
	b.WriteString("<table><thead><tr><th>Attempts</th><th>Agent</th><th>Hook</th><th>Judge</th><th>Known</th><th>Unknown</th><th>Not applicable</th><th>Known total</th></tr></thead><tbody>\n")
	fmt.Fprintf(&b, "<tr><td class=\"num\">%d</td><td class=\"num\">%d</td><td class=\"num\">%d</td><td class=\"num\">%d</td><td class=\"num\">%d</td><td class=\"num\">%d</td><td class=\"num\">%d</td><td class=\"num\">%s</td></tr>\n",
		e.TotalAttempts, e.AgentAttempts, e.HookAttempts, e.JudgeAttempts,
		e.KnownAttempts, e.UnknownAttempts, e.NotApplicableAttempts, fmtMoney(e.KnownTotal.CostUSD))
	b.WriteString("</tbody></table>\n")
	if e.Complete {
		b.WriteString("<p class=\"note\">Every applicable attempt has known usage. Expenditure includes superseded, failed and regraded attempts.</p>\n")
	} else {
		b.WriteString("<p class=\"note\">The known total is a lower bound: at least one applicable attempt has unknown usage. Unknown is never counted as zero.</p>\n")
	}
	return b.String()
}

func renderComparisonHTML(report model.Report) string {
	comparison := report.Comparison
	if comparison == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("<h2>Change since baseline</h2>\n")
	fmt.Fprintf(&b, "<p class=\"note\">Baseline: <code>%s</code> · Current: <code>%s</code> · Comparable scenarios: %d of %d current / %d baseline · Status: <strong>%s</strong></p>\n",
		esc(sideDir(comparison.Baseline)), esc(sideDir(comparison.Current)),
		comparison.ComparableScenarios, comparison.CurrentScenarios, comparison.BaselineScenarios,
		esc(comparison.Status))
	if !comparison.Comparable {
		b.WriteString("<p class=\"note\">No scenario with the same rubric ran in both suites; deltas are withheld.</p>\n")
	}
	if len(comparison.Excluded) > 0 {
		b.WriteString("<p class=\"note\">Excluded:</p>\n<ul class=\"note\">\n")
		for _, excluded := range comparison.Excluded {
			fmt.Fprintf(&b, "<li>%s: %s/%s — %s</li>\n",
				esc(excluded.Side), esc(excluded.Plugin), esc(excluded.Scenario), esc(excluded.Reason))
		}
		b.WriteString("</ul>\n")
	}
	if !comparison.Comparable {
		return b.String()
	}
	b.WriteString("<table><thead><tr><th>Metric</th><th>Baseline</th><th>Current</th><th>Delta</th></tr></thead><tbody>\n")
	for _, row := range comparisonRows(comparison) {
		fmt.Fprintf(&b, "<tr><td>%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td></tr>\n",
			row.label, row.baseline, row.current, row.delta)
	}
	b.WriteString("</tbody></table>\n")
	return b.String()
}

func card(b *strings.Builder, key, value, note string) {
	cardClass(b, key, value, "", note)
}

func cardClass(b *strings.Builder, key, value, class, note string) {
	if class != "" {
		class = " " + class
	}
	fmt.Fprintf(b, "<div class=\"card\"><div class=\"k\">%s</div><div class=\"v%s\">%s</div>",
		esc(key), class, esc(value))
	if note != "" {
		fmt.Fprintf(b, "<div class=\"note\">%s</div>", esc(note))
	}
	b.WriteString("</div>\n")
}

func classFor(value *float64) string {
	if value == nil {
		return ""
	}
	if *value > Epsilon {
		return "pos"
	}
	if *value < -Epsilon {
		return "neg"
	}
	return ""
}

func esc(value string) string { return html.EscapeString(value) }

func urlEscape(value string) string {
	replacer := strings.NewReplacer("%", "%25", "/", "%2F", " ", "%20", "?", "%3F", "#", "%23")
	return replacer.Replace(value)
}
