package model

// Report is the v3 analysis document skeleton. Report construction and the
// complete renderer model are owned by the reporting increment; this type
// fixes only the top-level contract.
type Report struct {
	Version     int               `json:"version"`
	Metadata    ReportMetadata    `json:"metadata"`
	Totals      Totals            `json:"totals"`
	Timing      Timing            `json:"timing"`
	Cost        CostReport        `json:"cost"`
	Usage       CostReport        `json:"usage"`
	Expenditure Expenditure       `json:"expenditure"`
	Verdict     Verdict           `json:"verdict"`
	Criteria    []CriterionResult `json:"criteria"`
	Scenarios   []ScenarioResult  `json:"scenarios"`
	Comparison  *Comparison       `json:"comparison,omitempty"`
	Links       Links             `json:"links"`
}

// ReportMetadata identifies the suite a report was built from.
type ReportMetadata struct {
	SuiteDir         string         `json:"suite_dir"`
	Label            string         `json:"label,omitempty"`
	Units            []string       `json:"units,omitempty"`
	AgentCmd         string         `json:"agent_cmd,omitempty"`
	JudgeCmd         string         `json:"judge_cmd,omitempty"`
	Model            string         `json:"model,omitempty"`
	Backend          string         `json:"backend,omitempty"`
	JudgeModel       string         `json:"judge_model,omitempty"`
	GitSHA           string         `json:"git_sha,omitempty"`
	GitDirty         bool           `json:"git_dirty"`
	CreatedAt        string         `json:"created_at,omitempty"`
	Arm              string         `json:"arm,omitempty"`
	N                int            `json:"n"`
	AgentInvocations int            `json:"agent_invocations"`
	Counts           map[string]int `json:"counts,omitempty"`
}

// Totals is the aggregate score block. Nullable numbers emit null, never zero.
type Totals struct {
	With            *float64       `json:"with"`
	Without         *float64       `json:"without"`
	Delta           *float64       `json:"delta"`
	N               int            `json:"n"`
	Functional      bool           `json:"functional"`
	FunctionalWith  *float64       `json:"functional_with,omitempty"`
	PendingPoints   float64        `json:"pending_points"`
	PendingCriteria int            `json:"pending_criteria"`
	Failures        int            `json:"failures"`
	NotComparable   int            `json:"not_comparable"`
	Regressions     int            `json:"regressions"`
	Counts          map[string]int `json:"counts,omitempty"`
}

// Timing is the suite wall-clock summary.
type Timing struct {
	TotalSeconds float64  `json:"total_seconds"`
	MeanSeconds  *float64 `json:"mean_seconds,omitempty"`
}

// CostReport mirrors the accepted-cost buckets. Skeleton until reporting.
type CostReport struct {
	Currency string `json:"currency"`
}

// Expenditure counts every attempt, including superseded and failed ones.
type Expenditure struct {
	Currency              string `json:"currency"`
	KnownTotal            Usage  `json:"known_total"`
	Complete              bool   `json:"complete"`
	TotalAttempts         int    `json:"total_attempts"`
	KnownAttempts         int    `json:"known_attempts"`
	UnknownAttempts       int    `json:"unknown_attempts"`
	NotApplicableAttempts int    `json:"not_applicable_attempts"`
	AgentAttempts         int    `json:"agent_attempts"`
	HookAttempts          int    `json:"hook_attempts"`
	JudgeAttempts         int    `json:"judge_attempts"`
}

// Verdict is the report verdict.
type Verdict struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// CriterionResult is one flattened rubric row.
type CriterionResult struct {
	Name       string   `json:"name"`
	Max        float64  `json:"max"`
	With       *float64 `json:"with"`
	Without    *float64 `json:"without"`
	Delta      *float64 `json:"delta"`
	PairedRuns int      `json:"paired_runs"`
	Status     string   `json:"status"`
}

// ScenarioResult is one scenario row. Skeleton until reporting.
type ScenarioResult struct {
	Plugin   string `json:"plugin"`
	Scenario string `json:"scenario"`
	Mode     string `json:"mode"`
	Status   string `json:"status"`
}

// Comparison is the baseline comparison block.
type Comparison struct {
	Status             string `json:"status"`
	Comparable         bool   `json:"comparable"`
	MeasurementWarning string `json:"measurement_warning,omitempty"`
}

// Links points at the other generated renderings.
type Links struct {
	JSON     string `json:"json,omitempty"`
	Markdown string `json:"markdown,omitempty"`
	HTML     string `json:"html,omitempty"`
}
