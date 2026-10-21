// Package scenario discovers, parses, and validates eval scenarios and the
// selection rules shared by list, plan, and suite.
package scenario

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/rikdc/ai-skills/internal/model"
)

// RequiredFiles are the files every runnable scenario must provide.
var RequiredFiles = []string{"task.md", "criteria.json", "scenario.json", "inputs/setup.sh"}

var unitPattern = regexp.MustCompile(`^(skills|commands|hooks)/[A-Za-z0-9._-]+$`)

// Entry identifies one scenario directory discovered under plugins.
type Entry struct {
	Plugin string
	Name   string
	Dir    string
}

// Pair returns the canonical plugin/scenario identifier.
func (e Entry) Pair() string { return e.Plugin + "/" + e.Name }

// Meta is scenario.json plus the field-presence facts that JSON types alone
// cannot express (an absent include differs from an empty array).
type Meta struct {
	Spec         model.Scenario
	HasInclude   bool
	IncludeArray bool
	HasUnit      bool
	HasUnits     bool
}

// Scenario is a loaded scenario, either checked (Load) or metadata-only
// (LoadMeta).
type Scenario struct {
	Root   string
	Plugin string
	Name   string
	Dir    string
	Meta   Meta
}

// Pair returns the canonical plugin/scenario identifier.
func (s *Scenario) Pair() string { return s.Plugin + "/" + s.Name }

// DeclaredUnits returns the scenario's declared units, deduplicated in
// declaration order.
func (s *Scenario) DeclaredUnits() []string {
	return dedupe(s.Meta.Spec.DeclaredUnits())
}

// Mode returns the effective mode.
func (s *Scenario) Mode() model.Mode { return s.Meta.Spec.ModeOrDefault() }

// Execution returns the effective execution backend.
func (s *Scenario) Execution() model.ExecutionBackend {
	return s.Meta.Spec.ExecutionBackend()
}

// DirectHook reports whether the scenario requests milestone-C direct hook
// execution.
func (s *Scenario) DirectHook() bool {
	return s.Meta.Spec.Execution != nil && s.Meta.Spec.Execution.Backend == model.ExecutionHook
}

// Discover lists scenario directories under root/plugins/*/evals/* in
// deterministic plugin/scenario order.
func Discover(root string) ([]Entry, error) {
	plugins, err := Plugins(root)
	if err != nil {
		return nil, err
	}
	var entries []Entry
	for _, plugin := range plugins {
		evalsDir := filepath.Join(root, "plugins", plugin, "evals")
		children, err := os.ReadDir(evalsDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", evalsDir, err)
		}
		for _, child := range children {
			info, err := os.Stat(filepath.Join(evalsDir, child.Name()))
			if err != nil || !info.IsDir() {
				continue
			}
			entries = append(entries, Entry{
				Plugin: plugin,
				Name:   child.Name(),
				Dir:    filepath.Join(evalsDir, child.Name()),
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Plugin != entries[j].Plugin {
			return entries[i].Plugin < entries[j].Plugin
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

// Plugins lists plugin directory names under root/plugins in lexical order.
func Plugins(root string) ([]string, error) {
	dir := filepath.Join(root, "plugins")
	children, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var names []string
	for _, child := range children {
		if strings.HasPrefix(child.Name(), ".") {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, child.Name()))
		if err != nil || !info.IsDir() {
			continue
		}
		names = append(names, child.Name())
	}
	sort.Strings(names)
	return names, nil
}

// ParseScenario parses scenario.json and records field presence.
func ParseScenario(path string) (Meta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Meta{}, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return Meta{}, fmt.Errorf("invalid JSON: %w", err)
	}
	var spec model.Scenario
	if err := json.Unmarshal(data, &spec); err != nil {
		return Meta{}, fmt.Errorf("invalid JSON: %w", err)
	}
	meta := Meta{Spec: spec}
	if value, ok := raw["include"]; ok && !isJSONNull(value) {
		meta.HasInclude = true
		trimmed := strings.TrimSpace(string(value))
		meta.IncludeArray = strings.HasPrefix(trimmed, "[")
	}
	if value, ok := raw["unit"]; ok && !isJSONNull(value) {
		meta.HasUnit = true
	}
	if value, ok := raw["units"]; ok && !isJSONNull(value) {
		meta.HasUnits = true
	}
	return meta, nil
}

// Load reads and fully validates one scenario.
func Load(root, plugin, name string) (*Scenario, error) {
	s := &Scenario{
		Root:   root,
		Plugin: plugin,
		Name:   name,
		Dir:    filepath.Join(root, "plugins", plugin, "evals", name),
	}
	meta, err := ParseScenario(filepath.Join(s.Dir, "scenario.json"))
	if err != nil {
		return nil, fmt.Errorf("%s/scenario.json: %w", s.Pair(), err)
	}
	s.Meta = meta
	if problems := s.Validate(); len(problems) > 0 {
		return nil, fmt.Errorf("%s: %s", s.Pair(), strings.Join(problems, "; "))
	}
	return s, nil
}

// LoadMeta reads scenario.json without validating fixture files.
func LoadMeta(root, plugin, name string) (*Scenario, error) {
	s := &Scenario{
		Root:   root,
		Plugin: plugin,
		Name:   name,
		Dir:    filepath.Join(root, "plugins", plugin, "evals", name),
	}
	meta, err := ParseScenario(filepath.Join(s.Dir, "scenario.json"))
	if err != nil {
		return nil, fmt.Errorf("%s/scenario.json: %w", s.Pair(), err)
	}
	s.Meta = meta
	return s, nil
}

// ParseCriteria reads and parses criteria.json.
func ParseCriteria(path string) (model.Rubric, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return model.Rubric{}, err
	}
	var rubric model.Rubric
	if err := json.Unmarshal(data, &rubric); err != nil {
		return model.Rubric{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return rubric, nil
}

// Validate returns every runtime validation problem for the scenario.
func (s *Scenario) Validate() []string {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	for _, required := range RequiredFiles {
		if _, err := os.Stat(filepath.Join(s.Dir, required)); err != nil {
			add("missing %s", required)
		}
	}
	if data, err := os.ReadFile(filepath.Join(s.Dir, "task.md")); err == nil && len(strings.TrimSpace(string(data))) == 0 {
		add("task.md is empty")
	}

	spec := s.Meta.Spec
	if strings.TrimSpace(spec.Description) == "" {
		add("description is empty")
	}

	if !s.Meta.HasInclude || !s.Meta.IncludeArray {
		add("'include' must be an array")
	} else {
		for _, include := range spec.Include {
			if include == "" {
				continue
			}
			full := filepath.Join(s.Dir, include)
			if _, err := os.Stat(full); err != nil {
				add("include path does not exist: %s", include)
				continue
			}
			if !insideDir(s.Dir, full) {
				add("include path resolves outside the scenario directory: %s", include)
			}
		}
	}

	if s.Meta.HasUnit && s.Meta.HasUnits {
		add("use either 'unit' or 'units', not both")
	}
	if !s.Meta.HasUnit && !s.Meta.HasUnits {
		add("missing unit metadata (add 'unit' or 'units')")
	}
	seen := map[string]bool{}
	for _, unit := range s.DeclaredUnits() {
		if seen[unit] {
			add("duplicate unit %q", unit)
			continue
		}
		seen[unit] = true
		if !UnitPattern(unit) {
			add("unit %q must use skills/<name>, commands/<name> or hooks/<name>", unit)
			continue
		}
		if !UnitExists(s.Root, s.Plugin, unit) {
			add("unit %q does not exist in plugin %q", unit, s.Plugin)
		}
	}

	mode := spec.ModeOrDefault()
	if !mode.Valid() {
		add("invalid mode %q", mode)
	}
	declared := s.DeclaredUnits()
	if mode == model.ModeFunctional && !hasHookUnit(declared) {
		add("functional mode requires a hook unit")
	}

	if spec.Execution != nil {
		backend := spec.Execution.Backend.OrDefault()
		if !backend.Valid() {
			add("invalid execution backend %q", spec.Execution.Backend)
		}
		if backend == model.ExecutionHook {
			if mode != model.ModeFunctional {
				add("execution backend \"hook\" requires functional mode")
			}
			if !hasHookUnit(declared) {
				add("execution backend \"hook\" requires a hook unit")
			}
		}
	}

	if rubric, err := ParseCriteria(filepath.Join(s.Dir, "criteria.json")); err != nil {
		add("criteria.json: %v", err)
	} else {
		problems = append(problems, ValidateRubric(rubric)...)
	}

	setup := filepath.Join(s.Dir, "inputs", "setup.sh")
	if info, err := os.Stat(setup); err == nil {
		if info.Mode()&0o111 == 0 {
			add("inputs/setup.sh is not executable")
		}
		if data, err := os.ReadFile(setup); err == nil {
			lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
			if len(lines) == 0 || lines[0] != "#!/usr/bin/env bash" {
				add("inputs/setup.sh shebang must be #!/usr/bin/env bash")
			}
			if !strings.Contains(string(data), "set -euo pipefail") {
				add("inputs/setup.sh is missing set -euo pipefail")
			}
		}
	}

	return problems
}

// ValidateRubric returns runtime rubric problems. It does not impose the
// authoring-only item-count and denominator rules.
func ValidateRubric(rubric model.Rubric) []string {
	var problems []string
	if rubric.Type != model.RubricTypeWeightedChecklist {
		problems = append(problems, fmt.Sprintf("type must be %s", model.RubricTypeWeightedChecklist))
	}
	if len(rubric.Checklist) == 0 {
		problems = append(problems, "checklist is empty")
	}
	seen := map[string]bool{}
	for i, item := range rubric.Checklist {
		if strings.TrimSpace(item.Name) == "" {
			problems = append(problems, fmt.Sprintf("checklist item %d has an empty name", i))
			continue
		}
		if seen[item.Name] {
			problems = append(problems, fmt.Sprintf("duplicate criterion name %q", item.Name))
		}
		seen[item.Name] = true
		if item.MaxScore <= 0 {
			problems = append(problems, fmt.Sprintf("criterion %q has a non-positive max_score", item.Name))
		}
	}
	if rubric.Denominator() <= 0 {
		problems = append(problems, "rubric denominator must be positive")
	}
	return problems
}

// UnitPattern reports whether unit has valid unit syntax.
func UnitPattern(unit string) bool { return unitPattern.MatchString(unit) }

// SplitUnit returns the unit kind ("skills", "commands", "hooks") and name.
func SplitUnit(unit string) (kind, name string, ok bool) {
	if !unitPattern.MatchString(unit) {
		return "", "", false
	}
	index := strings.IndexByte(unit, '/')
	return unit[:index], unit[index+1:], true
}

// UnitExists reports whether a declared unit exists in the plugin.
func UnitExists(root, plugin, unit string) bool {
	kind, name, ok := SplitUnit(unit)
	if !ok {
		return false
	}
	base := filepath.Join(root, "plugins", plugin)
	switch kind {
	case "skills":
		return isDir(filepath.Join(base, "skills", name))
	case "commands":
		return isFile(filepath.Join(base, "commands", name+".md"))
	case "hooks":
		return isFile(filepath.Join(base, "hooks", name+".sh")) ||
			isFile(filepath.Join(base, "hooks", name))
	default:
		return false
	}
}

func hasHookUnit(units []string) bool {
	for _, unit := range units {
		if strings.HasPrefix(unit, "hooks/") {
			return true
		}
	}
	return false
}

func dedupe(units []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(units))
	for _, unit := range units {
		if unit == "" || seen[unit] {
			continue
		}
		seen[unit] = true
		result = append(result, unit)
	}
	return result
}

func isJSONNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

func insideDir(dir, path string) bool {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
