// Package skilluse detects, from an agent log, whether the skill units
// installed for a with-arm run were actually used. Detection is deliberately
// conservative: a log it cannot read as a known agent format yields "unknown",
// never "not-invoked".
package skilluse

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rikdc/ai-skills/internal/model"
)

// DefaultSkillDir is the workspace skills directory used when none is given.
const DefaultSkillDir = ".claude/skills"

// Event types that mark a log line as a recognised agent event stream: the
// opencode JSON format and the Claude Code stream-json format.
var knownTypes = map[string]bool{
	"step_start": true, "step_finish": true, "text": true, "tool_use": true,
	"reasoning": true, "error": true,
	"system": true, "assistant": true, "user": true, "result": true, "stream_event": true,
}

// SkillNames returns the deduplicated skill names declared by units such as
// "skills/nix-expert". Command and hook units are not skills.
func SkillNames(units []string) []string {
	var names []string
	seen := map[string]bool{}
	for _, unit := range units {
		kind, name, ok := strings.Cut(unit, "/")
		if !ok || kind != "skills" || name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

// Detect scans an agent log for use of each named skill. A skill counts as
// used when a tool call invokes it through the skill tool, or when a tool call
// touches a path under <skillDir>/<name>/. Prose in assistant text never
// counts. An empty skill list yields the zero SkillUse.
func Detect(log io.Reader, skills []string, skillDir string) model.SkillUse {
	if len(skills) == 0 {
		return model.SkillUse{}
	}
	dir := strings.Trim(strings.ReplaceAll(skillDir, "\\", "/"), "/")
	if dir == "" {
		dir = DefaultSkillDir
	}
	used := map[string]bool{}
	recognised := false

	reader := bufio.NewReaderSize(log, 64<<10)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if scanLine(line, skills, dir, used) {
				recognised = true
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				// A read failure leaves the log partially seen: only a
				// positive finding is trustworthy.
				recognised = false
			}
			break
		}
	}

	result := model.SkillUse{Skills: make(map[string]string, len(skills))}
	invoked, notInvoked := 0, 0
	for _, name := range skills {
		switch {
		case used[name]:
			result.Skills[name] = model.SkillInvoked
			invoked++
		case recognised:
			result.Skills[name] = model.SkillNotInvoked
			notInvoked++
		default:
			result.Skills[name] = model.SkillUnknown
		}
	}
	switch {
	case invoked > 0:
		result.State = model.SkillInvoked
	case notInvoked == len(result.Skills):
		result.State = model.SkillNotInvoked
	default:
		result.State = model.SkillUnknown
	}
	return result
}

// DetectPath detects skill use in the agent log at path. A missing or
// unreadable log is unknown, because a pruned log proves nothing.
func DetectPath(path string, skills []string, skillDir string) model.SkillUse {
	if len(skills) == 0 {
		return model.SkillUse{}
	}
	file, err := os.Open(path)
	if err != nil {
		return Detect(strings.NewReader(""), skills, skillDir)
	}
	defer file.Close()
	return Detect(file, skills, skillDir)
}

// scanLine inspects one log line, records skills it uses, and reports whether
// the line is a recognised agent event.
func scanLine(line []byte, skills []string, dir string, used map[string]bool) bool {
	line = []byte(strings.TrimSpace(string(line)))
	if len(line) == 0 || line[0] != '{' {
		return false
	}
	var event map[string]any
	if err := json.Unmarshal(line, &event); err != nil {
		return false
	}
	eventType, _ := event["type"].(string)
	if !knownTypes[eventType] {
		return false
	}
	switch eventType {
	case "tool_use":
		// opencode: {"type":"tool_use","part":{"tool":"skill","state":{"input":{...}}}}
		part, _ := event["part"].(map[string]any)
		state, _ := part["state"].(map[string]any)
		tool, _ := part["tool"].(string)
		markCall(tool, state["input"], skills, dir, used)
	case "assistant":
		// Claude Code stream-json: message.content[] tool_use blocks.
		message, _ := event["message"].(map[string]any)
		content, _ := message["content"].([]any)
		for _, item := range content {
			block, _ := item.(map[string]any)
			if kind, _ := block["type"].(string); kind != "tool_use" {
				continue
			}
			tool, _ := block["name"].(string)
			markCall(tool, block["input"], skills, dir, used)
		}
	}
	return true
}

func markCall(tool string, input any, skills []string, dir string, used map[string]bool) {
	if strings.EqualFold(tool, "skill") {
		if fields, ok := input.(map[string]any); ok {
			for _, key := range []string{"name", "skill"} {
				value, _ := fields[key].(string)
				if value == "" {
					continue
				}
				// Plugin-qualified references look like "plugin:skill".
				if i := strings.LastIndex(value, ":"); i >= 0 {
					value = value[i+1:]
				}
				for _, name := range skills {
					if value == name {
						used[name] = true
					}
				}
			}
		}
	}
	for _, text := range stringsIn(input) {
		text = strings.ReplaceAll(text, "\\", "/")
		for _, name := range skills {
			prefix := dir + "/" + name
			if i := strings.Index(text, prefix); i >= 0 {
				rest := text[i+len(prefix):]
				if rest == "" || rest[0] == '/' || rest[0] == ' ' || rest[0] == '"' || rest[0] == '\'' {
					used[name] = true
				}
			}
		}
	}
}

// stringsIn returns every string leaf of a decoded JSON value in a stable
// order.
func stringsIn(value any) []string {
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch typed := v.(type) {
		case string:
			out = append(out, typed)
		case []any:
			for _, item := range typed {
				walk(item)
			}
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				walk(typed[key])
			}
		}
	}
	walk(value)
	return out
}

// Installed returns the skills a with-arm run has installed: the skill units
// among units, or every skill the plugin ships when no units were selected
// (the workspace installs them all in that case).
func Installed(pluginDir string, units []string) []string {
	if len(units) > 0 {
		return SkillNames(units)
	}
	entries, err := os.ReadDir(filepath.Join(pluginDir, "skills"))
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names
}
