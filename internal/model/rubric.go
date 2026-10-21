package model

// RubricTypeWeightedChecklist is the only supported criteria.json type.
const RubricTypeWeightedChecklist = "weighted_checklist"

// Authoring limits for criteria.json.
const (
	RubricMinItems     = 8
	RubricMaxItems     = 13
	RubricAuthoringSum = 100
)

// Denominator returns the fixed rubric denominator: the sum of every
// criterion's maximum, regardless of how many were scored.
func (r Rubric) Denominator() float64 {
	var total float64
	for _, item := range r.Checklist {
		total += item.MaxScore
	}
	return total
}
