package model

// AttemptKind identifies the work an attempt directory records.
type AttemptKind string

const (
	KindAgent AttemptKind = "agent"
	KindHook  AttemptKind = "hook"
	KindJudge AttemptKind = "judge"
)

// Checks execution outcomes recorded in FinishedAttempt.ChecksStatus.
const (
	ChecksOK            = "ok"
	ChecksNotApplicable = "not-applicable"
	ChecksFailed        = "failed"
)

// StartedAttempt is the immutable record written and fsynced before any setup
// or paid work starts.
type StartedAttempt struct {
	SchemaVersion int         `json:"schema_version"`
	AttemptID     string      `json:"attempt_id"`
	Kind          AttemptKind `json:"kind"`
	RunID         string      `json:"run_id"`
	StartedAt     string      `json:"started_at"`
	Fingerprint   string      `json:"fingerprint"`
	ConfigSHA256  string      `json:"config_sha256"`
	Executor      string      `json:"executor"`
	Model         string      `json:"model,omitempty"`
	Command       string      `json:"command,omitempty"`
}

// FinishedAttempt is the immutable outcome of an attempt.
type FinishedAttempt struct {
	SchemaVersion        int         `json:"schema_version"`
	AttemptID            string      `json:"attempt_id"`
	Kind                 AttemptKind `json:"kind"`
	FinishedAt           string      `json:"finished_at"`
	Status               RunStatus   `json:"status"`
	StopReason           string      `json:"stop_reason,omitempty"`
	Error                string      `json:"error,omitempty"`
	Fingerprint          string      `json:"fingerprint,omitempty"`
	Usage                Usage       `json:"usage"`
	ChecksStatus         string      `json:"checks_status,omitempty"`
	MechanicalSHA256     string      `json:"mechanical_sha256,omitempty"`
	EvidenceSHA256       string      `json:"evidence_sha256,omitempty"`
	GradingRequestSHA256 string      `json:"grading_request_sha256,omitempty"`

	// Execution observability. AgentExit is nil when the command never
	// started. DurationSeconds is the observed whole invocation time.
	AgentExit       *int     `json:"agent_exit,omitempty"`
	TimedOut        bool     `json:"timed_out,omitempty"`
	DurationSeconds *float64 `json:"duration_seconds,omitempty"`

	// Judge attempts only.
	GradedAgentAttempt   string `json:"graded_agent_attempt,omitempty"`
	GradedEvidenceSHA256 string `json:"graded_evidence_sha256,omitempty"`
	JudgeIdentitySHA256  string `json:"judge_identity_sha256,omitempty"`
	JudgementSHA256      string `json:"judgement_sha256,omitempty"`
}

// CurrentPointer is the only mutable acceptance pointer for a run.
type CurrentPointer struct {
	SchemaVersion             int    `json:"schema_version"`
	AgentAttempt              string `json:"agent_attempt"`
	AgentEvidenceSHA256       string `json:"agent_evidence_sha256"`
	AgentGradingRequestSHA256 string `json:"agent_grading_request_sha256"`
	JudgeAttempt              string `json:"judge_attempt,omitempty"`
	JudgeIdentitySHA256       string `json:"judge_identity_sha256,omitempty"`

	GradedAgentAttempt   string `json:"graded_agent_attempt,omitempty"`
	GradedEvidenceSHA256 string `json:"graded_evidence_sha256,omitempty"`
	GradingRequestSHA256 string `json:"grading_request_sha256,omitempty"`

	AcceptedAt string `json:"accepted_at"`
	AcceptedBy string `json:"accepted_by"`
}
