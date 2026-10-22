// Package usage normalizes token and cost usage from command-agent logs. It
// never invents zero: an invocation without usable usage is unknown.
package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/platform/execx"
)

// Sources reported by the built-in parsers.
const (
	SourceOpencode = "opencode-json"
	SourceClaude   = "claude-json"
	SourceHook     = "hook"
	SourceNone     = "none"
)

// DefaultHookTimeout bounds a usage hook.
const DefaultHookTimeout = 30 * time.Second

// Options configures one usage normalization.
type Options struct {
	LogPath  string
	UsageCmd string
	// Hook context exposed to the usage hook.
	RunDir    string
	Sandbox   string
	Arm       string
	Run       string
	AgentExit string
	Kind      string
	// Stderr receives hook diagnostics. Nil discards them.
	Stderr io.Writer
	// HookTimeout bounds the hook process.
	HookTimeout time.Duration
}

// Parse returns the normalized usage for one invocation. A missing log is
// unknown, not an error. A valid hook result overrides the built-in parsers;
// a failing or invalid hook is reported and ignored.
func Parse(opts Options) model.Usage {
	if opts.UsageCmd != "" {
		if u, ok := runHook(opts); ok {
			return u
		}
	}
	data, err := os.ReadFile(opts.LogPath)
	if err != nil {
		return model.Unknown(SourceNone, "log unavailable")
	}
	if u, ok := ParseOpencode(data); ok {
		return u
	}
	if u, ok := ParseClaude(data); ok {
		return u
	}
	return model.Unknown(SourceNone, "no recognized usage record")
}

// ParseOpencode sums every step_finish event's tokens and cost. Invalid or
// negative values are ignored; when no total is present the components are
// summed (including reasoning, which opencode reports separately).
func ParseOpencode(data []byte) (model.Usage, bool) {
	var (
		input, output, reasoning, cacheRead, cacheWrite, total *int64
		cost                                                   *float64
		count                                                  int
	)
	for _, line := range bytes.Split(data, []byte("\n")) {
		event := decodeObject(line)
		if event == nil {
			continue
		}
		if typ, _ := event["type"].(string); typ != "step_finish" {
			continue
		}
		part, ok := event["part"].(map[string]any)
		if !ok {
			continue
		}
		count++
		tokens, _ := part["tokens"].(map[string]any)
		input = addInt(input, intAt(tokens, "input"))
		output = addInt(output, intAt(tokens, "output"))
		reasoning = addInt(reasoning, intAt(tokens, "reasoning"))
		cache, _ := tokens["cache"].(map[string]any)
		cacheRead = addInt(cacheRead, intAt(cache, "read"))
		cacheWrite = addInt(cacheWrite, intAt(cache, "write"))
		total = addInt(total, intAt(tokens, "total"))
		cost = addFloat(cost, floatAt(part, "cost"))
	}
	if count == 0 {
		return model.Usage{}, false
	}
	u := build(SourceOpencode, fieldSet{
		input: input, output: output, reasoning: reasoning,
		cacheRead: cacheRead, cacheWrite: cacheWrite, total: total, cost: cost,
	})
	return u, u.State == model.UsageKnown
}

// ParseClaude reads the last type:"result" object. Thinking tokens are a
// subset of output, so they never add to a computed total.
func ParseClaude(data []byte) (model.Usage, bool) {
	var result map[string]any
	found := false
	for _, line := range bytes.Split(data, []byte("\n")) {
		event := decodeObject(line)
		if event == nil {
			continue
		}
		if typ, _ := event["type"].(string); typ == "result" {
			result = event
			found = true
		}
	}
	if !found {
		return model.Usage{}, false
	}
	usageMap, _ := result["usage"].(map[string]any)
	var reasoning *int64
	if details, ok := usageMap["output_tokens_details"].(map[string]any); ok {
		reasoning = intAt(details, "thinking_tokens")
	}
	if reasoning == nil {
		if details, ok := result["output_tokens_details"].(map[string]any); ok {
			reasoning = intAt(details, "thinking_tokens")
		}
	}
	f := fieldSet{
		input:      intAt(usageMap, "input_tokens"),
		output:     intAt(usageMap, "output_tokens"),
		reasoning:  reasoning,
		cacheRead:  intAt(usageMap, "cache_read_input_tokens"),
		cacheWrite: intAt(usageMap, "cache_creation_input_tokens"),
		total:      intAt(usageMap, "total_tokens"),
		cost:       floatAt(result, "total_cost_usd"),
	}
	u := build(SourceClaude, f)
	return u, u.State == model.UsageKnown
}

// decodeObject parses one JSON object line, returning nil for invalid input.
func decodeObject(line []byte) map[string]any {
	if len(bytes.TrimSpace(line)) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil
	}
	return obj
}

type fieldSet struct {
	input, output, reasoning, cacheRead, cacheWrite, total *int64
	cost                                                   *float64
}

// build applies the normalization rules: missing values stay omitted,
// a total is derived only when absent, and an object with no usable numbers is
// unknown source "none".
func build(source string, f fieldSet) model.Usage {
	if f.input == nil && f.output == nil && f.reasoning == nil &&
		f.cacheRead == nil && f.cacheWrite == nil && f.total == nil && f.cost == nil {
		return model.Unknown(SourceNone, "")
	}
	u := model.Usage{
		State:            model.UsageKnown,
		Known:            true,
		Source:           source,
		InputTokens:      f.input,
		OutputTokens:     f.output,
		ReasoningTokens:  f.reasoning,
		CacheReadTokens:  f.cacheRead,
		CacheWriteTokens: f.cacheWrite,
		TotalTokens:      f.total,
		CostUSD:          f.cost,
	}
	if u.TotalTokens == nil {
		var sum int64
		present := false
		add := func(v *int64) {
			if v != nil {
				sum += *v
				present = true
			}
		}
		add(f.input)
		add(f.output)
		add(f.cacheRead)
		add(f.cacheWrite)
		if source != SourceClaude {
			add(f.reasoning)
		}
		if present {
			u.TotalTokens = &sum
		}
	}
	if u.CostUSD != nil {
		u.Currency = "USD"
	}
	return u
}

// ParseHookJSON parses a normalized usage object, accepting the legacy
// known/source shape as well as the v3 state field. It returns false when the
// object has no usable numeric fields and no boolean known flag.
func ParseHookJSON(data []byte) (model.Usage, bool) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return model.Usage{}, false
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return model.Usage{}, false
	}

	source := SourceHook
	if s, ok := raw["source"].(string); ok && s != "" {
		source = s
	}
	f := fieldSet{
		input:      intAt(raw, "input_tokens"),
		output:     intAt(raw, "output_tokens"),
		reasoning:  intAt(raw, "reasoning_tokens"),
		cacheRead:  intAt(raw, "cache_read_tokens"),
		cacheWrite: intAt(raw, "cache_write_tokens"),
		total:      intAt(raw, "total_tokens"),
		cost:       floatAt(raw, "cost_usd"),
	}
	hasNumber := f.input != nil || f.output != nil || f.reasoning != nil ||
		f.cacheRead != nil || f.cacheWrite != nil || f.total != nil || f.cost != nil
	knownFlag, hasKnown := raw["known"].(bool)
	if !hasNumber {
		if !hasKnown {
			return model.Usage{}, false
		}
		_ = knownFlag
		return model.Unknown(SourceNone, ""), true
	}
	return build(source, f), true
}

func runHook(opts Options) (model.Usage, bool) {
	stderr := opts.Stderr
	if stderr == nil {
		stderr = io.Discard
	}

	hookFile, err := os.CreateTemp("", "evalctl-usage-hook-*")
	if err != nil {
		fmt.Fprintln(stderr, "evalctl usage: cannot create a hook output file; falling back to built-in extraction")
		return model.Usage{}, false
	}
	name := hookFile.Name()
	_ = hookFile.Close()
	defer os.Remove(name)

	cwd := "."
	if opts.Sandbox != "" {
		if fi, err := os.Stat(opts.Sandbox); err == nil && fi.IsDir() {
			cwd = opts.Sandbox
		}
	} else if opts.RunDir != "" {
		if fi, err := os.Stat(opts.RunDir); err == nil && fi.IsDir() {
			cwd = opts.RunDir
		}
	}

	env := withEnv(os.Environ(), map[string]string{
		"USAGE_FILE": name,
		"AGENT_LOG":  opts.LogPath,
		"RUN_DIR":    opts.RunDir,
		"SANDBOX":    opts.Sandbox,
		"ARM":        opts.Arm,
		"RUN":        opts.Run,
		"AGENT_EXIT": opts.AgentExit,
		"KIND":       opts.Kind,
	})

	timeout := opts.HookTimeout
	if timeout <= 0 {
		timeout = DefaultHookTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	res, runErr := execx.Run(ctx, execx.Spec{
		Path:   "/bin/bash",
		Args:   []string{"-c", opts.UsageCmd},
		Dir:    cwd,
		Env:    env,
		Stdout: nil,
		Stderr: stderr,
	})
	if runErr != nil || res.ExitCode != 0 {
		code := res.ExitCode
		if runErr != nil && code <= 0 {
			code = 1
		}
		fmt.Fprintf(stderr, "evalctl usage: usage command failed (exit %d); falling back to built-in extraction\n", code)
		return model.Usage{}, false
	}

	candidate := ""
	if data, err := os.ReadFile(name); err == nil && len(bytes.TrimSpace(data)) > 0 {
		candidate = strings.TrimSpace(string(data))
	}
	if candidate == "" {
		candidate = strings.TrimSpace(string(res.Stdout))
	}
	if candidate == "" {
		fmt.Fprintln(stderr, "evalctl usage: usage command produced no valid JSON; falling back to built-in extraction")
		return model.Usage{}, false
	}
	u, ok := ParseHookJSON([]byte(candidate))
	if !ok {
		fmt.Fprintln(stderr, "evalctl usage: usage command produced no valid JSON; falling back to built-in extraction")
		return model.Usage{}, false
	}
	return u, true
}

// withEnv replaces keys in base with overrides.
func withEnv(base []string, overrides map[string]string) []string {
	out := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key := entry
		if i := strings.IndexByte(entry, '='); i >= 0 {
			key = entry[:i]
		}
		if _, override := overrides[key]; override {
			continue
		}
		out = append(out, entry)
	}
	for key, value := range overrides {
		out = append(out, key+"="+value)
	}
	return out
}

func intAt(m map[string]any, key string) *int64 {
	if m == nil {
		return nil
	}
	n, ok := m[key].(json.Number)
	if !ok {
		return nil
	}
	f, err := n.Float64()
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f != math.Trunc(f) {
		return nil
	}
	if f > math.MaxInt64 {
		return nil
	}
	v := int64(f)
	return &v
}

func floatAt(m map[string]any, key string) *float64 {
	if m == nil {
		return nil
	}
	n, ok := m[key].(json.Number)
	if !ok {
		return nil
	}
	f, err := n.Float64()
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return nil
	}
	return &f
}

func addInt(dst, v *int64) *int64 {
	if v == nil {
		return dst
	}
	if dst == nil {
		n := *v
		return &n
	}
	*dst += *v
	return dst
}

func addFloat(dst, v *float64) *float64 {
	if v == nil {
		return dst
	}
	if dst == nil {
		n := *v
		return &n
	}
	*dst += *v
	return dst
}

// LogPath returns the conventional log path for an attempt directory.
func LogPath(dir, kind string) string {
	name := "agent.log"
	if kind == string(model.KindJudge) || kind == "judge" {
		name = "judge.log"
	}
	return filepath.Join(dir, name)
}
