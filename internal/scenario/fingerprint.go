package scenario

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rikdc/ai-skills/internal/model"
)

// Fingerprint hashes everything that shapes a scenario's runs: the scenario
// tree, the plugin's skills/commands/hooks, the Claude manifest, security
// patterns, and the harness/schema versions. Editing any input changes it.
func Fingerprint(root, plugin, name string) (string, error) {
	pluginDir := filepath.Join(root, "plugins", plugin)
	scenarioDir := filepath.Join(pluginDir, "evals", name)
	digest := sha256.New()

	// Fixed tree order: scenario, skills, commands, hooks.
	trees := []string{
		scenarioDir,
		filepath.Join(pluginDir, "skills"),
		filepath.Join(pluginDir, "commands"),
		filepath.Join(pluginDir, "hooks"),
	}
	for _, tree := range trees {
		if err := hashTree(digest, tree); err != nil {
			return "", err
		}
	}

	manifest := filepath.Join(pluginDir, ".claude-plugin", "plugin.json")
	if data, err := os.ReadFile(manifest); err == nil {
		fmt.Fprintf(digest, "manifest %s\n", hashBytes(data))
	} else if os.IsNotExist(err) {
		fmt.Fprint(digest, "manifest none\n")
	} else {
		return "", fmt.Errorf("fingerprint manifest: %w", err)
	}

	patterns := filepath.Join(pluginDir, "security-patterns.conf")
	if data, err := os.ReadFile(patterns); err == nil {
		fmt.Fprintf(digest, "patterns %s\n", hashBytes(data))
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("fingerprint security patterns: %w", err)
	}

	fmt.Fprintf(digest, "harness %s\n", model.Version)
	fmt.Fprintf(digest, "schema %d\n", model.SchemaVersion)
	return hex.EncodeToString(digest.Sum(nil)), nil
}

type fingerprintFile struct {
	rel     string
	exec    bool
	symlink bool
	target  string
	ignored bool
}

// hashTree writes one canonical line per regular file or symlink under root.
func hashTree(digest hash.Hash, root string) error {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}

	var files []fingerprintFile
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			return nil
		}
		if ignoredFingerprintFile(filepath.Base(path)) {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			files = append(files, fingerprintFile{rel: rel, symlink: true, target: filepath.ToSlash(target)})
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		files = append(files, fingerprintFile{rel: rel, exec: info.Mode()&0o111 != 0})
		return nil
	})
	if err != nil {
		return err
	}

	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	for _, file := range files {
		if file.symlink {
			fmt.Fprintf(digest, "symlink %s %s\n", file.rel, file.target)
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(file.rel))
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		mode := "file"
		if file.exec {
			mode = "exec"
		}
		fmt.Fprintf(digest, "%s %s %s\n", mode, file.rel, hashBytes(content))
	}
	return nil
}

func ignoredFingerprintFile(base string) bool {
	if base == ".DS_Store" || base == "result.json" || base == "result.jsonl" {
		return true
	}
	return strings.HasSuffix(base, ".log")
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
