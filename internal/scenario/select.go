package scenario

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/rikdc/ai-skills/internal/model"
)

// Selection is the shared list/plan/suite selection input.
type Selection struct {
	Plugins []string
	Units   []string
	Pattern string
	Changed string
	Shard   string
}

// Selected is one scenario that passed selection.
type Selected struct {
	Scenario *Scenario
	// Units is the intersection of --unit and the scenario's declarations. It
	// is empty when no --unit filter was given.
	Units []string
	// Declared is the full declaration list used for plan's install line.
	Declared []string
}

// Pair returns the canonical plugin/scenario identifier.
func (s Selected) Pair() string { return s.Scenario.Pair() }

// InstallUnits returns the units a narrowed invocation installs.
func (s Selected) InstallUnits() []string {
	if len(s.Units) > 0 {
		return s.Units
	}
	return s.Declared
}

// Mode returns the effective mode.
func (s Selected) Mode() model.Mode { return s.Scenario.Mode() }

// Execution returns the effective execution backend.
func (s Selected) Execution() model.ExecutionBackend { return s.Scenario.Execution() }

// DirectHook reports whether direct hook execution (milestone C) is requested.
func (s Selected) DirectHook() bool { return s.Scenario.DirectHook() }

// Select applies plugin, unit, regex, changed, and shard filters in the legacy
// suite order. Warnings describe incomplete scenarios that were skipped.
func Select(root string, opts Selection) ([]Selected, []string, error) {
	if err := validateRequestedUnits(root, opts); err != nil {
		return nil, nil, err
	}

	var pattern *regexp.Regexp
	if opts.Pattern != "" {
		compiled, err := regexp.Compile(opts.Pattern)
		if err != nil {
			return nil, nil, usagef("invalid --scenario regex %q: %v", opts.Pattern, err)
		}
		pattern = compiled
	}

	changed, err := changedSelection(root, opts.Changed)
	if err != nil {
		return nil, nil, err
	}

	shardIndex, shardCount, err := parseShard(opts.Shard)
	if err != nil {
		return nil, nil, err
	}

	entries, err := Discover(root)
	if err != nil {
		return nil, nil, err
	}

	var selected []Selected
	var warnings []string
	position := 0
	for _, entry := range entries {
		if !contains(opts.Plugins, entry.Plugin) && len(opts.Plugins) > 0 {
			continue
		}
		if pattern != nil && !pattern.MatchString(entry.Pair()) {
			continue
		}
		if changed != nil && !changed.matches(entry.Plugin, entry.Name) {
			continue
		}
		if missing := missingRequiredFiles(entry.Dir); len(missing) > 0 {
			warnings = append(warnings, "skipping incomplete scenario: "+entry.Pair())
			continue
		}

		meta, err := LoadMeta(root, entry.Plugin, entry.Name)
		if err != nil {
			return nil, warnings, usagef("%v", err)
		}
		declared := dedupe(meta.DeclaredUnits())
		for _, unit := range declared {
			if !UnitPattern(unit) {
				return nil, warnings, usagef("scenario %s declares invalid unit '%s' (expected skills/<name>, commands/<name> or hooks/<name>)", entry.Pair(), unit)
			}
			if !UnitExists(root, entry.Plugin, unit) {
				return nil, warnings, usagef("scenario %s declares unknown unit '%s' for plugin '%s'", entry.Pair(), unit, entry.Plugin)
			}
		}

		mode := meta.Mode()
		if !mode.Valid() {
			return nil, warnings, usagef("scenario %s has invalid mode '%s'", entry.Pair(), mode)
		}
		if mode == model.ModeFunctional {
			if len(declared) == 0 {
				return nil, warnings, usagef("functional scenario %s must declare the unit it exercises", entry.Pair())
			}
			if !hasHookUnit(declared) {
				return nil, warnings, usagef("functional scenario %s must declare a hook unit", entry.Pair())
			}
		}
		if meta.DirectHook() {
			// Detected and reported by plan; selection itself keeps it so
			// list can show the scenario and plan can mark it unsupported.
			if mode != model.ModeFunctional {
				return nil, warnings, usagef("scenario %s has execution backend \"hook\" but mode %q (milestone C requires functional hook scenarios)", entry.Pair(), mode)
			}
		}

		var narrowed []string
		if len(opts.Units) > 0 {
			if len(declared) == 0 {
				return nil, warnings, usagef("scenario %s has no unit metadata; add \"unit\": \"skills/<name>\" or \"units\": [...] to scenario.json", entry.Pair())
			}
			for _, wanted := range opts.Units {
				if contains(declared, wanted) {
					narrowed = append(narrowed, wanted)
				}
			}
			if len(narrowed) == 0 {
				continue
			}
		}

		position++
		if shardCount > 1 && (position-1)%shardCount != shardIndex-1 {
			continue
		}
		selected = append(selected, Selected{
			Scenario: meta,
			Units:    narrowed,
			Declared: declared,
		})
	}

	if len(selected) == 0 {
		if len(opts.Units) > 0 {
			return nil, warnings, usagef("no scenarios declare unit(s): %s", strings.Join(opts.Units, " "))
		}
		return nil, warnings, usagef("no scenarios selected")
	}
	return selected, warnings, nil
}

func validateRequestedUnits(root string, opts Selection) error {
	for _, unit := range opts.Units {
		if !UnitPattern(unit) {
			return usagef("invalid --unit '%s' (expected skills/<name>, commands/<name> or hooks/<name>)", unit)
		}
		found := false
		plugins := opts.Plugins
		if len(plugins) == 0 {
			all, err := Plugins(root)
			if err != nil {
				return err
			}
			plugins = all
		}
		for _, plugin := range plugins {
			if UnitExists(root, plugin, unit) {
				found = true
				break
			}
		}
		if !found {
			return usagef("unknown --unit '%s' (not shipped by the selected plugin(s))", unit)
		}
	}
	return nil
}

func missingRequiredFiles(dir string) []string {
	var missing []string
	for _, required := range RequiredFiles {
		if _, err := os.Stat(filepath.Join(dir, required)); err != nil {
			missing = append(missing, required)
		}
	}
	return missing
}

func parseShard(value string) (index, count int, err error) {
	if value == "" {
		return 0, 1, nil
	}
	match := regexp.MustCompile(`^([0-9]+)/([0-9]+)$`).FindStringSubmatch(value)
	if match == nil {
		return 0, 0, usagef("--shard must look like 1/3")
	}
	index, err = strconv.Atoi(match[1])
	if err != nil {
		return 0, 0, usagef("--shard must look like 1/3")
	}
	count, err = strconv.Atoi(match[2])
	if err != nil {
		return 0, 0, usagef("--shard must look like 1/3")
	}
	if count < 1 {
		return 0, 0, usagef("--shard denominator must be at least 1")
	}
	if index < 1 || index > count {
		return 0, 0, usagef("--shard index must be between 1 and %d", count)
	}
	return index, count, nil
}

type changedSet struct {
	scenarios map[string]bool
	plugins   map[string]bool
}

func (c changedSet) matches(plugin, name string) bool {
	return c.scenarios[plugin+"/"+name] || c.plugins[plugin]
}

// changedSelection returns nil when no --changed filter was requested.
func changedSelection(root, ref string) (*changedSet, error) {
	if ref == "" {
		return nil, nil
	}
	if err := runGit(root, "rev-parse", "--git-dir"); err != nil {
		return nil, usagef("--changed needs a git checkout")
	}
	diff, err := gitOutput(root, "diff", "--name-only", ref, "--", "plugins")
	if err != nil {
		return nil, usagef("cannot diff against '%s' (fetch it first)", ref)
	}
	untracked, err := gitOutput(root, "ls-files", "--others", "--exclude-standard", "--", "plugins")
	if err != nil {
		return nil, usagef("cannot list untracked files under plugins")
	}
	paths := append(strings.Split(diff, "\n"), strings.Split(untracked, "\n")...)
	set := &changedSet{scenarios: map[string]bool{}, plugins: map[string]bool{}}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" || !strings.HasPrefix(path, "plugins/") {
			continue
		}
		rest := strings.TrimPrefix(path, "plugins/")
		plugin, tail, ok := strings.Cut(rest, "/")
		if !ok || plugin == "" || tail == "" {
			continue
		}
		switch {
		case strings.HasPrefix(tail, "evals/"):
			scenario := strings.TrimPrefix(tail, "evals/")
			scenario, _, _ = strings.Cut(scenario, "/")
			if scenario != "" {
				set.scenarios[plugin+"/"+scenario] = true
			}
		case strings.HasPrefix(tail, "skills/"),
			strings.HasPrefix(tail, "commands/"),
			strings.HasPrefix(tail, "hooks/"),
			strings.HasPrefix(tail, ".claude-plugin/"),
			strings.HasPrefix(tail, ".tessl-plugin/"),
			tail == "security-patterns.conf":
			set.plugins[plugin] = true
		}
	}
	return set, nil
}

func runGit(root string, args ...string) error {
	_, err := gitOutput(root, args...)
	return err
}

func gitOutput(root string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func usagef(format string, args ...any) error {
	return &usageError{message: fmt.Sprintf(format, args...)}
}

type usageError struct{ message string }

func (e *usageError) Error() string { return e.message }

// IsUsageError reports whether err is a selection usage error.
func IsUsageError(err error) bool {
	_, ok := err.(*usageError)
	return ok
}

// UsageMessage returns the message of a selection usage error.
func UsageMessage(err error) string {
	if e, ok := err.(*usageError); ok {
		return e.message
	}
	return err.Error()
}
