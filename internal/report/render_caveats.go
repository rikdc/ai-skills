package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rikdc/ai-skills/internal/model"
)

// skillRow is one with-arm run's recorded skill use.
type skillRow struct {
	scenario string
	run      int
	state    string
	detail   string
}

func skillRows(report model.Report) []skillRow {
	var rows []skillRow
	for _, sc := range sortScenarios(report.Scenarios) {
		for _, run := range sc.Runs {
			if run.SkillUse == nil {
				continue
			}
			names := make([]string, 0, len(run.SkillUse.Skills))
			for name := range run.SkillUse.Skills {
				names = append(names, name)
			}
			sort.Strings(names)
			parts := make([]string, 0, len(names))
			for _, name := range names {
				parts = append(parts, name+": "+run.SkillUse.Skills[name])
			}
			rows = append(rows, skillRow{
				scenario: scenarioLabel(sc), run: run.Run,
				state: run.SkillUse.State, detail: strings.Join(parts, ", "),
			})
		}
	}
	return rows
}

func failureLine(group model.AttemptFailureGroup) string {
	return fmt.Sprintf("%d of %d %s attempts failed; first error: %s",
		group.Failed, group.Total, group.Kind, group.FirstError)
}

func renderCaveatsCLI(report model.Report, bold func(code, text string) string) string {
	var b strings.Builder
	if len(report.Warnings) > 0 {
		fmt.Fprintf(&b, "%s\n", bold("1;31", "Warnings"))
		for _, warning := range report.Warnings {
			fmt.Fprintf(&b, "  ! [%s] %s\n", warning.Code, warning.Message)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func renderDetailsCLI(report model.Report, bold func(code, text string) string) string {
	var b strings.Builder
	if rows := skillRows(report); len(rows) > 0 {
		fmt.Fprintf(&b, "\n%s\n", bold("1", "Skill use (with arm)"))
		fmt.Fprintf(&b, "  %-44s %4s  %-12s %s\n", "SCENARIO", "RUN", "SKILL USE", "SKILLS")
		for _, row := range rows {
			fmt.Fprintf(&b, "  %-44s %4d  %-12s %s\n", row.scenario, row.run, row.state, row.detail)
		}
	}
	if failures := report.AttemptFailures; failures != nil {
		fmt.Fprintf(&b, "\n%s\n", bold("1", "Failed attempts"))
		for _, group := range failures.Groups {
			fmt.Fprintf(&b, "  %s\n", failureLine(group))
		}
	}
	return b.String()
}

func renderCaveatsMarkdown(report model.Report) string {
	if len(report.Warnings) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Warnings\n\n")
	for _, warning := range report.Warnings {
		fmt.Fprintf(&b, "- **%s**: %s\n", mdEscape(warning.Code), mdEscape(warning.Message))
	}
	b.WriteString("\n")
	return b.String()
}

func renderDetailsMarkdown(report model.Report) string {
	var b strings.Builder
	if rows := skillRows(report); len(rows) > 0 {
		b.WriteString("\n## Skill use\n\n")
		b.WriteString("Detected from the with-arm agent log. `unknown` means the log was missing or in an unrecognised format; it is never read as not invoked.\n\n")
		b.WriteString("| Scenario | Run | Skill use | Skills |\n| --- | --- | --- | --- |\n")
		for _, row := range rows {
			fmt.Fprintf(&b, "| %s | %d | %s | %s |\n",
				mdEscape(row.scenario), row.run, mdEscape(row.state), mdEscape(row.detail))
		}
	}
	if failures := report.AttemptFailures; failures != nil {
		b.WriteString("\n## Failed attempts\n\n")
		b.WriteString("| Kind | Failed | Total | First error |\n| --- | --- | --- | --- |\n")
		for _, group := range failures.Groups {
			fmt.Fprintf(&b, "| %s | %d | %d | %s |\n",
				group.Kind, group.Failed, group.Total, mdEscape(group.FirstError))
		}
	}
	return b.String()
}

func renderCaveatsHTML(report model.Report) string {
	if len(report.Warnings) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<section class=\"warnings\"><div class=\"state\">Warnings</div><ul>\n")
	for _, warning := range report.Warnings {
		fmt.Fprintf(&b, "<li><code>%s</code> %s</li>\n", esc(warning.Code), esc(warning.Message))
	}
	b.WriteString("</ul></section>\n")
	return b.String()
}

func renderDetailsHTML(report model.Report) string {
	var b strings.Builder
	if rows := skillRows(report); len(rows) > 0 {
		b.WriteString("<h2>Skill use</h2>\n")
		b.WriteString("<p class=\"note\">Detected from the with-arm agent log. <code>unknown</code> means the log was missing or in an unrecognised format; it is never read as not invoked.</p>\n")
		b.WriteString("<table><thead><tr><th>Scenario</th><th>Run</th><th>Skill use</th><th>Skills</th></tr></thead><tbody>\n")
		for _, row := range rows {
			class := ""
			if row.state == model.SkillNotInvoked {
				class = " class=\"neg\""
			}
			fmt.Fprintf(&b, "<tr><td>%s</td><td class=\"num\">%d</td><td%s>%s</td><td>%s</td></tr>\n",
				esc(row.scenario), row.run, class, esc(row.state), esc(row.detail))
		}
		b.WriteString("</tbody></table>\n")
	}
	if failures := report.AttemptFailures; failures != nil {
		b.WriteString("<h2>Failed attempts</h2>\n")
		b.WriteString("<table><thead><tr><th>Kind</th><th>Failed</th><th>Total</th><th>First error</th></tr></thead><tbody>\n")
		for _, group := range failures.Groups {
			fmt.Fprintf(&b, "<tr><td>%s</td><td class=\"num\">%d</td><td class=\"num\">%d</td><td>%s</td></tr>\n",
				esc(string(group.Kind)), group.Failed, group.Total, esc(group.FirstError))
		}
		b.WriteString("</tbody></table>\n")
	}
	return b.String()
}
