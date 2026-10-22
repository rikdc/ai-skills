package model

import "time"

// Limits bounds one agent execution. Zero values mean "use the default" except
// MaxTotalTokens, for which zero means unlimited as specified in §4.2. Start
// from DefaultLimits for the documented defaults and override fields.
type Limits struct {
	MaxSteps           int           `json:"max_steps"`
	MaxTotalTokens     int64         `json:"max_total_tokens"`
	MaxOutputTokens    int           `json:"max_output_tokens"`
	Timeout            time.Duration `json:"timeout"`
	RequestTimeout     time.Duration `json:"request_timeout"`
	MaxPromptBytes     int           `json:"max_prompt_bytes"`
	MaxToolOutputBytes int           `json:"max_tool_output_bytes"`
	ContextWindow      int           `json:"context_window"`
	KeepRecentGroups   int           `json:"keep_recent_groups"`
}

// Default limits from the v3 specification.
const (
	DefaultMaxSteps           = 40
	DefaultMaxTotalTokens     = int64(500_000)
	DefaultMaxOutputTokens    = 4096
	DefaultTimeout            = 15 * time.Minute
	DefaultRequestTimeout     = 120 * time.Second
	DefaultMaxPromptBytes     = 256 * 1024
	DefaultMaxToolOutputBytes = 64 * 1024
	DefaultContextWindow      = 128_000
	DefaultKeepRecentGroups   = 3
)

// DefaultLimits returns the documented v3 defaults.
func DefaultLimits() Limits {
	return Limits{
		MaxSteps:           DefaultMaxSteps,
		MaxTotalTokens:     DefaultMaxTotalTokens,
		MaxOutputTokens:    DefaultMaxOutputTokens,
		Timeout:            DefaultTimeout,
		RequestTimeout:     DefaultRequestTimeout,
		MaxPromptBytes:     DefaultMaxPromptBytes,
		MaxToolOutputBytes: DefaultMaxToolOutputBytes,
		ContextWindow:      DefaultContextWindow,
		KeepRecentGroups:   DefaultKeepRecentGroups,
	}
}

// WithDefaults fills zero-valued fields with the documented defaults except
// MaxTotalTokens: zero there means unlimited, not "use 500_000". Call
// DefaultLimits when the documented token cap is wanted. A negative
// MaxTotalTokens is normalized to zero (unlimited).
func (l Limits) WithDefaults() Limits {
	d := DefaultLimits()
	if l.MaxSteps == 0 {
		l.MaxSteps = d.MaxSteps
	}
	if l.MaxTotalTokens < 0 {
		l.MaxTotalTokens = 0
	}
	if l.MaxOutputTokens == 0 {
		l.MaxOutputTokens = d.MaxOutputTokens
	}
	if l.Timeout == 0 {
		l.Timeout = d.Timeout
	}
	if l.RequestTimeout == 0 {
		l.RequestTimeout = d.RequestTimeout
	}
	if l.MaxPromptBytes == 0 {
		l.MaxPromptBytes = d.MaxPromptBytes
	}
	if l.MaxToolOutputBytes == 0 {
		l.MaxToolOutputBytes = d.MaxToolOutputBytes
	}
	if l.ContextWindow == 0 {
		l.ContextWindow = d.ContextWindow
	}
	if l.KeepRecentGroups == 0 {
		l.KeepRecentGroups = d.KeepRecentGroups
	}
	return l
}

// ToolPolicy bounds what the native runtime may run in a workspace. Command
// agents are whole-run executors and do not consult it; it exists so the
// native runtime and the runner share one contract.
type ToolPolicy struct {
	// AllowedTools lists tool names the native runtime may offer. An empty list
	// means the runtime's built-in set.
	AllowedTools []string `json:"allowed_tools,omitempty"`
	// AllowBash permits the bash tool. It constrains only the agent runtime;
	// bash still runs with the invoking user's privileges.
	AllowBash bool `json:"allow_bash,omitempty"`
	// PassEnv lists environment variable names that survive sanitization and
	// are exposed to tools.
	PassEnv []string `json:"pass_env,omitempty"`
}
