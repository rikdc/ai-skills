package model

// Report is the single analysis document. CLI, Markdown, HTML and JSON are
// renderings of this value; no renderer recomputes scores, deltas or verdicts.
type Report struct {
	Version     int               `json:"version"`
	Metadata    ReportMetadata    `json:"metadata"`
	Totals      Totals            `json:"totals"`
	Timing      Timing            `json:"timing"`
	Cost        CostReport        `json:"cost"`        // accepted results
	Usage       CostReport        `json:"usage"`       // accepted results alias
	Expenditure Expenditure       `json:"expenditure"` // all attempts
	Verdict     Verdict           `json:"verdict"`
	Criteria    []CriterionResult `json:"criteria"`
	Scenarios   []ScenarioResult  `json:"scenarios"`
	Comparison  *Comparison       `json:"comparison,omitempty"`
	Links       Links             `json:"links"`

	// Warnings are conditions that weaken the verdict: skills never used,
	// retried or failed attempts, a judge that shares the agent's model, and
	// too few runs. Each is also rendered prominently by every format.
	Warnings []Warning `json:"warnings,omitempty"`
	// AttemptFailures summarises failed or interrupted attempts, which the
	// accepted run results otherwise hide.
	AttemptFailures *AttemptFailures `json:"attempt_failures,omitempty"`
}

// Warning codes.
const (
	WarnSkillNotExercised = "skill-not-exercised"
	WarnSkillUnknown      = "skill-use-unknown"
	WarnSameModelJudge    = "same-model-judge"
	WarnAttemptFailures   = "attempt-failures"
	WarnLowN              = "low-n"
)

// Warning is one report-level caveat.
type Warning struct {
	Code     string `json:"code"`
	Scenario string `json:"scenario,omitempty"`
	Message  string `json:"message"`
}

// AttemptFailureGroup counts the failed attempts of one kind.
type AttemptFailureGroup struct {
	Kind       AttemptKind `json:"kind"`
	Failed     int         `json:"failed"`
	Total      int         `json:"total"`
	FirstError string      `json:"first_error,omitempty"`
}

// AttemptFailures lists the attempt kinds that had failed or interrupted
// attempts. It is empty when every attempt succeeded.
type AttemptFailures struct {
	Failed int                   `json:"failed"`
	Groups []AttemptFailureGroup `json:"groups"`
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
	Functional      int            `json:"functional"`
	FunctionalWith  *float64       `json:"functional_with"`
	PendingPoints   float64        `json:"pending_points"`
	PendingCriteria int            `json:"pending_criteria"`
	Failures        int            `json:"failures"`
	NotComparable   int            `json:"not_comparable"`
	Regressions     int            `json:"regressions"`
	Counts          map[string]int `json:"counts,omitempty"`
	// SkillNotExercised counts comparison scenarios left out of With, Without,
	// Delta and the criterion tallies because the with arm never used the
	// skill, so they measure nothing about it.
	SkillNotExercised int `json:"skill_not_exercised,omitempty"`

	// Legacy v1 report counters retained for shell parity. Nilable numbers
	// stay pointers so an unavailable range emits null rather than zero.
	RunnerFailures        int      `json:"runner_failures"`
	FailedScenarios       int      `json:"failed_scenarios"`
	FailedRuns            int      `json:"failed_runs"`
	ResultsFailures       int      `json:"results_failures"`
	WithMin               *float64 `json:"with_min"`
	WithMax               *float64 `json:"with_max"`
	WithoutMin            *float64 `json:"without_min"`
	WithoutMax            *float64 `json:"without_max"`
	CoveragePercent       float64  `json:"coverage_percent"`
	ScoredCriteriaCells   int      `json:"scored_criteria_cells"`
	ExpectedCriteriaCells int      `json:"expected_criteria_cells"`
	Won                   int      `json:"won"`
	Lost                  int      `json:"lost"`
	Tied                  int      `json:"tied"`
}

// Timing is the suite wall-clock and invocation-time summary. TotalSeconds and
// MeanSeconds are the A1 fields; WallSeconds is the v1-compatible name and is
// the value the report actually renders.
type Timing struct {
	TotalSeconds float64  `json:"total_seconds,omitempty"`
	MeanSeconds  *float64 `json:"mean_seconds,omitempty"`
	WallSeconds  *float64 `json:"wall_seconds"`
	AgentSeconds *float64 `json:"agent_seconds"`
	JudgeSeconds *float64 `json:"judge_seconds"`
	Runs         int      `json:"runs"`
	TimedOutRuns int      `json:"timed_out_runs"`
}

// CostReport mirrors the v1 accepted-cost shape. Usage is the same shape with
// only the currency, with/without and delta blocks.
type CostReport struct {
	Currency          string           `json:"currency"`
	With              CostStats        `json:"with"`
	Without           CostStats        `json:"without"`
	Comparison        *CostComparison  `json:"comparison,omitempty"`
	DeltaTotals       *CostDeltaTotals `json:"delta_totals,omitempty"`
	Delta             *CostDelta       `json:"delta,omitempty"`
	CostPerDeltaPoint *float64         `json:"cost_per_delta_point,omitempty"`
}

// CostStats is the per-arm cost/token aggregation.
type CostStats struct {
	RunsTotal int           `json:"runs_total"`
	Agent     FieldStats    `json:"agent"`
	Judge     FieldStats    `json:"judge"`
	Combined  CombinedStats `json:"combined"`
}

// FieldStats aggregates one usage stream (agent or judge) across runs.
type FieldStats struct {
	RunsKnown int         `json:"runs_known"`
	RunsTotal int         `json:"runs_total"`
	Cost      MetricStats `json:"cost"`
	Tokens    TokenStats  `json:"tokens"`
}

// MetricStats is one metric's known total and mean.
type MetricStats struct {
	Total     *float64 `json:"total"`
	RunsKnown int      `json:"runs_known"`
	RunsTotal int      `json:"runs_total"`
	Mean      *float64 `json:"mean"`
}

// TokenStats is the token breakdown with an embedded total metric.
type TokenStats struct {
	MetricStats
	Input      MetricStats `json:"input"`
	Output     MetricStats `json:"output"`
	Reasoning  MetricStats `json:"reasoning"`
	CacheRead  MetricStats `json:"cache_read"`
	CacheWrite MetricStats `json:"cache_write"`
}

// CombinedStats aggregates agent+judge values where both are known.
type CombinedStats struct {
	RunsKnown int         `json:"runs_known"`
	RunsTotal int         `json:"runs_total"`
	Cost      MetricStats `json:"cost"`
	Tokens    TokenStats  `json:"tokens"`
}

// CostComparison splits the comparison-only accepted costs.
type CostComparison struct {
	With    CostStats `json:"with"`
	Without CostStats `json:"without"`
}

// CostDeltaTotals is the difference of comparison totals across arms.
type CostDeltaTotals struct {
	AgentCost               *float64 `json:"agent_cost"`
	JudgeCost               *float64 `json:"judge_cost"`
	CombinedCost            *float64 `json:"combined_cost"`
	CombinedInputTokens     *float64 `json:"combined_input_tokens"`
	CombinedOutputTokens    *float64 `json:"combined_output_tokens"`
	CombinedReasoningTokens *float64 `json:"combined_reasoning_tokens"`
	CombinedCacheReadTokens *float64 `json:"combined_cache_read_tokens"`
	CombinedTokens          *float64 `json:"combined_tokens"`
}

// CostDelta is the difference of per-run means across arms.
type CostDelta struct {
	AgentCost               *float64 `json:"agent_cost"`
	JudgeCost               *float64 `json:"judge_cost"`
	CombinedCost            *float64 `json:"combined_cost"`
	AgentTokens             *float64 `json:"agent_tokens"`
	JudgeTokens             *float64 `json:"judge_tokens"`
	CombinedTokens          *float64 `json:"combined_tokens"`
	AgentInputTokens        *float64 `json:"agent_input_tokens"`
	AgentOutputTokens       *float64 `json:"agent_output_tokens"`
	AgentReasoningTokens    *float64 `json:"agent_reasoning_tokens"`
	AgentCacheReadTokens    *float64 `json:"agent_cache_read_tokens"`
	JudgeInputTokens        *float64 `json:"judge_input_tokens"`
	JudgeOutputTokens       *float64 `json:"judge_output_tokens"`
	JudgeReasoningTokens    *float64 `json:"judge_reasoning_tokens"`
	JudgeCacheReadTokens    *float64 `json:"judge_cache_read_tokens"`
	CombinedInputTokens     *float64 `json:"combined_input_tokens"`
	CombinedOutputTokens    *float64 `json:"combined_output_tokens"`
	CombinedReasoningTokens *float64 `json:"combined_reasoning_tokens"`
	CombinedCacheReadTokens *float64 `json:"combined_cache_read_tokens"`
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

// Verdict is the report verdict. The JSON key is "state" to keep the v1
// report contract; Status is the A1 Go field name.
type Verdict struct {
	Status string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

// CriterionResult is one rubric row. Flat rows carry plugin/scenario; rows
// nested under a scenario leave them empty.
type CriterionResult struct {
	Plugin     string   `json:"plugin,omitempty"`
	Scenario   string   `json:"scenario,omitempty"`
	Name       string   `json:"name"`
	Max        float64  `json:"max"`
	With       *float64 `json:"with"`
	Without    *float64 `json:"without"`
	Delta      *float64 `json:"delta"`
	PairedRuns int      `json:"paired_runs"`
	Status     string   `json:"status"`
}

// ItemList is the per-run mechanical or judged item container.
type ItemList struct {
	Items []JudgedItem `json:"items"`
}

// RunRow is one run in a scenario report.
type RunRow struct {
	Arm                  string             `json:"arm"`
	Run                  int                `json:"run"`
	Status               string             `json:"status"`
	StopReason           string             `json:"stop_reason,omitempty"`
	AgentAttempt         string             `json:"agent_attempt,omitempty"`
	JudgeAttempt         string             `json:"judge_attempt,omitempty"`
	Complete             bool               `json:"complete"`
	AgentExit            *int               `json:"agent_exit"`
	TimedOut             bool               `json:"timed_out"`
	DurationSeconds      *float64           `json:"duration_seconds"`
	JudgeDurationSeconds *float64           `json:"judge_duration_seconds"`
	Mechanical           ItemList           `json:"mechanical"`
	Judged               *ItemList          `json:"judged"`
	Usage                Usage              `json:"usage"`
	AgentUsage           Usage              `json:"agent_usage"`
	JudgeUsage           Usage              `json:"judge_usage"`
	CombinedUsage        Usage              `json:"combined_usage"`
	Evidence             string             `json:"evidence"`
	Max                  float64            `json:"max"`
	Score                *float64           `json:"score"`
	Percent              *float64           `json:"percent"`
	PendingPoints        float64            `json:"pending_points"`
	PendingCount         int                `json:"pending_count"`
	Scored               map[string]float64 `json:"scored,omitempty"`
	SkillUse             *SkillUse          `json:"skill_use,omitempty"`
}

// ArmStats summarises one arm of one scenario.
type ArmStats struct {
	Runs          int       `json:"runs"`
	ScoreMean     *float64  `json:"score_mean"`
	PercentMean   *float64  `json:"percent_mean"`
	PercentMin    *float64  `json:"percent_min"`
	PercentMax    *float64  `json:"percent_max"`
	PendingPoints float64   `json:"pending_points"`
	Failures      int       `json:"failures"`
	Timing        ArmTiming `json:"timing"`
}

// ArmTiming sums invocation time over an arm.
type ArmTiming struct {
	AgentSeconds *float64 `json:"agent_seconds"`
	JudgeSeconds *float64 `json:"judge_seconds"`
	TimedOutRuns int      `json:"timed_out_runs"`
}

// ArmPair holds both scenario arms.
type ArmPair struct {
	With    ArmStats `json:"with"`
	Without ArmStats `json:"without"`
}

// ScenarioResult is one analysed scenario.
type ScenarioResult struct {
	Plugin          string            `json:"plugin"`
	Scenario        string            `json:"scenario"`
	Units           []string          `json:"units"`
	Mode            string            `json:"mode"`
	Functional      bool              `json:"functional"`
	ArmCount        int               `json:"arm_count"`
	Denominator     float64           `json:"denominator"`
	CriteriaCount   int               `json:"criteria_count"`
	ConfiguredRuns  int               `json:"configured_runs"`
	Status          string            `json:"status"`
	Runs            []RunRow          `json:"runs"`
	Arms            ArmPair           `json:"arms"`
	Delta           *float64          `json:"delta"`
	PendingPoints   float64           `json:"pending_points"`
	PendingCriteria int               `json:"pending_criteria"`
	Criteria        []CriterionResult `json:"criteria"`
	Regressions     []CriterionResult `json:"regressions"`
	// Skill summarises skill use over the with-arm runs; absent when the
	// scenario installs no skills.
	Skill *SkillCoverage `json:"skill,omitempty"`
}

// ComparisonExclusion records why a scenario did not contribute a delta.
type ComparisonExclusion struct {
	Side     string `json:"side"`
	Plugin   string `json:"plugin"`
	Scenario string `json:"scenario"`
	Reason   string `json:"reason"`
}

// ComparisonMetric is one preformatted value pair for a baseline side.
type ComparisonMetric struct {
	With                *float64 `json:"with"`
	Without             *float64 `json:"without"`
	Delta               *float64 `json:"delta"`
	CostWithPerRun      *float64 `json:"cost_with_per_run"`
	CostWithoutPerRun   *float64 `json:"cost_without_per_run"`
	InputWithPerRun     *float64 `json:"input_with_per_run"`
	OutputWithPerRun    *float64 `json:"output_with_per_run"`
	InputWithoutPerRun  *float64 `json:"input_without_per_run"`
	OutputWithoutPerRun *float64 `json:"output_without_per_run"`
}

// ComparisonCost is per-run cost for a baseline side.
type ComparisonCost struct {
	AgentPerRun   *float64 `json:"agent_per_run"`
	JudgePerRun   *float64 `json:"judge_per_run"`
	WithPerRun    *float64 `json:"with_per_run"`
	WithoutPerRun *float64 `json:"without_per_run"`
	PairPerRun    *float64 `json:"pair_per_run"`
}

// ComparisonTokens is per-run combined tokens for a baseline side.
type ComparisonTokens struct {
	WithInputPerRun     *float64 `json:"with_input_per_run"`
	WithOutputPerRun    *float64 `json:"with_output_per_run"`
	WithTotalPerRun     *float64 `json:"with_total_per_run"`
	WithoutInputPerRun  *float64 `json:"without_input_per_run"`
	WithoutOutputPerRun *float64 `json:"without_output_per_run"`
	WithoutTotalPerRun  *float64 `json:"without_total_per_run"`
	PairInputPerRun     *float64 `json:"pair_input_per_run"`
	PairOutputPerRun    *float64 `json:"pair_output_per_run"`
	PairTotalPerRun     *float64 `json:"pair_total_per_run"`
}

// ComparisonTiming is the timing view of a baseline side.
type ComparisonTiming struct {
	WallSeconds  *float64 `json:"wall_seconds"`
	AgentSeconds *float64 `json:"agent_seconds"`
	JudgeSeconds *float64 `json:"judge_seconds"`
}

// ComparisonSide is the baseline or current side of a comparison.
type ComparisonSide struct {
	SuiteDir  string           `json:"suite_dir"`
	CreatedAt string           `json:"created_at"`
	GitSHA    string           `json:"git_sha"`
	N         int              `json:"n"`
	Verdict   string           `json:"verdict"`
	With      *float64         `json:"with"`
	Without   *float64         `json:"without"`
	Delta     *float64         `json:"delta"`
	Cost      ComparisonCost   `json:"cost"`
	Tokens    ComparisonTokens `json:"tokens"`
	Timing    ComparisonTiming `json:"timing"`
}

// ComparisonDelta is current minus baseline.
type ComparisonDelta struct {
	WithScore    *float64 `json:"with_score"`
	WithoutScore *float64 `json:"without_score"`
	Effect       *float64 `json:"effect"`

	AgentCostPerRun   *float64 `json:"agent_cost_per_run"`
	JudgeCostPerRun   *float64 `json:"judge_cost_per_run"`
	WithCostPerRun    *float64 `json:"with_cost_per_run"`
	WithoutCostPerRun *float64 `json:"without_cost_per_run"`
	PairCostPerRun    *float64 `json:"pair_cost_per_run"`

	WithInputTokensPerRun     *float64 `json:"with_input_tokens_per_run"`
	WithOutputTokensPerRun    *float64 `json:"with_output_tokens_per_run"`
	WithTotalTokensPerRun     *float64 `json:"with_total_tokens_per_run"`
	WithoutInputTokensPerRun  *float64 `json:"without_input_tokens_per_run"`
	WithoutOutputTokensPerRun *float64 `json:"without_output_tokens_per_run"`
	WithoutTotalTokensPerRun  *float64 `json:"without_total_tokens_per_run"`

	WallSeconds  *float64 `json:"wall_seconds"`
	AgentSeconds *float64 `json:"agent_seconds"`
	JudgeSeconds *float64 `json:"judge_seconds"`
}

// ComparisonScenario is one matched scenario's current/baseline view.
type ComparisonScenario struct {
	Plugin   string           `json:"plugin"`
	Scenario string           `json:"scenario"`
	Current  ComparisonMetric `json:"current"`
	Baseline ComparisonMetric `json:"baseline"`
	Delta    ComparisonMetric `json:"delta"`
}

// Comparison is the baseline comparison block.
type Comparison struct {
	Status              string                `json:"status"`
	Comparable          bool                  `json:"comparable"`
	ComparableScenarios int                   `json:"comparable_scenarios"`
	CurrentScenarios    int                   `json:"current_scenarios"`
	BaselineScenarios   int                   `json:"baseline_scenarios"`
	ExcludedScenarios   int                   `json:"excluded_scenarios"`
	Excluded            []ComparisonExclusion `json:"excluded"`
	Baseline            *ComparisonSide       `json:"baseline"`
	Current             *ComparisonSide       `json:"current"`
	Delta               *ComparisonDelta      `json:"delta"`
	Scenarios           []ComparisonScenario  `json:"scenarios"`
	MeasurementWarning  string                `json:"measurement_warning,omitempty"`
}

// EvidenceLink is a relative path to scenario evidence.
type EvidenceLink struct {
	Plugin   string `json:"plugin"`
	Scenario string `json:"scenario"`
	Path     string `json:"path"`
}

// Links points at the other generated renderings.
type Links struct {
	JSON     string         `json:"json,omitempty"`
	Markdown string         `json:"markdown,omitempty"`
	HTML     string         `json:"html,omitempty"`
	Evidence []EvidenceLink `json:"evidence,omitempty"`
}

// SuiteState is the analysis-relevant part of suite.json.
type SuiteState struct {
	Counts          map[string]int `json:"counts"`
	DurationSeconds *float64       `json:"duration_seconds,omitempty"`
}

// ResultRecord is one results.jsonl row, used only for failure accounting.
type ResultRecord struct {
	Plugin   string `json:"plugin"`
	Scenario string `json:"scenario"`
	Status   string `json:"status"`
}

// ScenarioInput is one scenario loaded from a suite.
type ScenarioInput struct {
	Plugin      string
	Scenario    string
	Units       []string
	Mode        string
	Execution   string
	Fingerprint string
	Rubric      []Criterion
	Runs        []RunInput
}

// RunInput is one loaded run before analysis. Judged items are present only
// when the judgement passed its binding and rubric validation.
type RunInput struct {
	Arm                  string
	Run                  int
	Status               string
	StopReason           string
	AgentAttempt         string
	JudgeAttempt         string
	Complete             bool
	AgentExit            *int
	TimedOut             bool
	DurationSeconds      *float64
	JudgeDurationSeconds *float64
	MechanicalItems      []JudgedItem
	JudgedItems          []JudgedItem
	Usage                Usage
	AgentUsage           Usage
	JudgeUsage           Usage
	CombinedUsage        Usage
	Evidence             string
	SkillUse             *SkillUse
}

// AttemptInput is one attempt's usage for expenditure accounting.
type AttemptInput struct {
	Kind  AttemptKind
	Usage Usage
	// Status is the finished status, or interrupted when no finished record
	// exists. Error is the recorded failure text, if any.
	Status RunStatus
	Error  string
}

// ReportInput is the read-only suite view reportio hands to the pure analysis.
type ReportInput struct {
	SuiteDir        string
	Manifest        SuiteManifest
	ManifestVersion int
	State           SuiteState
	Results         []ResultRecord
	Scenarios       []ScenarioInput
	Attempts        []AttemptInput
}
