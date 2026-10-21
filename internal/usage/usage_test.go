package usage

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rikdc/ai-skills/internal/model"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func int64Ptr(v int64) *int64       { return &v }
func float64Ptr(v float64) *float64 { return &v }

func requireInt(t *testing.T, label string, got *int64, want int64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: got nil, want %d", label, want)
	}
	if *got != want {
		t.Fatalf("%s: got %d, want %d", label, *got, want)
	}
}

func TestOpencodeSumsStepFinish(t *testing.T) {
	data := `not json
{"type":"step_start","part":{}}
{"type":"step_finish","part":{"tokens":{"total":11182,"input":11168,"output":3,"reasoning":11,"cache":{"write":0,"read":0}},"cost":0.0016822}}
{"type":"step_finish","part":{"tokens":{"total":100,"input":90,"output":10,"reasoning":0,"cache":{"write":5,"read":7}},"cost":0.0001}}
`
	u, ok := ParseOpencode([]byte(data))
	if !ok {
		t.Fatal("expected known usage")
	}
	if u.State != model.UsageKnown || !u.Known || u.Source != SourceOpencode {
		t.Fatalf("state/source: %+v", u)
	}
	requireInt(t, "input", u.InputTokens, 11258)
	requireInt(t, "output", u.OutputTokens, 13)
	requireInt(t, "reasoning", u.ReasoningTokens, 11)
	requireInt(t, "cache_write", u.CacheWriteTokens, 5)
	requireInt(t, "cache_read", u.CacheReadTokens, 7)
	requireInt(t, "total", u.TotalTokens, 11282)
	if u.CostUSD == nil || *u.CostUSD != 0.0017822 {
		t.Fatalf("cost: %v", u.CostUSD)
	}
	if u.Currency != "USD" {
		t.Fatalf("currency: %q", u.Currency)
	}
}

func TestClaudeLastResult(t *testing.T) {
	data := `{"type":"system","subtype":"init"}
{"type":"result","total_cost_usd":0.1773532,"usage":{"input_tokens":2,"cache_creation_input_tokens":21946,"cache_read_input_tokens":8486,"output_tokens":4,"output_tokens_details":{"thinking_tokens":3}}}
`
	u, ok := ParseClaude([]byte(data))
	if !ok {
		t.Fatal("expected known usage")
	}
	if u.Source != SourceClaude || !u.Known {
		t.Fatalf("source: %+v", u)
	}
	requireInt(t, "input", u.InputTokens, 2)
	requireInt(t, "output", u.OutputTokens, 4)
	requireInt(t, "reasoning", u.ReasoningTokens, 3)
	requireInt(t, "cache_write", u.CacheWriteTokens, 21946)
	requireInt(t, "cache_read", u.CacheReadTokens, 8486)
	requireInt(t, "total", u.TotalTokens, 30438)
	if u.CostUSD == nil || *u.CostUSD != 0.1773532 {
		t.Fatalf("cost: %v", u.CostUSD)
	}
}

func TestCostOnlyUsageIsKnown(t *testing.T) {
	u, ok := ParseClaude([]byte(`{"type":"result","total_cost_usd":0.25}`))
	if !ok || !u.Known {
		t.Fatalf("usage: %+v", u)
	}
	if u.CostUSD == nil || *u.CostUSD != 0.25 || u.Currency != "USD" {
		t.Fatalf("cost: %+v", u)
	}
	if u.TotalTokens != nil {
		t.Fatalf("invented total: %v", *u.TotalTokens)
	}
}

func TestUnknownIsNeverZero(t *testing.T) {
	path := writeTemp(t, "none.log", "nothing useful\n")
	u := Parse(Options{LogPath: path})
	if u.State != model.UsageUnknown || u.Known {
		t.Fatalf("state: %+v", u)
	}
	if u.Source != SourceNone {
		t.Fatalf("source: %q", u.Source)
	}
	if u.InputTokens != nil || u.TotalTokens != nil || u.CostUSD != nil {
		t.Fatalf("invented numerics: %+v", u)
	}
}

func TestMissingLogIsUnknown(t *testing.T) {
	u := Parse(Options{LogPath: filepath.Join(t.TempDir(), "missing.log")})
	if u.State != model.UsageUnknown || u.Source != SourceNone {
		t.Fatalf("state: %+v", u)
	}
}

func TestPartialTokenUsage(t *testing.T) {
	path := writeTemp(t, "partial.log", `{"type":"step_finish","part":{"tokens":{"input":5}}}`+"\n")
	u := Parse(Options{LogPath: path})
	if !u.Known {
		t.Fatalf("expected known: %+v", u)
	}
	requireInt(t, "input", u.InputTokens, 5)
	requireInt(t, "total", u.TotalTokens, 5)
	if u.CostUSD != nil {
		t.Fatalf("cost should be absent: %v", u.CostUSD)
	}
}

func TestTokenOnlyUsageIncludesReasoning(t *testing.T) {
	path := writeTemp(t, "tokens.log", `{"type":"step_finish","part":{"tokens":{"input":5,"reasoning":3}}}`+"\n")
	u := Parse(Options{LogPath: path})
	requireInt(t, "input", u.InputTokens, 5)
	requireInt(t, "reasoning", u.ReasoningTokens, 3)
	requireInt(t, "total", u.TotalTokens, 8)
}

func TestInvalidNumbersIgnored(t *testing.T) {
	path := writeTemp(t, "invalid.log", `{"type":"step_finish","part":{"tokens":{"input":-3,"output":"x","total":10},"cost":-1}}`+"\n")
	u := Parse(Options{LogPath: path})
	if !u.Known {
		t.Fatalf("expected known: %+v", u)
	}
	requireInt(t, "total", u.TotalTokens, 10)
	if u.InputTokens != nil || u.OutputTokens != nil || u.CostUSD != nil {
		t.Fatalf("invented values: %+v", u)
	}
}

func hookScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hook.sh")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env bash\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHookWritesUsageFile(t *testing.T) {
	hook := hookScript(t, `printf '%s\n' '{"known":true,"source":"hook-file","total_tokens":42,"cost_usd":0.5}' >"$USAGE_FILE"`)
	logPath := writeTemp(t, "opencode.log", `{"type":"step_finish","part":{"tokens":{"input":1}}}`+"\n")
	u := Parse(Options{LogPath: logPath, UsageCmd: hook})
	if !u.Known || u.Source != "hook-file" {
		t.Fatalf("usage: %+v", u)
	}
	requireInt(t, "total", u.TotalTokens, 42)
	if u.CostUSD == nil || *u.CostUSD != 0.5 {
		t.Fatalf("cost: %v", u.CostUSD)
	}
}

func TestHookPrintsJSON(t *testing.T) {
	hook := hookScript(t, `printf '%s\n' '{"known":true,"source":"hook-stdout","input_tokens":7}'`)
	logPath := writeTemp(t, "opencode.log", `{"type":"step_finish","part":{"tokens":{"input":1}}}`+"\n")
	u := Parse(Options{LogPath: logPath, UsageCmd: hook})
	if u.Source != "hook-stdout" {
		t.Fatalf("source: %q", u.Source)
	}
	requireInt(t, "input", u.InputTokens, 7)
	requireInt(t, "total", u.TotalTokens, 7)
}

func TestFailingHookFallsBack(t *testing.T) {
	hook := hookScript(t, `exit 3`)
	logPath := writeTemp(t, "opencode.log", `{"type":"step_finish","part":{"tokens":{"input":1}}}`+"\n")
	var stderr bytes.Buffer
	u := Parse(Options{LogPath: logPath, UsageCmd: hook, Stderr: &stderr})
	if u.Source != SourceOpencode || !u.Known {
		t.Fatalf("source: %+v", u)
	}
	if !strings.Contains(stderr.String(), "usage command failed") {
		t.Fatalf("missing warning: %q", stderr.String())
	}
}

func TestHookStderrReachesCaller(t *testing.T) {
	hook := hookScript(t, `echo "hook-diagnostic-9f3d: cannot read vendor usage" >&2; exit 2`)
	logPath := writeTemp(t, "opencode.log", `{"type":"step_finish","part":{"tokens":{"input":1}}}`+"\n")
	var stderr bytes.Buffer
	u := Parse(Options{LogPath: logPath, UsageCmd: hook, Stderr: &stderr})
	if u.Source != SourceOpencode {
		t.Fatalf("source: %+v", u)
	}
	if !strings.Contains(stderr.String(), "hook-diagnostic-9f3d") {
		t.Fatalf("stderr was discarded: %q", stderr.String())
	}
}

func TestInvalidHookFallsBack(t *testing.T) {
	hook := hookScript(t, `printf 'not json\n'`)
	logPath := writeTemp(t, "claude.log", `{"type":"result","usage":{"input_tokens":2,"output_tokens":4}}`+"\n")
	var stderr bytes.Buffer
	u := Parse(Options{LogPath: logPath, UsageCmd: hook, Stderr: &stderr})
	if u.Source != SourceClaude {
		t.Fatalf("source: %+v", u)
	}
	if !strings.Contains(stderr.String(), "no valid JSON") {
		t.Fatalf("missing warning: %q", stderr.String())
	}
}

func TestHookEnv(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "capture.txt")
	hook := hookScript(t, `printf '%s|%s|%s|%s|%s|%s|%s' "$USAGE_FILE" "$AGENT_LOG" "$RUN_DIR" "$SANDBOX" "$ARM" "$RUN" "$AGENT_EXIT" >"$CAPTURE"
printf '%s\n' '{"known":true,"total_tokens":1}' >"$USAGE_FILE"`)
	t.Setenv("CAPTURE", capture)
	logPath := writeTemp(t, "agent.log", "")
	runDir := t.TempDir()
	u := Parse(Options{
		LogPath: logPath, UsageCmd: hook, RunDir: runDir, Sandbox: runDir,
		Arm: "with", Run: "2", AgentExit: "0", Kind: "agent",
	})
	if !u.Known {
		t.Fatalf("usage: %+v", u)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(data), "|")
	if len(parts) != 7 {
		t.Fatalf("capture: %q", data)
	}
	if !strings.HasSuffix(parts[1], "agent.log") {
		t.Fatalf("AGENT_LOG: %q", parts[1])
	}
	if parts[2] != runDir || parts[3] != runDir {
		t.Fatalf("RUN_DIR/SANDBOX: %q/%q", parts[2], parts[3])
	}
	if parts[4] != "with" || parts[5] != "2" || parts[6] != "0" {
		t.Fatalf("ARM/RUN/AGENT_EXIT: %q/%q/%q", parts[4], parts[5], parts[6])
	}
	if parts[0] == "" {
		t.Fatal("USAGE_FILE was empty")
	}
}

func TestCombine(t *testing.T) {
	na := model.NotApplicable("none")
	known := model.KnownUsage("agent")
	known.InputTokens = int64Ptr(10)
	known.OutputTokens = int64Ptr(4)
	known.TotalTokens = int64Ptr(14)
	known.CostUSD = float64Ptr(0.25)
	unknown := model.Unknown("judge", "no usage")
	unknown.InputTokens = int64Ptr(3)
	costOnly := model.KnownUsage("hook")
	costOnly.CostUSD = float64Ptr(0.5)

	cases := []struct {
		name       string
		a, b       model.Usage
		wantState  model.UsageState
		wantInput  *int64
		wantOutput *int64
		wantCost   *float64
	}{
		{"both not applicable", na, na, model.UsageNotApplicable, nil, nil, nil},
		{"one not applicable", known, na, model.UsageKnown, int64Ptr(10), int64Ptr(4), float64Ptr(0.25)},
		{"known sum", known, known, model.UsageKnown, int64Ptr(20), int64Ptr(8), float64Ptr(0.5)},
		{"known plus unknown", known, unknown, model.UsageUnknown, int64Ptr(13), int64Ptr(4), nil},
		{"unknown plus not applicable", unknown, na, model.UsageUnknown, int64Ptr(3), nil, nil},
		{"known with one-sided components", known, costOnly, model.UsageUnknown, int64Ptr(10), int64Ptr(4), float64Ptr(0.75)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := model.Combine(tc.a, tc.b)
			if got.State != tc.wantState || got.Known != (tc.wantState == model.UsageKnown) {
				t.Fatalf("state: %+v", got)
			}
			if !sameInt(got.InputTokens, tc.wantInput) {
				t.Fatalf("input: %v want %v", derefInt(got.InputTokens), derefInt(tc.wantInput))
			}
			if !sameInt(got.OutputTokens, tc.wantOutput) {
				t.Fatalf("output: %v want %v", derefInt(got.OutputTokens), derefInt(tc.wantOutput))
			}
			if !sameFloat(got.CostUSD, tc.wantCost) {
				t.Fatalf("cost: %v want %v", derefFloat(got.CostUSD), derefFloat(tc.wantCost))
			}
		})
	}
}

func sameInt(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func sameFloat(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func derefInt(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func derefFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func TestPricing(t *testing.T) {
	p := Pricing{Models: map[string]ModelPrice{
		"gpt-4.1-mini": {Input: 0.40, Output: 1.60, CacheRead: 0.10, CacheWrite: 0.40},
	}}
	u := model.KnownUsage("provider")
	u.InputTokens = int64Ptr(1_000_000)
	u.OutputTokens = int64Ptr(1_000_000)
	u.CacheReadTokens = int64Ptr(400_000)
	u.CacheWriteTokens = int64Ptr(100_000)

	cost, ok := p.CostFor("gpt-4.1-mini", u)
	if !ok {
		t.Fatal("expected a price")
	}
	// billable input 600k * 0.40 + cache read 400k * 0.10 +
	// cache write 100k * 0.40 + output 1M * 1.60 = 0.24+0.04+0.04+1.60
	want := 1.92
	if diff := cost - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("cost %f, want %f", cost, want)
	}
	applied := p.Apply("gpt-4.1-mini", u)
	if applied.CostUSD == nil || applied.Currency != "USD" {
		t.Fatalf("applied: %+v", applied)
	}

	missing := u
	missing.InputTokens = nil
	if _, ok := p.CostFor("gpt-4.1-mini", missing); ok {
		t.Fatal("missing input must not be priced as zero")
	}
	if _, ok := p.CostFor("other-model", u); ok {
		t.Fatal("unknown model must not be priced")
	}
	unknown := model.Unknown("provider", "")
	if _, ok := p.CostFor("gpt-4.1-mini", unknown); ok {
		t.Fatal("unknown usage must not be priced")
	}
}

func TestPricingDisjointInputSources(t *testing.T) {
	p := Pricing{Models: map[string]ModelPrice{
		"m": {Input: 3, Output: 15, CacheRead: 0.30, CacheWrite: 3.75},
	}}
	for _, source := range []string{SourceClaude, SourceOpencode} {
		u := model.KnownUsage(source)
		u.InputTokens = int64Ptr(1_200)
		u.OutputTokens = int64Ptr(1_000)
		u.CacheReadTokens = int64Ptr(50_000)
		cost, ok := p.CostFor("m", u)
		if !ok {
			t.Fatalf("%s: expected a price", source)
		}
		// input 1.2k * 3 + cache read 50k * 0.30 + output 1k * 15, per million.
		want := 0.0036 + 0.015 + 0.015
		if diff := cost - want; diff > 1e-12 || diff < -1e-12 {
			t.Fatalf("%s: cost %f, want %f", source, cost, want)
		}
	}
}

func TestLoadPricingRejectsNegative(t *testing.T) {
	path := writeTemp(t, "pricing.json", `{"models":{"m":{"input":-1,"output":1}}}`)
	if _, err := LoadPricing(path); err == nil {
		t.Fatal("expected an error")
	}
}
