// Package config resolves repository roots and flag/environment/default
// precedence for the CLI.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrRootNotFound is returned when no ancestor contains tessl.json and
// plugins/.
var ErrRootNotFound = fmt.Errorf("no repository root found (tessl.json and plugins/): %w", os.ErrNotExist)

// ResolveRoot applies flag > EVALCTL_ROOT > ancestor discovery precedence.
// An explicit flag or environment root must name an existing directory.
func ResolveRoot(flagRoot string, flagSet bool, cwd string, getenv func(string) string) (string, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	if flagSet {
		if strings.TrimSpace(flagRoot) == "" {
			return "", fmt.Errorf("--root is empty")
		}
		return existingDir(flagRoot)
	}
	if env := strings.TrimSpace(getenv("EVALCTL_ROOT")); env != "" {
		return existingDir(env)
	}
	start, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	dir := start
	for {
		if hasRootMarkers(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%w (starting from %s)", ErrRootNotFound, start)
		}
		dir = parent
	}
}

// ResolveString applies flag > first non-empty env > default precedence.
func ResolveString(flagValue string, flagSet bool, def string, getenv func(string) string, envNames ...string) string {
	if flagSet {
		return flagValue
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	for _, name := range envNames {
		if v := getenv(name); v != "" {
			return v
		}
	}
	return def
}

// ResolveBool applies flag > first parseable env > default precedence.
func ResolveBool(flagValue bool, flagSet bool, def bool, getenv func(string) string, envNames ...string) bool {
	if flagSet {
		return flagValue
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	for _, name := range envNames {
		if v, ok := parseBool(getenv(name)); ok {
			return v
		}
	}
	return def
}

func existingDir(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve root %q: %w", path, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("root %q: %w", path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("root %q is not a directory", path)
	}
	return abs, nil
}

func hasRootMarkers(dir string) bool {
	if info, err := os.Stat(filepath.Join(dir, "tessl.json")); err != nil || info.IsDir() {
		return false
	}
	if info, err := os.Stat(filepath.Join(dir, "plugins")); err != nil || !info.IsDir() {
		return false
	}
	return true
}

func parseBool(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		if v, err := strconv.ParseBool(s); err == nil {
			return v, true
		}
		return false, false
	}
}
