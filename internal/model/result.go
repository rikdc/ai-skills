package model

// Mechanical is the mechanically scored portion of a rubric.
type Mechanical struct {
	Score float64      `json:"score"`
	Max   float64      `json:"max"`
	Items []JudgedItem `json:"items"`
}

// RunResult is the derived result.json projection. The authoritative source is
// current.json plus immutable attempt directories.
type RunResult struct {
	SchemaVersion        int        `json:"schema_version"`
	Plugin               string     `json:"plugin"`
	Scenario             string     `json:"scenario"`
	Arm                  string     `json:"arm"`
	Mode                 string     `json:"mode"`
	Run                  int        `json:"run"`
	AgentAttempt         string     `json:"agent_attempt"`
	JudgeAttempt         string     `json:"judge_attempt,omitempty"`
	CurrentSHA256        string     `json:"current_sha256"`
	Complete             bool       `json:"complete"`
	Status               RunStatus  `json:"status"`
	StopReason           string     `json:"stop_reason,omitempty"`
	Backend              string     `json:"backend,omitempty"`
	Model                string     `json:"model,omitempty"`
	Fingerprint          string     `json:"fingerprint"`
	ChecksRequired       bool       `json:"checks_required"`
	AgentExit            *int       `json:"agent_exit"`
	TimedOut             bool       `json:"timed_out"`
	DurationSeconds      *float64   `json:"duration_seconds"`
	StartedAt            string     `json:"started_at,omitempty"`
	FinishedAt           string     `json:"finished_at,omitempty"`
	Mechanical           Mechanical `json:"mechanical"`
	Usage                Usage      `json:"usage"`
	AgentUsage           Usage      `json:"agent_usage"`
	JudgeUsage           Usage      `json:"judge_usage"`
	CombinedUsage        Usage      `json:"combined_usage"`
	Evidence             string     `json:"evidence"`
	EvidenceSHA256       string     `json:"evidence_sha256"`
	GradingRequestSHA256 string     `json:"grading_request_sha256"`
	JudgeIdentitySHA256  string     `json:"judge_identity_sha256,omitempty"`
	Error                string     `json:"error,omitempty"`

	// SkillUse records whether the with arm used the installed skill units.
	// It is derived from the accepted agent log and absent for the without
	// arm and for scenarios that install no skills.
	SkillUse *SkillUse `json:"skill_use,omitempty"`
}
