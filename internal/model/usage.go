package model

// UsageState is the three-state accounting contract. Unknown is never zero.
type UsageState string

const (
	UsageNotApplicable UsageState = "not_applicable"
	UsageUnknown       UsageState = "unknown"
	UsageKnown         UsageState = "known"
)

// Usage records token and cost usage for one invocation. Missing numeric
// components are omitted, never written as zero.
type Usage struct {
	State            UsageState `json:"state"`
	Known            bool       `json:"known"`
	Source           string     `json:"source"`
	Reason           string     `json:"reason,omitempty"`
	InputTokens      *int64     `json:"input_tokens,omitempty"`
	OutputTokens     *int64     `json:"output_tokens,omitempty"`
	ReasoningTokens  *int64     `json:"reasoning_tokens,omitempty"`
	CacheReadTokens  *int64     `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens *int64     `json:"cache_write_tokens,omitempty"`
	TotalTokens      *int64     `json:"total_tokens,omitempty"`
	CostUSD          *float64   `json:"cost_usd,omitempty"`
	Currency         string     `json:"currency,omitempty"`
}
