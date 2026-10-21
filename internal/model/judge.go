package model

// GenerationIdentity is the canonical generation configuration of a judge.
type GenerationIdentity struct {
	Temperature     string `json:"temperature"`
	Seed            *int64 `json:"seed"`
	MaxOutputTokens int    `json:"max_output_tokens"`
	MaxSteps        int    `json:"max_steps"`
	ContextWindow   int    `json:"context_window"`
}

// JudgeIdentity is the canonical, hashed identity of a judge configuration.
type JudgeIdentity struct {
	Kind          string             `json:"kind"`
	Command       string             `json:"command,omitempty"`
	Provider      string             `json:"provider,omitempty"`
	BaseURL       string             `json:"base_url,omitempty"`
	Model         string             `json:"model,omitempty"`
	Generation    GenerationIdentity `json:"generation"`
	PromptVersion string             `json:"prompt_version"`
	GraderVersion string             `json:"grader_version"`
	SchemaVersion int                `json:"schema_version"`
}

// JudgeResult is the judge-result.json skeleton. Identity, hash, and the
// three evidence-binding fields are mandatory for acceptance and reuse.
type JudgeResult struct {
	SchemaVersion        int           `json:"schema_version"`
	ExitCode             int           `json:"exit_code"`
	TimedOut             bool          `json:"timed_out"`
	DurationSeconds      *float64      `json:"duration_seconds,omitempty"`
	Valid                bool          `json:"valid"`
	Identity             JudgeIdentity `json:"identity"`
	JudgeIdentitySHA256  string        `json:"judge_identity_sha256"`
	JudgeCmd             string        `json:"judge_cmd"`
	Attempt              string        `json:"attempt"`
	GradedAgentAttempt   string        `json:"graded_agent_attempt"`
	GradedEvidenceSHA256 string        `json:"graded_evidence_sha256"`
	GradingRequestSHA256 string        `json:"grading_request_sha256"`
	JudgementSHA256      string        `json:"judgement_sha256"`
	Error                string        `json:"error,omitempty"`
}
