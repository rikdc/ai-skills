package suite

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/platform/errs"
	"github.com/rikdc/ai-skills/internal/scenario"
)

var errNotSuite = errors.New("not an evalctl suite directory")

// IsNotSuite reports whether err means the directory has no suite manifest,
// so there is nothing to refresh.
func IsNotSuite(err error) bool { return errors.Is(err, errNotSuite) }

// Refresh recomputes the aggregate artifacts of an existing suite (results.jsonl,
// suite.json, grading/index.tsv and summary.md) from its run directories. A
// standalone `evalctl grade` adds judge attempts after the suite wrote those
// files, so without a refresh their counts, scores and pending totals describe
// the suite as it stood before grading.
//
// Only derived state changes. The planned/executed/skipped counts, the
// recorded duration and the kept-failure count describe how the suite ran,
// not what is on disk, and are carried over from the existing suite.json.
func Refresh(root, suiteDir string) error {
	out, err := filepath.Abs(suiteDir)
	if err != nil {
		return errs.Usage("resolve suite directory: %v", err)
	}
	var manifest model.SuiteManifest
	data, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %s has no manifest.json", errNotSuite, out)
	}
	if err != nil {
		return errs.Operational("read manifest: %v", err)
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return errs.Operational("parse manifest: %v", err)
	}

	var previous suiteDocument
	if data, err := os.ReadFile(filepath.Join(out, "suite.json")); err == nil {
		if err := json.Unmarshal(data, &previous); err != nil {
			return errs.Operational("parse suite.json: %v", err)
		}
	}

	release, err := lockFile(out)
	if err != nil {
		return err
	}
	defer release()

	fingerprints := make(map[string]string, len(manifest.Scenarios))
	selected := make([]scenario.Selected, 0, len(manifest.Scenarios))
	for _, planned := range manifest.Scenarios {
		item := scenario.Selected{
			Scenario: &scenario.Scenario{
				Root:   root,
				Plugin: planned.Plugin,
				Name:   planned.Scenario,
				Dir:    filepath.Join(root, "plugins", planned.Plugin, "evals", planned.Scenario),
				Meta: scenario.Meta{Spec: model.Scenario{
					Mode: model.Mode(planned.Mode),
				}},
			},
			Declared: planned.Units,
		}
		fingerprints[item.Pair()] = planned.Fingerprint
		selected = append(selected, item)
	}

	r := &Runner{cfg: Config{
		Root: root,
		Out:  out,
		Arm:  manifest.Config.Arm,
		Runs: manifest.Config.Runs,
	}}
	stats := r.aggregate(selected, fingerprints, nil, time.Now(),
		previous.Counts["executed"], previous.Counts["skipped"])
	stats.DurationSeconds = previous.DurationSeconds
	for _, key := range []string{"duration_seconds", "kept_failures"} {
		if value, ok := previous.Counts[key]; ok {
			stats.Counts[key] = value
		}
	}
	if err := r.writeArtifacts(manifest, stats); err != nil {
		return errs.Operational("%v", err)
	}
	return nil
}
