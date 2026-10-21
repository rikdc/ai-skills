package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/rikdc/ai-skills/internal/model"
)

// warnSameModelJudge tells the user, without blocking, that the judge and the
// agent are the same model, which tends to make scores lenient.
func warnSameModelJudge(cmd *cobra.Command, agentCmd, agentModel, judgeCmd, judgeModel string) {
	if name, same := model.SameJudgeModel(agentCmd, agentModel, judgeCmd, judgeModel); same {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", model.SameModelWarning(name))
	}
}

// suiteAgent reads the agent identity recorded in the manifest of the suite
// that contains dir (a suite directory or one of its run directories). It
// returns empty strings when dir is not inside a suite.
func suiteAgent(dir string) (agentCmd, agentModel string) {
	if root := suiteRootOf(dir); root != "" {
		var manifest model.SuiteManifest
		if data, err := os.ReadFile(filepath.Join(root, "manifest.json")); err == nil &&
			json.Unmarshal(data, &manifest) == nil {
			return manifest.Config.AgentCmd, manifest.Config.Model
		}
	}
	return "", ""
}

// suiteRootOf returns the nearest ancestor of dir (or dir itself) that holds
// a suite manifest and suite.json, or "".
func suiteRootOf(dir string) string {
	dir = filepath.Clean(dir)
	for range 8 {
		if fileExists(filepath.Join(dir, "manifest.json")) && fileExists(filepath.Join(dir, "suite.json")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
