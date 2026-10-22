package model

// Criterion is one rubric checklist item.
type Criterion struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	MaxScore    float64 `json:"max_score"`
}

// Rubric is the fixed-denominator checklist for a scenario.
type Rubric struct {
	Context   string      `json:"context"`
	Type      string      `json:"type"`
	Checklist []Criterion `json:"checklist"`
}

// GradingRequest identifies the accepted agent execution and the criteria a
// judge still has to score.
type GradingRequest struct {
	SchemaVersion    int         `json:"schema_version"`
	Plugin           string      `json:"plugin"`
	Scenario         string      `json:"scenario"`
	Arm              string      `json:"arm"`
	Run              int         `json:"run"`
	AgentAttempt     string      `json:"agent_attempt"`
	EvidenceSHA256   string      `json:"evidence_sha256"`
	Criteria         Rubric      `json:"criteria"`
	CoveredByChecks  []string    `json:"covered_by_checks"`
	JudgementPending []Criterion `json:"judgement_pending"`
}

// JudgedItem is one scored criterion from mechanics or a judge.
type JudgedItem struct {
	Name  string  `json:"name"`
	Score float64 `json:"score"`
	Max   float64 `json:"max"`
	Note  string  `json:"note,omitempty"`
}

// Judgement is the raw judgement.json document: a bare JSON array.
type Judgement []JudgedItem
