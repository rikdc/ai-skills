// Package provider defines the model-call boundary used by native execution
// and native judging. Milestone A ships the interface only; the command
// adapter implements executor.AgentExecutor and never this interface.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rikdc/ai-skills/internal/model"
)

// Role is a Chat Completions message role.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is one chat message.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolCall is one assistant tool call.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall carries a raw JSON-object argument string.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool is one offered function.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a tool.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ChatRequest is one non-streaming chat completion request.
type ChatRequest struct {
	Model             string
	Messages          []Message
	Tools             []Tool
	ToolChoice        string
	Temperature       *float64
	MaxOutputTokens   int
	Seed              *int64
	ParallelToolCalls *bool
}

// ChatResponse is one decoded chat completion response.
type ChatResponse struct {
	Message      Message
	FinishReason string
	Refusal      string
	Usage        model.Usage
	Raw          json.RawMessage
}

// ProviderIdentity identifies a native provider for provenance.
type ProviderIdentity struct {
	Kind    string
	Model   string
	BaseURL string
}

// ChatProvider is a model-call boundary. It is used only by native execution
// and native judging.
type ChatProvider interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
	Identity() ProviderIdentity
}

// APIError classifies a provider or transport failure.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	RetryAfter time.Duration
	Raw        []byte
}

// Error implements error.
func (e *APIError) Error() string {
	if e == nil {
		return "provider error"
	}
	msg := e.Message
	if msg == "" {
		msg = "provider request failed"
	}
	if e.StatusCode != 0 {
		return fmt.Sprintf("provider error (status %d): %s", e.StatusCode, msg)
	}
	if e.Code != "" {
		return fmt.Sprintf("provider error (%s): %s", e.Code, msg)
	}
	return "provider error: " + msg
}

// Terminal reports whether retrying the request is pointless. 400, 401, 403,
// 404, and 422 are terminal; 401/403 additionally suggest an env check.
func (e *APIError) Terminal() bool {
	switch e.StatusCode {
	case 400, 401, 403, 404, 422:
		return true
	default:
		return false
	}
}

// Retryable reports whether the error is in the documented retry set.
func (e *APIError) Retryable() bool {
	switch e.StatusCode {
	case 408, 409, 429, 500, 502, 503, 504:
		return true
	case 0:
		// Transport failure before a response.
		return true
	default:
		return false
	}
}

// IsAuthError reports whether the error is an authentication/authorization
// failure, for which callers should add an API-key environment hint.
func (e *APIError) IsAuthError() bool {
	return e.StatusCode == 401 || e.StatusCode == 403
}

// AsAPIError extracts an APIError from err.
func AsAPIError(err error) (*APIError, bool) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr, true
	}
	return nil, false
}
