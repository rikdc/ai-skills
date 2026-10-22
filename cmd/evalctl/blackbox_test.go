package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

var (
	binaryOnce sync.Once
	binaryPath string
	binaryErr  error
)

func evalctlBinary(t *testing.T) string {
	t.Helper()
	binaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "evalctl-blackbox-")
		if err != nil {
			binaryErr = err
			return
		}
		binaryPath = filepath.Join(dir, "evalctl")
		build := exec.Command("go", "build", "-o", binaryPath, "./cmd/evalctl")
		build.Dir = filepath.Join("..", "..")
		output, err := build.CombinedOutput()
		if err != nil {
			binaryErr = &buildError{err: err, output: string(output)}
		}
	})
	if binaryErr != nil {
		t.Fatalf("build evalctl: %v", binaryErr)
	}
	return binaryPath
}

type buildError struct {
	err    error
	output string
}

func (e *buildError) Error() string { return e.err.Error() + ": " + e.output }

func runBinary(t *testing.T, dir string, env []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(evalctlBinary(t), args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !asExitError(err, &exitErr) {
			t.Fatalf("run %v: %v", args, err)
		}
		code = exitErr.ExitCode()
	}
	return stdout.String(), stderr.String(), code
}

func asExitError(err error, target **exec.ExitError) bool {
	if exitErr, ok := err.(*exec.ExitError); ok {
		*target = exitErr
		return true
	}
	return false
}

func blackBoxRepo(t *testing.T) (root string, marker string) {
	t.Helper()
	root = t.TempDir()
	marker = filepath.Join(t.TempDir(), "setup-done")
	write := func(path, content string, mode os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "tessl.json"), `{"name":"blackbox/repo"}`, 0o644)
	write(filepath.Join(root, "plugins", "demo", "skills", "demo", "SKILL.md"),
		"---\nname: demo\ndescription: blackbox demo skill with enough words\n---\n", 0o644)
	dir := filepath.Join(root, "plugins", "demo", "evals", "basic")
	write(filepath.Join(dir, "scenario.json"),
		`{"description":"blackbox","include":["./inputs"],"unit":"skills/demo"}`, 0o644)
	write(filepath.Join(dir, "task.md"), "# Task\n\nDo the thing.\n", 0o644)
	write(filepath.Join(dir, "criteria.json"),
		`{"context":"c","type":"weighted_checklist","checklist":[{"name":"a","max_score":10}]}`, 0o644)
	write(filepath.Join(dir, "inputs", "setup.sh"),
		"#!/usr/bin/env bash\nset -euo pipefail\nprintf done > "+marker+"\n", 0o755)
	return root, marker
}

func TestBlackBoxExitCodes(t *testing.T) {
	root, _ := blackBoxRepo(t)
	home := t.TempDir()
	env := []string{"HOME=" + home, "TMPDIR=" + t.TempDir(), "NO_COLOR=1"}

	if _, _, code := runBinary(t, root, env, "version"); code != 0 {
		t.Fatalf("version exit = %d, want 0", code)
	}
	_, stderr, code := runBinary(t, root, env, "run")
	if code != 2 || !strings.Contains(stderr, "--agent-cmd is required") {
		t.Fatalf("run exit = %d stderr=%q, want 2", code, stderr)
	}
	_, stderr, code = runBinary(t, root, env, "run", "--plugin", "demo", "--scenario", "basic", "--backend", "openai", "--agent-cmd", "true")
	if code != 2 || !strings.Contains(stderr, "milestone B") {
		t.Fatalf("openai exit = %d stderr=%q, want 2", code, stderr)
	}
	_, stderr, code = runBinary(t, root, env, "report", filepath.Join(root, "missing"))
	if code != 1 || !strings.Contains(stderr, "no such suite directory") {
		t.Fatalf("report exit = %d stderr=%q, want 1", code, stderr)
	}
	_, stderr, code = runBinary(t, root, env, "--nope", "version")
	if code != 2 || !strings.Contains(stderr, "unknown flag") {
		t.Fatalf("unknown flag exit = %d stderr=%q, want 2", code, stderr)
	}
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"list", "--shard", "2/1"}, "between 1 and 1"},
		{[]string{"list", "--unit", "skills/nope"}, "unknown --unit"},
		{[]string{"list", "--changed", "missing-ref"}, "needs a git checkout"},
		{[]string{"plan", "--arm", "sideways"}, "--arm must be"},
	} {
		_, stderr, code := runBinary(t, root, env, test.args...)
		if code != 2 || !strings.Contains(stderr, test.want) {
			t.Fatalf("%v exit = %d stderr=%q, want 2 with %q", test.args, code, stderr, test.want)
		}
	}
}

func TestBlackBoxRunDoesNotWriteHOME(t *testing.T) {
	root, _ := blackBoxRepo(t)
	home := t.TempDir()
	out := filepath.Join(t.TempDir(), "results")
	env := []string{"HOME=" + home, "TMPDIR=" + t.TempDir(), "NO_COLOR=1"}
	stdout, stderr, code := runBinary(t, root, env, "run",
		"--plugin", "demo", "--scenario", "basic", "--arm", "with",
		"--agent-cmd", "true", "--out", out)
	if code != 0 {
		t.Fatalf("run exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("run wrote to HOME: %v", entries)
	}
	if _, err := os.Stat(filepath.Join(out, "demo", "basic", "with", "run-1", "result.json")); err != nil {
		t.Fatalf("run did not write result.json: %v", err)
	}
}

func TestBlackBoxDoctorDoesNotLeakSecrets(t *testing.T) {
	root, _ := blackBoxRepo(t)
	env := []string{"EVALCTL_API_KEY=super-secret-token", "OPENAI_API_KEY=another-secret"}
	stdout, stderr, code := runBinary(t, root, env, "doctor")
	if code != 0 {
		t.Fatalf("doctor exit = %d stderr=%q", code, stderr)
	}
	if strings.Contains(stdout, "super-secret-token") || strings.Contains(stdout, "another-secret") {
		t.Fatalf("doctor leaked a secret: %q", stdout)
	}
	if !strings.Contains(stdout, "EVALCTL_API_KEY=set") {
		t.Fatalf("doctor did not report key presence: %q", stdout)
	}
}

func TestBlackBoxSuiteJudgeReceivesCredentials(t *testing.T) {
	root, _ := blackBoxRepo(t)
	home := t.TempDir()
	env := []string{"HOME=" + home, "TMPDIR=" + t.TempDir(), "NO_COLOR=1"}
	// A fake CLI credential that --auth-cli must copy into the judge's own home.
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"fake":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "results")
	// The judge must see the copied credential in its HOME or it fails loudly.
	judgeCmd := `sh -c 'test -f "$HOME/.claude.json" || { echo judge-has-no-credentials >&2; exit 9; }; printf "[{\"name\":\"a\",\"score\":10,\"max\":10}]" > judgement.json'`
	stdout, stderr, code := runBinary(t, root, env, "suite",
		"--plugin", "demo", "--scenario", "basic", "--arm", "with",
		"--agent-cmd", "true", "--judge-cmd", judgeCmd,
		"--auth-cli", "--runs", "1", "--out", out)
	if code != 0 {
		t.Fatalf("suite exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	data, err := os.ReadFile(filepath.Join(out, "suite.json"))
	if err != nil {
		t.Fatalf("read suite.json: %v", err)
	}
	var doc struct {
		Counts map[string]int `json:"counts"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse suite.json: %v", err)
	}
	if got := doc.Counts["judged_runs"]; got != 1 {
		t.Fatalf("judged_runs = %d, want 1 (judge did not see credentials?)\nsuite.json=%s", got, data)
	}
	if got := doc.Counts["pending_runs"]; got != 0 {
		t.Fatalf("pending_runs = %d, want 0\nsuite.json=%s", got, data)
	}
	if got := doc.Counts["judge_attempts"]; got != 1 {
		t.Fatalf("judge_attempts = %d, want 1 (inline grading should satisfy final grading)\nsuite.json=%s", got, data)
	}

	results, err := os.ReadFile(filepath.Join(out, "results.jsonl"))
	if err != nil {
		t.Fatalf("read results.jsonl: %v", err)
	}
	if strings.Contains(string(results), `"status":"pending"`) {
		t.Fatalf("results.jsonl still reports a pending scenario:\n%s", results)
	}
	if !strings.Contains(string(results), `"judge_attempt":"judge-`) {
		t.Fatalf("results.jsonl has no accepted judge attempt:\n%s", results)
	}
}

func TestBlackBoxSignals(t *testing.T) {
	for _, test := range []struct {
		name   string
		signal os.Signal
		want   int
	}{
		{"interrupt", os.Interrupt, 130},
		{"terminate", syscall.SIGTERM, 143},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, marker := blackBoxRepo(t)
			cmd := exec.Command(evalctlBinary(t), "run",
				"--plugin", "demo", "--scenario", "basic", "--arm", "with",
				"--agent-cmd", "sleep 30", "--out", filepath.Join(t.TempDir(), "out"))
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "TMPDIR="+t.TempDir())
			var output bytes.Buffer
			cmd.Stdout = &output
			cmd.Stderr = &output
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(15 * time.Second)
			for {
				if _, err := os.Stat(marker); err == nil {
					break
				}
				if time.Now().After(deadline) {
					_ = cmd.Process.Kill()
					t.Fatalf("setup did not complete; output: %s", output.String())
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err := cmd.Process.Signal(test.signal); err != nil {
				t.Fatal(err)
			}
			err := cmd.Wait()
			exitErr, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("wait = %v, want exit %d", err, test.want)
			}
			if code := exitErr.ExitCode(); code != test.want {
				t.Fatalf("exit = %d, want %d (output: %s)", code, test.want, output.String())
			}
		})
	}
}
