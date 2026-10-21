package judge

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/rikdc/ai-skills/internal/model"
)

// ValidateJudgement enforces the spec §5.2 strict judgement contract against
// the pending criteria of a grading request. It returns the parsed items in
// their submitted order. Any error means the judgement is rejected and the
// criteria stay pending: never partially or over-scored.
func ValidateJudgement(raw []byte, pending []model.Criterion) (model.Judgement, error) {
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("judgement is not a JSON array: %v", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("judgement array is empty")
	}

	expected := make(map[string]model.Criterion, len(pending))
	for _, criterion := range pending {
		expected[criterion.Name] = criterion
	}

	judgement := make(model.Judgement, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	for index, entry := range entries {
		var item struct {
			Name  *string  `json:"name"`
			Score *float64 `json:"score"`
			Max   *float64 `json:"max"`
			Note  *string  `json:"note"`
		}
		if err := json.Unmarshal(entry, &item); err != nil {
			return nil, fmt.Errorf("item %d: %v", index+1, err)
		}
		if item.Name == nil {
			return nil, fmt.Errorf("item %d: missing name", index+1)
		}
		name := *item.Name
		if name == "" {
			return nil, fmt.Errorf("item %d: empty criterion name", index+1)
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate criterion %q", name)
		}
		criterion, ok := expected[name]
		if !ok {
			return nil, fmt.Errorf("unknown criterion %q", name)
		}
		if item.Score == nil {
			return nil, fmt.Errorf("criterion %q: missing score", name)
		}
		if item.Max == nil {
			return nil, fmt.Errorf("criterion %q: missing max", name)
		}
		score, max := *item.Score, *item.Max
		if math.IsNaN(score) || math.IsInf(score, 0) {
			return nil, fmt.Errorf("criterion %q: score is not finite", name)
		}
		if math.IsNaN(max) || math.IsInf(max, 0) {
			return nil, fmt.Errorf("criterion %q: max is not finite", name)
		}
		if score < 0 {
			return nil, fmt.Errorf("criterion %q: score %s < 0", name, formatScore(score))
		}
		if score > max {
			return nil, fmt.Errorf("criterion %q: score %s > max %s",
				name, formatScore(score), formatScore(max))
		}
		if max != criterion.MaxScore {
			return nil, fmt.Errorf("criterion %q: max %s does not equal rubric max %s",
				name, formatScore(max), formatScore(criterion.MaxScore))
		}
		seen[name] = true
		judged := model.JudgedItem{Name: name, Score: score, Max: max}
		if item.Note != nil {
			judged.Note = *item.Note
		}
		judgement = append(judgement, judged)
	}

	for _, criterion := range pending {
		if !seen[criterion.Name] {
			return nil, fmt.Errorf("missing criterion %q", criterion.Name)
		}
	}
	return judgement, nil
}

// JudgementScore sums a validated judgement.
func JudgementScore(judgement model.Judgement) (score, max float64) {
	for _, item := range judgement {
		score += item.Score
		max += item.Max
	}
	return score, max
}
