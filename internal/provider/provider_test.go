package provider

import (
	"encoding/json"
	"testing"
)

func TestAPIErrorClassification(t *testing.T) {
	cases := []struct {
		status    int
		terminal  bool
		retryable bool
	}{
		{400, true, false},
		{401, true, false},
		{403, true, false},
		{404, true, false},
		{422, true, false},
		{408, false, true},
		{409, false, true},
		{429, false, true},
		{500, false, true},
		{502, false, true},
		{503, false, true},
		{504, false, true},
		{0, false, true},
	}
	for _, tc := range cases {
		e := &APIError{StatusCode: tc.status, Message: "boom"}
		if e.Terminal() != tc.terminal {
			t.Fatalf("status %d terminal = %v", tc.status, e.Terminal())
		}
		if e.Retryable() != tc.retryable {
			t.Fatalf("status %d retryable = %v", tc.status, e.Retryable())
		}
		if (tc.status == 401 || tc.status == 403) != e.IsAuthError() {
			t.Fatalf("status %d auth hint", tc.status)
		}
	}
}

func TestChatRequestJSONShape(t *testing.T) {
	req := ChatRequest{
		Model:             "m",
		Messages:          []Message{{Role: RoleUser, Content: "hi"}},
		Tools:             []Tool{{Type: "function", Function: ToolFunction{Name: "bash", Parameters: json.RawMessage(`{"type":"object"}`)}}},
		ToolChoice:        "auto",
		MaxOutputTokens:   100,
		ParallelToolCalls: new(bool),
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("empty request")
	}
}
