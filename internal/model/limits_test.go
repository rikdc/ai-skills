package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLimitsDefaults(t *testing.T) {
	d := DefaultLimits()
	if d.MaxSteps != DefaultMaxSteps || d.Timeout != DefaultTimeout ||
		d.ContextWindow != DefaultContextWindow || d.KeepRecentGroups != DefaultKeepRecentGroups {
		t.Fatalf("defaults: %+v", d)
	}
	filled := (Limits{MaxSteps: 7}).WithDefaults()
	if filled.MaxSteps != 7 {
		t.Fatalf("explicit value overwritten: %+v", filled)
	}
	if filled.Timeout != DefaultTimeout || filled.MaxOutputTokens != DefaultMaxOutputTokens {
		t.Fatalf("zero values not defaulted: %+v", filled)
	}
	unlimited := (Limits{MaxTotalTokens: -1}).WithDefaults()
	if unlimited.MaxTotalTokens != 0 {
		t.Fatalf("negative total should mean unlimited: %d", unlimited.MaxTotalTokens)
	}
	if (Limits{}).WithDefaults().MaxTotalTokens != 0 {
		t.Fatal("zero MaxTotalTokens must stay unlimited")
	}
	if d.MaxTotalTokens != DefaultMaxTotalTokens {
		t.Fatalf("documented default total: %d", d.MaxTotalTokens)
	}
}

func TestLimitsJSON(t *testing.T) {
	in := Limits{
		MaxSteps:           3,
		MaxTotalTokens:     100,
		MaxOutputTokens:    10,
		Timeout:            2 * time.Minute,
		RequestTimeout:     time.Second,
		MaxPromptBytes:     1024,
		MaxToolOutputBytes: 512,
		ContextWindow:      4096,
		KeepRecentGroups:   2,
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Limits
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("round trip: %+v != %+v", out, in)
	}
}

func TestToolPolicyJSON(t *testing.T) {
	in := ToolPolicy{AllowedTools: []string{"bash"}, AllowBash: true, PassEnv: []string{"LANG"}}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out ToolPolicy
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if !out.AllowBash || len(out.AllowedTools) != 1 || out.AllowedTools[0] != "bash" {
		t.Fatalf("round trip: %+v", out)
	}
}
