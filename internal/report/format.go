package report

import (
	"fmt"
	"math"
	"strconv"

	"github.com/rikdc/ai-skills/internal/model"
)

const dash = "—"

func fmtScore(value *float64) string {
	if value == nil {
		return dash
	}
	return strconv.FormatFloat(*value, 'f', 1, 64)
}

func fmtDelta(value *float64) string {
	if value == nil {
		return dash
	}
	if *value > 0 {
		return "+" + strconv.FormatFloat(*value, 'f', 1, 64)
	}
	return strconv.FormatFloat(*value, 'f', 1, 64)
}

func fmtMoney(value *float64) string {
	if value == nil {
		return dash
	}
	return fmt.Sprintf("$%.3f", *value)
}

func roundTo(value float64, digits int) float64 {
	scale := math.Pow(10, float64(digits))
	return math.Round(value*scale) / scale
}

func fixed(value float64, digits int) string {
	return strconv.FormatFloat(roundTo(value, digits), 'f', digits, 64)
}

func fmtMoneyDelta(value *float64) string {
	if value == nil {
		return dash
	}
	v := *value
	switch {
	case v == 0:
		return "$0.000"
	case v > 0:
		if v >= 0.001 {
			return "+$" + fixed(v, 3)
		}
		if v >= 0.0001 {
			return "+$" + fixed(v, 5)
		}
		return "< $0.0001"
	default:
		a := -v
		if a >= 0.001 {
			return "-$" + fixed(a, 3)
		}
		if a >= 0.0001 {
			return "-$" + fixed(a, 5)
		}
		return "> -$0.0001"
	}
}

func fmtTokens(value *float64) string {
	if value == nil {
		return dash
	}
	n := *value
	switch {
	case n >= 1000000:
		return strconv.FormatFloat(roundTo(n/1000000, 1), 'f', 1, 64) + "M"
	case n >= 1000:
		return strconv.FormatFloat(roundTo(n/1000, 1), 'f', 1, 64) + "k"
	default:
		return strconv.FormatFloat(math.Round(n), 'f', 0, 64)
	}
}

func fmtTokensDelta(value *float64) string {
	if value == nil {
		return dash
	}
	sign := ""
	n := *value
	if n > 0 {
		sign = "+"
	} else if n < 0 {
		sign = "-"
		n = -n
	}
	switch {
	case n >= 1000000:
		return sign + strconv.FormatFloat(roundTo(n/1000000, 1), 'f', 1, 64) + "M"
	case n >= 1000:
		return sign + strconv.FormatFloat(roundTo(n/1000, 1), 'f', 1, 64) + "k"
	default:
		return sign + strconv.FormatFloat(math.Round(n), 'f', 0, 64)
	}
}

func fmtSeconds(value *float64) string {
	if value == nil {
		return dash
	}
	n := *value
	switch {
	case n >= 3600:
		return strconv.FormatFloat(roundTo(n/3600, 1), 'f', 1, 64) + "h"
	case n >= 60:
		return strconv.FormatFloat(math.Round(n/60), 'f', 0, 64) + "m"
	default:
		return strconv.FormatFloat(math.Round(n), 'f', 0, 64) + "s"
	}
}

func fmtSecondsDelta(value *float64) string {
	if value == nil {
		return dash
	}
	sign := ""
	n := *value
	if n > 0 {
		sign = "+"
	} else if n < 0 {
		sign = "-"
		n = -n
	}
	switch {
	case n >= 3600:
		return sign + strconv.FormatFloat(roundTo(n/3600, 1), 'f', 1, 64) + "h"
	case n >= 60:
		return sign + strconv.FormatFloat(math.Round(n/60), 'f', 0, 64) + "m"
	default:
		return sign + strconv.FormatFloat(math.Round(n), 'f', 0, 64) + "s"
	}
}

func fmtCostPerPoint(value *float64) string {
	if value == nil {
		return dash
	}
	v := *value
	switch {
	case v <= 0:
		return dash
	case v >= 0.001:
		return fmt.Sprintf("$%.3f", v)
	case v >= 0.0001:
		return fmt.Sprintf("$%.5f", v)
	default:
		return "< $0.0001"
	}
}

func fmtCoverage(stats model.MetricStats) string {
	return fmt.Sprintf("%d/%d", stats.RunsKnown, stats.RunsTotal)
}

func deltaArrow(value *float64) string {
	switch {
	case value == nil:
		return dash
	case *value > Epsilon:
		return "▲"
	case *value < -Epsilon:
		return "▼"
	default:
		return "="
	}
}

func hasKnownCost(report model.Report) bool {
	values := []*float64{
		report.Cost.With.Agent.Cost.Total, report.Cost.Without.Agent.Cost.Total,
		report.Cost.With.Judge.Cost.Total, report.Cost.Without.Judge.Cost.Total,
		report.Cost.With.Combined.Tokens.Total, report.Cost.Without.Combined.Tokens.Total,
	}
	for _, value := range values {
		if value != nil {
			return true
		}
	}
	return false
}

func totalRuns(report model.Report) int {
	total := 0
	for _, sc := range report.Scenarios {
		total += len(sc.Runs)
	}
	return total
}

func sortScenarios(scenarios []model.ScenarioResult) []model.ScenarioResult {
	out := append([]model.ScenarioResult(nil), scenarios...)
	value := func(sc model.ScenarioResult) float64 {
		if sc.Delta == nil {
			return -1000000
		}
		return *sc.Delta
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && value(out[j]) > value(out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func sortCriteria(criteria []model.CriterionResult) []model.CriterionResult {
	out := append([]model.CriterionResult(nil), criteria...)
	less := func(a, b model.CriterionResult) bool {
		aNil, bNil := a.Delta == nil, b.Delta == nil
		if aNil != bNil {
			return !aNil
		}
		if aNil {
			return false
		}
		return *a.Delta < *b.Delta
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && less(out[j], out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func regressions(criteria []model.CriterionResult) []model.CriterionResult {
	var out []model.CriterionResult
	for _, criterion := range criteria {
		if criterion.Delta != nil && *criterion.Delta < -Epsilon {
			out = append(out, criterion)
		}
	}
	return out
}

func scenarioLabel(sc model.ScenarioResult) string {
	return sc.Plugin + "/" + sc.Scenario
}

func criterionLabel(criterion model.CriterionResult) string {
	return criterion.Plugin + "/" + criterion.Scenario
}
