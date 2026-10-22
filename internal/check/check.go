// Package check implements the deterministic authoring gate for eval
// scenarios, plugin manifests, and skills. It runs no model and no network.
package check

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/scenario"
)

// Finding classes distinguish legacy shell-gate rules from v3 tightenings.
const (
	ClassShellGate = "shell-gate"
	ClassV3        = "v3"
)

// Finding is one authoring-gate failure.
type Finding struct {
	Rule    string `json:"rule"`
	Class   string `json:"class"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
	Line    int    `json:"line,omitempty"`
}

// Result is the complete gate output.
type Result struct {
	Findings []Finding `json:"findings"`
}

// OK reports whether the gate found nothing.
func (r Result) OK() bool { return len(r.Findings) == 0 }

type checker struct {
	root      string
	workspace string
	findings  []Finding
}

func (c *checker) add(rule, class, message string) {
	c.findings = append(c.findings, Finding{Rule: rule, Class: class, Message: message})
}

// Run executes the full deterministic gate under root.
func Run(root string) (Result, error) {
	c := &checker{root: root}
	c.readWorkspace()
	c.checkPlacement()

	entries, err := scenario.Discover(root)
	if err != nil {
		return Result{}, err
	}
	for _, entry := range entries {
		c.checkScenario(entry)
	}
	c.checkManifests()
	c.checkSkills()
	return Result{Findings: c.findings}, nil
}

func (c *checker) readWorkspace() {
	data, err := os.ReadFile(filepath.Join(c.root, "tessl.json"))
	if err == nil {
		var doc struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(data, &doc) == nil {
			if index := strings.IndexByte(doc.Name, '/'); index >= 0 {
				c.workspace = doc.Name[:index]
			} else {
				c.workspace = doc.Name
			}
		}
	}
	if c.workspace == "" {
		c.add("workspace", ClassShellGate, `tessl.json: no workspace name (expected {"name": "<workspace>/<project>"})`)
	}
}

func (c *checker) checkPlacement() {
	if _, err := os.Lstat(filepath.Join(c.root, "evals")); err == nil {
		c.add("placement", ClassShellGate, "evals/ exists at the repository root - scenarios belong inside their plugin")
	}
	plugins, _ := scenario.Plugins(c.root)
	for _, plugin := range plugins {
		skillsDir := filepath.Join(c.root, "plugins", plugin, "skills")
		children, err := os.ReadDir(skillsDir)
		if err != nil {
			continue
		}
		for _, child := range children {
			stray := filepath.Join("plugins", plugin, "skills", child.Name(), "evals") + "/"
			if info, err := os.Stat(filepath.Join(c.root, stray)); err == nil && info.IsDir() {
				c.add("placement", ClassShellGate, stray+": scenarios belong at the plugin root (plugins/<plugin>/evals/), not under a skill")
			}
		}
	}
}

func (c *checker) checkScenario(entry scenario.Entry) {
	dir := entry.Dir
	rel := filepath.ToSlash(filepath.Join("plugins", entry.Plugin, "evals", entry.Name))

	if regexp.MustCompile(`^scenario-[0-9]+$`).MatchString(entry.Name) {
		c.add("scenario-name", ClassShellGate, rel+": canonical tessl name - use a descriptive name, a re-download would overwrite hand edits")
	}

	for _, required := range []string{"task.md", "criteria.json", "scenario.json", "inputs"} {
		if _, err := os.Stat(filepath.Join(dir, required)); err != nil {
			c.add("scenario-files", ClassShellGate, rel+": missing "+required)
		}
	}
	if info, err := os.Stat(filepath.Join(dir, "task.md")); err == nil && info.Mode().IsRegular() && info.Size() == 0 {
		c.add("scenario-task", ClassShellGate, rel+"/task.md: empty")
	}

	scenarioPath := filepath.Join(dir, "scenario.json")
	if info, err := os.Stat(scenarioPath); err == nil && info.Mode().IsRegular() {
		c.checkScenarioJSON(rel, dir, scenarioPath)
	}

	criteriaPath := filepath.Join(dir, "criteria.json")
	if info, err := os.Stat(criteriaPath); err == nil && info.Mode().IsRegular() {
		c.checkCriteria(rel, criteriaPath)
	}

	c.checkFixture(rel, dir)
}

type rawExecution struct {
	Backend json.RawMessage `json:"backend"`
	Cases   json.RawMessage `json:"cases"`
}

type rawScenario struct {
	Description json.RawMessage `json:"description"`
	Include     json.RawMessage `json:"include"`
	Unit        json.RawMessage `json:"unit"`
	Units       json.RawMessage `json:"units"`
	Mode        json.RawMessage `json:"mode"`
	Execution   json.RawMessage `json:"execution"`
}

func (c *checker) checkScenarioJSON(rel, dir, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		c.add("scenario-json", ClassShellGate, rel+"/scenario.json: invalid JSON")
		return
	}
	var doc rawScenario
	if err := json.Unmarshal(data, &doc); err != nil {
		c.add("scenario-json", ClassShellGate, rel+"/scenario.json: invalid JSON")
		return
	}

	if description := rawString(doc.Description); strings.TrimSpace(description) == "" {
		c.add("scenario-description", ClassV3, rel+"/scenario.json: description is empty")
	}

	// include must be an array whose paths exist inside the scenario dir.
	if !rawIsArray(doc.Include) {
		c.add("scenario-include", ClassShellGate, rel+"/scenario.json: 'include' must be an array")
	} else {
		for _, include := range rawStrings(doc.Include) {
			if include == "" {
				continue
			}
			full := filepath.Join(dir, include)
			if _, err := os.Stat(full); err != nil {
				c.add("scenario-include", ClassShellGate, rel+"/scenario.json: include path does not exist: "+include)
				continue
			}
			if !insideScenarioDir(dir, full) {
				c.add("scenario-include", ClassV3, rel+"/scenario.json: include path resolves outside the scenario directory: "+include)
			}
		}
	}

	hasUnit := rawPresent(doc.Unit)
	hasUnits := rawPresent(doc.Units)
	if hasUnit && hasUnits {
		c.add("scenario-units", ClassShellGate, rel+"/scenario.json: use either 'unit' or 'units', not both")
	}
	units := rawUnits(doc.Unit, doc.Units)
	if len(units) == 0 {
		c.add("scenario-units", ClassShellGate, rel+"/scenario.json: missing unit metadata (add 'unit' or 'units')")
	} else {
		for _, unit := range units {
			kind, name, ok := scenario.SplitUnit(unit)
			if !ok {
				c.add("scenario-units", ClassShellGate,
					fmt.Sprintf("%s/scenario.json: unit '%s' must use skills/<name>, commands/<name> or hooks/<name>", rel, unit))
				continue
			}
			var exist bool
			switch kind {
			case "skills":
				exist = isDir(filepath.Join(c.root, "plugins", entryPlugin(rel), "skills", name))
			case "commands":
				exist = isFile(filepath.Join(c.root, "plugins", entryPlugin(rel), "commands", name+".md"))
			case "hooks":
				base := filepath.Join(c.root, "plugins", entryPlugin(rel), "hooks", name)
				exist = isFile(base+".sh") || isFile(base)
			}
			if !exist {
				c.add("scenario-units", ClassShellGate,
					fmt.Sprintf("%s/scenario.json: unit '%s' does not exist in plugins/%s/%s", rel, unit, entryPlugin(rel), kind))
			}
		}
	}

	mode := "comparison"
	if rawPresent(doc.Mode) {
		if value, ok := rawStringOK(doc.Mode); ok {
			mode = value
		} else {
			mode = string(doc.Mode)
		}
	}
	switch mode {
	case "comparison":
	case "functional":
		hasHook := false
		for _, unit := range units {
			if strings.HasPrefix(unit, "hooks/") {
				hasHook = true
			}
		}
		if !hasHook {
			c.add("scenario-mode", ClassShellGate, rel+"/scenario.json: functional mode requires a hook unit")
		}
	default:
		c.add("scenario-mode", ClassShellGate, rel+"/scenario.json: mode must be comparison or functional")
	}

	if rawPresent(doc.Execution) {
		var execution rawExecution
		if err := json.Unmarshal(doc.Execution, &execution); err != nil {
			c.add("scenario-execution", ClassV3, rel+"/scenario.json: execution must be an object")
			return
		}
		backend := "agent"
		if rawPresent(execution.Backend) {
			if value, ok := rawStringOK(execution.Backend); ok {
				backend = value
			} else {
				backend = string(execution.Backend)
			}
		}
		switch backend {
		case "agent":
		case "hook":
			if mode != "functional" {
				c.add("scenario-execution", ClassV3, rel+"/scenario.json: execution backend 'hook' requires functional mode")
			}
			hasHook := false
			for _, unit := range units {
				if strings.HasPrefix(unit, "hooks/") {
					hasHook = true
				}
			}
			if !hasHook {
				c.add("scenario-execution", ClassV3, rel+"/scenario.json: execution backend 'hook' requires a hook unit")
			}
		default:
			c.add("scenario-execution", ClassV3, rel+"/scenario.json: execution backend must be agent or hook")
		}
	}
}

func entryPlugin(rel string) string {
	rest := strings.TrimPrefix(rel, "plugins/")
	plugin, _, _ := strings.Cut(rest, "/")
	return plugin
}

func (c *checker) checkCriteria(rel, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		c.add("criteria-json", ClassShellGate, rel+"/criteria.json: invalid JSON")
		return
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		c.add("criteria-json", ClassShellGate, rel+"/criteria.json: invalid JSON")
		return
	}
	prefix := rel + "/criteria.json: "

	if context := rawString(doc["context"]); context == "" {
		c.add("criteria-context", ClassShellGate, prefix+"empty context")
	}
	if rawType(doc["type"]) != model.RubricTypeWeightedChecklist {
		c.add("criteria-type", ClassShellGate, prefix+"type must be weighted_checklist")
	}

	itemsRaw, ok := doc["checklist"]
	var items []json.RawMessage
	if ok {
		_ = json.Unmarshal(itemsRaw, &items)
	}
	if len(items) < model.RubricMinItems || len(items) > model.RubricMaxItems {
		c.add("criteria-count", ClassShellGate,
			fmt.Sprintf("%s%d checklist items (expected %d-%d)", prefix, len(items), model.RubricMinItems, model.RubricMaxItems))
	}

	var names []string
	var sum float64
	for _, raw := range items {
		var item struct {
			Name        json.RawMessage `json:"name"`
			Description json.RawMessage `json:"description"`
			MaxScore    json.RawMessage `json:"max_score"`
		}
		_ = json.Unmarshal(raw, &item)
		name, nameOK := rawStringOK(item.Name)
		description := rawString(item.Description)
		maxScore, scoreOK := rawNumber(item.MaxScore)
		if !nameOK || name == "" || description == "" || !scoreOK || maxScore <= 0 {
			label := name
			if label == "" {
				label = "(unnamed)"
			}
			c.add("criteria-item", ClassShellGate,
				prefix+"item needs a name, a description and a positive max_score: "+label)
		}
		if nameOK {
			names = append(names, name)
		}
		if scoreOK {
			sum += maxScore
		}
	}

	seen := map[string]int{}
	for _, name := range names {
		seen[name]++
	}
	var duplicates []string
	for name, count := range seen {
		if count > 1 {
			duplicates = append(duplicates, name)
		}
	}
	if len(duplicates) > 0 {
		sort.Strings(duplicates)
		c.add("criteria-duplicate", ClassShellGate, prefix+"duplicate item names: "+strings.Join(duplicates, ", "))
	}

	if sum != model.RubricAuthoringSum {
		c.add("criteria-sum", ClassShellGate,
			fmt.Sprintf("%smax_score sums to %s (expected 100)", prefix, formatNumber(sum)))
	}
}

var (
	cdPattern       = regexp.MustCompile(`(^|[;&|(])[[:space:]]*cd[[:space:]]+[^;&|)]+`)
	gitQuietPattern = regexp.MustCompile(`git init[^;&|#]*[[:space:]](-q|--quiet)([[:space:];&|)]|$)`)
	hiddenPattern   = regexp.MustCompile(`\$HOME|~/`)
)

func (c *checker) checkFixture(rel, dir string) {
	inputsDir := filepath.Join(dir, "inputs")
	setup := filepath.Join(inputsDir, "setup.sh")
	info, err := os.Stat(setup)
	if err != nil || !info.Mode().IsRegular() {
		c.add("fixture", ClassShellGate, rel+"/inputs: no setup.sh")
		return
	}

	data, err := os.ReadFile(setup)
	if err == nil {
		content := string(data)
		setupRel := rel + "/inputs/setup.sh"
		if info.Mode()&0o111 == 0 {
			c.add("fixture", ClassShellGate, setupRel+": not executable")
		}
		lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
		if len(lines) == 0 || lines[0] != "#!/usr/bin/env bash" {
			c.add("fixture", ClassShellGate, setupRel+": shebang must be #!/usr/bin/env bash")
		}
		if !strings.Contains(content, "set -euo pipefail") {
			c.add("fixture", ClassShellGate, setupRel+": missing set -euo pipefail")
		}
		if hiddenPattern.MatchString(content) {
			c.add("fixture", ClassShellGate, setupRel+": writes outside the workspace ($HOME) - authors run setups locally")
		}

		topLevelInit := false
		nested := false
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimLeftFunc(line, unicode.IsSpace), "#") {
				continue
			}
			if loc := cdPattern.FindStringIndex(line); loc != nil {
				target := strings.TrimSpace(line[loc[0]:loc[1]])
				if index := strings.LastIndex(target, "cd "); index >= 0 {
					target = target[index+3:]
				}
				target = strings.TrimSpace(target)
				nested = !strings.Contains(target, "dirname") &&
					!strings.Contains(target, "$0") &&
					!strings.Contains(target, "BASH_SOURCE") &&
					!matchesDotTarget(target)
			}
			if strings.Contains(line, "git init") {
				if !gitQuietPattern.MatchString(line) {
					c.add("fixture", ClassShellGate,
						fmt.Sprintf("%s:%d: git init must be quiet (git init -q)", setupRel, i+1))
				}
				if !nested {
					topLevelInit = true
				}
			}
		}
		if topLevelInit && !strings.Contains(content, ".git/info/exclude") {
			c.add("fixture", ClassShellGate,
				setupRel+": initialises the working directory without a .git/info/exclude for fixture files")
		}

		entries, _ := os.ReadDir(inputsDir)
		for _, entry := range entries {
			base := entry.Name()
			if base == "setup.sh" {
				continue
			}
			if !containsWord(content, base) {
				c.add("fixture", ClassShellGate,
					rel+"/inputs/"+base+": nothing in setup.sh references it (dead fixture)")
			}
		}
	}

	entries, _ := os.ReadDir(inputsDir)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		base := entry.Name()
		if base == "setup.sh" {
			continue
		}
		if strings.HasSuffix(base, ".sh") {
			c.add("fixture", ClassShellGate,
				rel+"/inputs/"+base+": only inputs/setup.sh may be a shell entry point")
		}
	}
}

func matchesDotTarget(target string) bool {
	target = strings.Trim(target, `"'`)
	target = strings.TrimSuffix(target, "/")
	return target == "." || target == ".."
}

func containsWord(haystack, word string) bool {
	start := 0
	for {
		index := strings.Index(haystack[start:], word)
		if index < 0 {
			return false
		}
		index += start
		before := index == 0 || !isWordByte(haystack[index-1])
		after := index+len(word) >= len(haystack) || !isWordByte(haystack[index+len(word)])
		if before && after {
			return true
		}
		start = index + 1
	}
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

type manifest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Private     *bool  `json:"private"`
}

func (c *checker) checkManifests() {
	plugins, _ := scenario.Plugins(c.root)
	for _, plugin := range plugins {
		pluginDir := filepath.Join(c.root, "plugins", plugin)
		evalsDir := filepath.Join(pluginDir, "evals")
		evalsInfo, err := os.Stat(evalsDir)
		if err != nil || !evalsInfo.IsDir() {
			continue
		}
		relDir := filepath.ToSlash(filepath.Join("plugins", plugin)) + "/"
		if entries, err := os.ReadDir(evalsDir); err == nil && len(entries) == 0 {
			c.add("manifest", ClassShellGate, relDir+"/evals: empty directory")
		}

		claudePath := filepath.Join(pluginDir, ".claude-plugin", "plugin.json")
		claude, _, claudeOK := readManifest(claudePath)
		if !isFile(claudePath) {
			c.add("manifest", ClassShellGate, relDir+"/.claude-plugin/plugin.json: missing (the Claude manifest is the source of truth)")
		} else if !claudeOK {
			c.add("manifest", ClassShellGate, relDir+"/.claude-plugin/plugin.json: invalid JSON")
		}

		tesslPath := filepath.Join(pluginDir, ".tessl-plugin", "plugin.json")
		tessl, hasPrivate, tesslOK := readManifest(tesslPath)
		if !isFile(tesslPath) {
			c.add("manifest", ClassShellGate, relDir+"/.tessl-plugin/plugin.json: missing - scenarios need a tessl manifest to run")
		} else if !tesslOK {
			c.add("manifest", ClassShellGate, relDir+"/.tessl-plugin/plugin.json: invalid JSON")
		} else {
			if c.workspace != "" {
				expected := c.workspace + "/" + plugin
				if tessl.Name != expected {
					c.add("manifest", ClassShellGate,
						fmt.Sprintf("%s/.tessl-plugin/plugin.json: name is '%s' (expected '%s')", relDir, tessl.Name, expected))
				}
			}
			if hasPrivate && (tessl.Private == nil || !*tessl.Private) {
				c.add("manifest", ClassShellGate, relDir+"/.tessl-plugin/plugin.json: private must be true for repository plugins")
			}
			if claudeOK {
				if tessl.Version != claude.Version {
					c.add("manifest", ClassShellGate,
						fmt.Sprintf("%s/.tessl-plugin/plugin.json: version '%s' does not match the Claude manifest '%s'", relDir, tessl.Version, claude.Version))
				}
				if tessl.Description != claude.Description {
					c.add("manifest", ClassShellGate, relDir+"/.tessl-plugin/plugin.json: description does not match the Claude manifest")
				}
			}
		}
	}
}

func readManifest(path string) (manifest, bool, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest{}, false, false
	}
	var doc manifest
	if err := json.Unmarshal(data, &doc); err != nil {
		return manifest{}, false, false
	}
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(data, &raw)
	_, hasPrivate := raw["private"]
	return doc, hasPrivate, true
}

func (c *checker) checkSkills() {
	plugins, _ := scenario.Plugins(c.root)
	for _, plugin := range plugins {
		skillsDir := filepath.Join(c.root, "plugins", plugin, "skills")
		children, err := os.ReadDir(skillsDir)
		if err != nil {
			continue
		}
		sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
		for _, child := range children {
			if !child.IsDir() {
				continue
			}
			path := filepath.Join(skillsDir, child.Name(), "SKILL.md")
			if _, err := os.Stat(path); err != nil {
				continue
			}
			c.checkSkill(filepath.ToSlash(filepath.Join("plugins", plugin, "skills", child.Name(), "SKILL.md")), path, child.Name())
		}
	}
}

var (
	nameKeyPattern        = regexp.MustCompile(`^name:`)
	descriptionKeyPattern = regexp.MustCompile(`^description:`)
	nameValuePattern      = regexp.MustCompile(`^name:[ \t]*(.*)`)
	descValuePattern      = regexp.MustCompile(`^description:[ \t]*(.*)`)
	allowedToolsPattern   = regexp.MustCompile(`^allowed-tools:[ \t]*(.*)`)
	blockCharPattern      = regexp.MustCompile(`^([|>][+-]?)$`)
	unboundedToolPattern  = regexp.MustCompile(`^Bash\([[:space:]]*:?[[:space:]]*\*[[:space:]]*\)$`)
	pointerPattern        = regexp.MustCompile("`plugins/[^`]+`")
)

func (c *checker) checkSkill(rel, path, skillName string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(content, "\n")

	if len(lines) == 0 || lines[0] != "---" {
		c.add("skill-frontmatter", ClassShellGate, rel+": no YAML frontmatter")
	} else {
		var frontmatter []string
		closed := false
		for _, line := range lines[1:] {
			if strings.TrimRight(line, " \t") == "---" {
				closed = true
				break
			}
			frontmatter = append(frontmatter, line)
		}
		if !closed {
			c.add("skill-frontmatter", ClassShellGate, rel+": frontmatter is not closed (no second ---)")
		} else {
			nameKeys, descriptionKeys := 0, 0
			declaredName := ""
			description := ""
			foundName := false
			foundDescription := false
			for _, line := range frontmatter {
				if nameKeyPattern.MatchString(line) {
					nameKeys++
				}
				if descriptionKeyPattern.MatchString(line) {
					descriptionKeys++
				}
			}
			for _, line := range frontmatter {
				if match := nameValuePattern.FindStringSubmatch(line); match != nil && !foundName {
					declaredName = match[1]
					foundName = true
					break
				}
			}
			for index, line := range frontmatter {
				match := descValuePattern.FindStringSubmatch(line)
				if match == nil || foundDescription {
					continue
				}
				foundDescription = true
				description = match[1]
				if blockCharPattern.MatchString(description) {
					var parts []string
					for _, following := range frontmatter[index+1:] {
						if following == "" || (following[0] != ' ' && following[0] != '\t') {
							break
						}
						parts = append(parts, strings.TrimLeft(following, " \t"))
					}
					description = ""
					for _, part := range parts {
						description += part + " "
					}
				}
				break
			}

			if nameKeys != 1 {
				c.add("skill-frontmatter", ClassShellGate,
					fmt.Sprintf("%s: %d 'name:' keys in frontmatter (expected 1)", rel, nameKeys))
			}
			if descriptionKeys != 1 {
				c.add("skill-frontmatter", ClassShellGate,
					fmt.Sprintf("%s: %d 'description:' keys in frontmatter (expected 1)", rel, descriptionKeys))
			}
			if declaredName != skillName {
				c.add("skill-frontmatter", ClassShellGate,
					fmt.Sprintf("%s: frontmatter name '%s' does not match the directory '%s'", rel, declaredName, skillName))
			}
			if utf8.RuneCountInString(description) < 60 {
				c.add("skill-description", ClassShellGate,
					fmt.Sprintf("%s: description is %d characters - too short to carry activation triggers", rel, utf8.RuneCountInString(description)))
			}

			for _, tool := range allowedTools(frontmatter) {
				if tool == "Bash" || unboundedToolPattern.MatchString(tool) || strings.Contains(tool, "PATH=*") {
					c.add("skill-allowed-tools", ClassShellGate,
						fmt.Sprintf("%s: allowed-tools contains '%s' - it approves every command", rel, tool))
				}
			}
		}
	}

	lineCount := strings.Count(content, "\n")
	if lineCount > 500 {
		c.add("skill-length", ClassShellGate, fmt.Sprintf("%s: %d lines (Agent Skills limit is 500)", rel, lineCount))
	}

	for _, match := range pointerPattern.FindAllString(content, -1) {
		pointer := strings.Trim(match, "`")
		if strings.ContainsAny(pointer, "<>*") {
			continue
		}
		if index := strings.IndexByte(pointer, '#'); index >= 0 {
			pointer = pointer[:index]
		}
		if index := strings.IndexByte(pointer, ':'); index >= 0 {
			pointer = pointer[:index]
		}
		if _, err := os.Stat(filepath.Join(c.root, pointer)); err != nil {
			c.add("skill-pointer", ClassShellGate,
				fmt.Sprintf("%s: points at a path that does not exist: %s", rel, pointer))
		}
	}
}

func allowedTools(frontmatter []string) []string {
	var values []string
	collecting := false
	for _, line := range frontmatter {
		if match := allowedToolsPattern.FindStringSubmatch(line); match != nil {
			values = append(values, match[1])
			collecting = true
			continue
		}
		if collecting {
			trimmed := strings.TrimLeft(line, " \t")
			if trimmed == line {
				break
			}
			trimmed = strings.TrimPrefix(trimmed, "-")
			values = append(values, strings.TrimLeft(trimmed, " \t"))
			continue
		}
	}
	var tools []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			part = strings.Trim(part, `"'`)
			if part != "" {
				tools = append(tools, part)
			}
		}
	}
	return tools
}

// Changes lists the skills and scenarios a ref touched.
type Changes struct {
	Skills    []string `json:"skills"`
	Scenarios []string `json:"scenarios"`
}

// DetectChanges implements check --changes with a three-dot diff against HEAD.
func DetectChanges(root, ref string) (Changes, error) {
	command := exec.Command("git", "-C", root, "rev-parse", "--git-dir")
	if err := command.Run(); err != nil {
		return Changes{}, fmt.Errorf("--changes needs a git checkout")
	}
	command = exec.Command("git", "-C", root, "diff", "--name-only", ref+"...HEAD", "--", "plugins")
	output, err := command.Output()
	if err != nil {
		return Changes{}, fmt.Errorf("cannot diff against '%s' (fetch it first)", ref)
	}
	skillPattern := regexp.MustCompile(`^plugins/[^/]+/skills/[^/]+`)
	scenarioPattern := regexp.MustCompile(`^plugins/[^/]+/evals/[^/]+`)
	skills := map[string]bool{}
	scenarios := map[string]bool{}
	for _, line := range strings.Split(string(output), "\n") {
		if match := skillPattern.FindString(line); match != "" {
			skills[match] = true
		}
		if match := scenarioPattern.FindString(line); match != "" {
			scenarios[match] = true
		}
	}
	return Changes{Skills: sortedKeys(skills), Scenarios: sortedKeys(scenarios)}, nil
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func rawPresent(raw json.RawMessage) bool {
	return len(raw) > 0 && strings.TrimSpace(string(raw)) != "null"
}

func rawIsArray(raw json.RawMessage) bool {
	return rawPresent(raw) && strings.HasPrefix(strings.TrimSpace(string(raw)), "[")
}

func rawString(raw json.RawMessage) string {
	value, _ := rawStringOK(raw)
	return value
}

func rawStringOK(raw json.RawMessage) (string, bool) {
	if !rawPresent(raw) {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

func rawStrings(raw json.RawMessage) []string {
	var values []string
	_ = json.Unmarshal(raw, &values)
	return values
}

func rawType(raw json.RawMessage) string {
	return rawString(raw)
}

func rawNumber(raw json.RawMessage) (float64, bool) {
	if !rawPresent(raw) {
		return 0, false
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, false
	}
	return value, true
}

func rawUnits(unit, units json.RawMessage) []string {
	var result []string
	if rawIsArray(units) {
		for _, value := range rawStrings(units) {
			if value != "" {
				result = append(result, value)
			}
		}
		return result
	}
	if rawPresent(units) {
		// Mirrors jq's `.units // [.unit] // []`: a non-null non-array units
		// value wins and yields no iteration.
		return nil
	}
	if value, ok := rawStringOK(unit); ok && value != "" {
		return []string{value}
	}
	return nil
}

func formatNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func insideScenarioDir(dir, path string) bool {
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
