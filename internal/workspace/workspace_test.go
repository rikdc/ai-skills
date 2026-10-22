package workspace

import (
	"archive/tar"
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func TestStageIncludesPreservesModeAndRejectsEscapingSymlink(t *testing.T) {
	scenarioDir := t.TempDir()
	writeFile(t, filepath.Join(scenarioDir, "inputs", "setup.sh"), "#!/usr/bin/env bash\n", 0o755)
	writeFile(t, filepath.Join(scenarioDir, "inputs", "data.txt"), "data\n", 0o600)
	if err := os.Symlink("data.txt", filepath.Join(scenarioDir, "inputs", "inside")); err != nil {
		t.Fatal(err)
	}

	ws, err := New("demo", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Cleanup()

	if err := ws.StageIncludes(scenarioDir, []string{"./inputs"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(ws.Dir, "inputs", "setup.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("setup mode = %o", info.Mode().Perm())
	}
	if _, err := os.Lstat(filepath.Join(ws.Dir, "inputs", "inside")); err != nil {
		t.Fatalf("inside symlink not staged: %v", err)
	}

	outside := filepath.Join(t.TempDir(), "secret")
	writeFile(t, outside, "secret", 0o600)
	writeFile(t, filepath.Join(scenarioDir, "inputs", "bad.sh"), "#!/bin/bash\n", 0o755)
	if err := os.Symlink(outside, filepath.Join(scenarioDir, "inputs", "escape")); err != nil {
		t.Fatal(err)
	}
	if err := ws.StageIncludes(scenarioDir, []string{"./inputs"}); err == nil {
		t.Fatal("expected escaping symlink rejection")
	}
}

func TestInstallUnitsSelectsAndInstallsAll(t *testing.T) {
	pluginDir := t.TempDir()
	writeFile(t, filepath.Join(pluginDir, "skills", "alpha", "SKILL.md"), "alpha", 0o644)
	writeFile(t, filepath.Join(pluginDir, "skills", "beta", "SKILL.md"), "beta", 0o644)
	writeFile(t, filepath.Join(pluginDir, "commands", "greet.md"), "greet", 0o644)

	ws, err := New("demo", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Cleanup()
	if err := ws.InstallUnits(pluginDir, "demo", ".claude/skills", []string{"skills/alpha", "commands/greet"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(ws.Dir, ".claude", "skills", "alpha", "SKILL.md")); err != nil {
		t.Fatalf("alpha not installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ws.Dir, ".claude", "skills", "beta")); err == nil {
		t.Fatal("beta should not be installed")
	}
	if _, err := os.Stat(filepath.Join(ws.Dir, ".claude", "commands", "demo", "greet.md")); err != nil {
		t.Fatalf("command not installed: %v", err)
	}

	ws2, err := New("demo", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws2.Cleanup()
	if err := ws2.InstallUnits(pluginDir, "demo", ".claude/skills", nil); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		".claude/skills/alpha/SKILL.md",
		".claude/skills/beta/SKILL.md",
		".claude/commands/demo/greet.md",
	} {
		if _, err := os.Stat(filepath.Join(ws2.Dir, filepath.FromSlash(path))); err != nil {
			t.Fatalf("%s not installed: %v", path, err)
		}
	}

	if err := ValidateSkillDir("/abs"); err == nil {
		t.Fatal("absolute skill dir accepted")
	}
	if err := ValidateSkillDir("../escape"); err == nil {
		t.Fatal("escaping skill dir accepted")
	}
}

func TestStageHookSourceWithAndWithout(t *testing.T) {
	pluginDir := t.TempDir()
	writeFile(t, filepath.Join(pluginDir, "hooks", "hook.sh"), "#!/bin/bash\necho with\n", 0o755)
	writeFile(t, filepath.Join(pluginDir, "security-patterns.conf"), "pattern", 0o600)

	ws, err := New("demo", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Cleanup()
	if err := ws.StageHookSource(pluginDir, "with"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(ws.Dir, ".eval-plugin", "hooks", "hook.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "echo with") {
		t.Fatalf("with hook = %q", data)
	}
	if got := ws.HookSource(); got != filepath.Join(ws.Dir, ".eval-plugin") {
		t.Fatalf("hook source = %q", got)
	}

	if err := ws.StageHookSource(pluginDir, "without"); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(ws.Dir, ".eval-plugin", "hooks", "hook.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Disabled control hook") {
		t.Fatalf("without hook = %q", data)
	}
}

func TestSnapshotAndArchiveExcludeHome(t *testing.T) {
	ws, err := New("demo", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Cleanup()
	writeFile(t, filepath.Join(ws.Dir, "a.txt"), "a", 0o600)
	writeFile(t, filepath.Join(ws.Dir, "nested", "b.txt"), "b", 0o600)
	writeFile(t, filepath.Join(ws.Home, "secret.txt"), "secret", 0o600)

	snapshot := filepath.Join(t.TempDir(), "before.txt")
	if err := ws.Snapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "./a.txt") || !strings.Contains(string(data), "./nested/b.txt") {
		t.Fatalf("snapshot = %s", data)
	}
	if strings.Contains(string(data), "secret.txt") {
		t.Fatalf("snapshot leaked home: %s", data)
	}

	archive := filepath.Join(t.TempDir(), "snapshot.tar")
	if err := ws.Archive(archive, 1<<20); err != nil {
		t.Fatal(err)
	}
	members := tarMembers(t, archive)
	if !members["./a.txt"] || !members["./nested/b.txt"] {
		t.Fatalf("archive members = %v", members)
	}
	for name := range members {
		if strings.HasPrefix(name, "./home") {
			t.Fatalf("archive includes home member %q", name)
		}
	}

	if err := ws.Archive(filepath.Join(t.TempDir(), "small.tar"), 1); err == nil {
		t.Fatal("expected archive size failure")
	}
}

func tarMembers(t *testing.T, path string) map[string]bool {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	members := map[string]bool{}
	reader := tar.NewReader(file)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		members[header.Name] = true
	}
	return members
}

func TestRunSetupUsesWorkspaceHome(t *testing.T) {
	realHome := t.TempDir()
	t.Setenv("HOME", realHome)

	scenarioDir := t.TempDir()
	script := filepath.Join(scenarioDir, "setup.sh")
	writeFile(t, script, "#!/usr/bin/env bash\nset -euo pipefail\ntouch \"$HOME/marker\"\necho \"$EVAL_WORKSPACE\" > workspace.txt\n", 0o755)

	ws, err := New("demo", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Cleanup()
	logPath := filepath.Join(t.TempDir(), "setup.log")
	result, err := ws.RunSetup(context.Background(), scenarioDir, t.TempDir(), script, logPath, 5*time.Second)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("setup = %+v, err=%v", result, err)
	}
	if _, err := os.Stat(filepath.Join(ws.Home, "marker")); err != nil {
		t.Fatalf("setup did not write to workspace home: %v", err)
	}
	if _, err := os.Stat(filepath.Join(realHome, "marker")); err == nil {
		t.Fatal("setup wrote to the real home")
	}
	if _, err := os.Stat(filepath.Join(ws.Dir, "workspace.txt")); err != nil {
		t.Fatalf("EVAL_WORKSPACE was not the workspace: %v", err)
	}
}

// TestEnvironmentPassEnvOverridesCredentialDenyList pins the workspace side of
// the precedence rule: --pass-env wins over the credential deny list for the
// setup and checks environments, while non-opted-in credentials stay stripped.
func TestEnvironmentPassEnvOverridesCredentialDenyList(t *testing.T) {
	t.Setenv("OPENCODE_API_KEY", "sk-workspace-secret")
	t.Setenv("GITHUB_TOKEN", "ghp-workspace-secret")
	t.Setenv("EVALCTL_SESSION", "workspace-session")

	without, err := New("demo", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer without.Cleanup()

	checks := without.ChecksEnvironment("/scenario", "/plugin", "criteria")
	for _, name := range []string{"OPENCODE_API_KEY", "GITHUB_TOKEN", "EVALCTL_SESSION"} {
		if hasEnvEntry(checks, name+"=") {
			t.Fatalf("%s leaked into the checks environment:\n%v", name, checks)
		}
	}
	for _, want := range []string{
		"CRITERIA=criteria",
		"HOME=" + without.Home,
		"TMPDIR=" + without.TempDir,
		"EVAL_WORKSPACE=" + without.Dir,
		"SANDBOX=" + without.Dir,
	} {
		if !hasEnvEntry(checks, want) {
			t.Fatalf("%s missing from checks environment:\n%v", want, checks)
		}
	}

	with, err := New("demo", []string{"OPENCODE_API_KEY"})
	if err != nil {
		t.Fatal(err)
	}
	defer with.Cleanup()

	checks = with.ChecksEnvironment("/scenario", "/plugin", "criteria")
	if !hasEnvEntry(checks, "OPENCODE_API_KEY=sk-workspace-secret") {
		t.Fatalf("opted-in key missing from checks environment:\n%v", checks)
	}
	for _, name := range []string{"GITHUB_TOKEN", "EVALCTL_SESSION"} {
		if hasEnvEntry(checks, name+"=") {
			t.Fatalf("%s leaked into the checks environment:\n%v", name, checks)
		}
	}
	if !sort.StringsAreSorted(checks) {
		t.Fatalf("checks environment is not sorted:\n%v", checks)
	}

	setup := with.SetupEnvironment("/scenario", "/plugin")
	if !hasEnvEntry(setup, "OPENCODE_API_KEY=sk-workspace-secret") {
		t.Fatalf("opted-in key missing from setup environment:\n%v", setup)
	}
	if hasEnvEntry(setup, "CRITERIA=") {
		t.Fatalf("setup environment must not carry CRITERIA:\n%v", setup)
	}
}

func hasEnvEntry(env []string, entry string) bool {
	for _, line := range env {
		if strings.HasPrefix(line, entry) {
			return true
		}
	}
	return false
}
