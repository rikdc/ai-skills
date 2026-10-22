// Package executor defines the whole-run execution boundary between the
// runner/suite and an agent runtime.
//
// The boundary is deliberately not a chat-turn abstraction: a command agent
// receives a task, runs to completion, and returns one result. Native
// runtimes implement the same interface and use a provider.ChatProvider
// internally.
package executor

import (
	"context"
	"time"

	"github.com/rikdc/ai-skills/internal/model"
)

// AgentRunRequest is one whole agent invocation.
type AgentRunRequest struct {
	Task         string // task.md or judge prompt
	Workspace    string // cwd for the agent/command
	HomeDir      string // HOME for the agent/command
	SystemPrompt string // native prompt; command adapters may ignore it
	LogDir       string // attempt directory for logs/transcript
	Limits       model.Limits
	ToolPolicy   model.ToolPolicy

	// Additive fields used by command adapters to place and label evidence.
	// Native runtimes may ignore them.
	LogFile string            // log basename; adapters default to agent.log
	Kind    model.AttemptKind // agent or judge, for log naming
	Arm     string            // with or without
	Run     int               // run number
}

// AgentRunResult is the outcome of one whole invocation.
type AgentRunResult struct {
	Status     model.RunStatus
	StopReason string
	Usage      model.Usage // whole invocation; unknown if unavailable
	Output     string      // final assistant text, if any
	Error      string

	// Additive observability fields. ExitCode is nil when the command never
	// started.
	ExitCode  *int
	TimedOut  bool
	Cancelled bool
	Duration  time.Duration
}

// RuntimeIdentity identifies the executor for provenance.
type RuntimeIdentity struct {
	Kind    string // command|native|hook
	Model   string
	Command string
	BaseURL string
}

// AgentExecutor executes a whole agent or judge invocation.
type AgentExecutor interface {
	Run(ctx context.Context, req AgentRunRequest) (AgentRunResult, error)
	Identity() RuntimeIdentity
}
