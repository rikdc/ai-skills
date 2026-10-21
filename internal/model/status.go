package model

// RunStatus is the outcome of one execution attempt.
type RunStatus string

const (
	StatusOK             RunStatus = "ok"
	StatusSetupFailed    RunStatus = "setup-failed"
	StatusAgentFailed    RunStatus = "agent-failed"
	StatusTimedOut       RunStatus = "timed-out"
	StatusEvidenceFailed RunStatus = "evidence-failed"
	StatusChecksFailed   RunStatus = "checks-failed"
	StatusCancelled      RunStatus = "cancelled"
	StatusIncomplete     RunStatus = "incomplete"
	// StatusInterrupted is derived from a started attempt without a finished
	// record. It is never written into finished.json.
	StatusInterrupted RunStatus = "interrupted"
	// StatusMissing is report-only for a run with no accepted attempt.
	StatusMissing RunStatus = "missing"
)

// Valid reports whether s is a known v3 run status.
func (s RunStatus) Valid() bool {
	switch s {
	case StatusOK, StatusSetupFailed, StatusAgentFailed, StatusTimedOut,
		StatusEvidenceFailed, StatusChecksFailed, StatusCancelled,
		StatusIncomplete, StatusInterrupted, StatusMissing:
		return true
	default:
		return false
	}
}

// Stop reasons recorded in finished attempts and result projections.
const (
	StopCompleted         = "completed"
	StopSetupError        = "setup_error"
	StopProviderError     = "provider_error"
	StopMaxOutputTokens   = "max_output_tokens"
	StopContentFilter     = "content_filter"
	StopRefused           = "refused"
	StopEmptyResponse     = "empty_response"
	StopProtocolError     = "protocol_error"
	StopToolProtocolError = "tool_protocol_error"
	StopMaxSteps          = "max_steps"
	StopMaxTotalTokens    = "max_total_tokens"
	StopContextExhausted  = "context_exhausted"
	StopTimeout           = "timeout"
	StopCancelled         = "cancelled"
	StopEvidenceError     = "evidence_error"
	StopChecksError       = "checks_error"
	StopInterrupted       = "interrupted"
)
