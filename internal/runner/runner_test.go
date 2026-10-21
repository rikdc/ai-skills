package runner

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/platform/errs"
	"github.com/rikdc/ai-skills/internal/provider/command"
	"github.com/rikdc/ai-skills/internal/runner/attempts"
)

const defaultCriteria = `{
  "context": "test rubric",
  "type": "weighted_checklist",
  "checklist": [
    {"name": "One", "description": "first", "max_score": 10},
    {"name": "Two", "description": "second", "max_score": 10}
  ]
}`

type fixture struct {
	root     string
	plugin   string
	scenario string
}

type fixtureOptions struct {
	mode       model.Mode
	units      []string
	execution  *model.Execution
	setup      string
	checks     string
	skillDir   string
	commandDir string
}

func newFixture(t *testing.T, opts fixtureOptions) *fixture {
	t.Helper()
	if opts.mode == "" {
		opts.mode = model.ModeComparison
	}
	if len(opts.units) == 0 {
		opts.units = []string{"skills/good"}
	}
	if opts.setup == "" {
		opts.setup = "#!/usr/bin/env bash\nset -euo pipefail\ntouch \"$HOME/setup-leak\"\necho setup ok\n"
	}
	fx := &fixture{root: t.TempDir(), plugin: "demo", scenario: "basic"}
	pluginDir := filepath.Join(fx.root, "plugins", fx.plugin)
	writeFixtureFile(t, filepath.Join(pluginDir, "skills", "good", "SKILL.md"), "---\nname: good\n---\nbody\n", 0o644)
	writeFixtureFile(t, filepath.Join(pluginDir, "commands", "greet.md"), "# Greet\n", 0o644)
	writeFixtureFile(t, filepath.Join(pluginDir, "hooks", "hook.sh"), "#!/usr/bin/env bash\necho with-hook\n", 0o755)
	writeFixtureFile(t, filepath.Join(pluginDir, "security-patterns.conf"), "secret\n", 0o600)

	scenarioDir := filepath.Join(pluginDir, "evals", fx.scenario)
	writeFixtureFile(t, filepath.Join(scenarioDir, "task.md"), "Do the task.\n", 0o644)
	writeFixtureFile(t, filepath.Join(scenarioDir, "criteria.json"), defaultCriteria, 0o644)
	writeFixtureFile(t, filepath.Join(scenarioDir, "inputs", "setup.sh"), opts.setup, 0o755)
	if opts.checks != "" {
		writeFixtureFile(t, filepath.Join(scenarioDir, "checks.sh"), opts.checks, 0o755)
	}

	scenarioJSON := map[string]any{
		"description": "fixture scenario",
		"include":     []string{"./inputs"},
		"mode":        string(opts.mode),
	}
	if len(opts.units) == 1 {
		scenarioJSON["unit"] = opts.units[0]
	} else {
		scenarioJSON["units"] = opts.units
	}
	if opts.execution != nil {
		scenarioJSON["execution"] = opts.execution
	}
	data, err := json.MarshalIndent(scenarioJSON, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, filepath.Join(scenarioDir, "scenario.json"), string(data)+"\n", 0o644)
	return fx
}

func writeFixtureFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

type runOptions struct {
	arm              string
	runs             int
	units            []string
	out              string
	timeout          time.Duration
	keep             bool
	command          string
	shellPath        string
	hooks            attempts.Hooks
	maxSize          int64
	attemptArtifacts string
	logger           *slog.Logger
}

func execute(t *testing.T, fx *fixture, opts runOptions) (*Summary, error) {
	t.Helper()
	return executeCtx(t, fx, opts, context.Background())
}

func executeCtx(t *testing.T, fx *fixture, opts runOptions, ctx context.Context) (*Summary, error) {
	t.Helper()
	if opts.arm == "" {
		opts.arm = "with"
	}
	if opts.runs == 0 {
		opts.runs = 1
	}
	if opts.timeout == 0 {
		opts.timeout = 10 * time.Second
	}
	if opts.command == "" {
		opts.command = "true"
	}
	if opts.out == "" {
		opts.out = t.TempDir()
	}
	exec, err := command.New(command.Config{Command: opts.command, ShellPath: opts.shellPath, Timeout: opts.timeout})
	if err != nil {
		t.Fatal(err)
	}
	r, err := New(Config{
		Root:             fx.root,
		Plugin:           fx.plugin,
		Scenario:         fx.scenario,
		Arm:              opts.arm,
		Runs:             opts.runs,
		Units:            opts.units,
		Out:              opts.out,
		Timeout:          opts.timeout,
		SetupTimeout:     10 * time.Second,
		ChecksTimeout:    10 * time.Second,
		Keep:             opts.keep,
		MaxSnapshotBytes: opts.maxSize,
		AttemptArtifacts: opts.attemptArtifacts,
		StoreHooks:       opts.hooks,
		Executor:         exec,
		Logger:           opts.logger,
	})
	if err != nil {
		return nil, err
	}
	return r.Run(ctx)
}

func runPath(out, arm string, run int) string {
	return filepath.Join(out, "demo", "basic", arm, "run-"+itoa(run))
}

func itoa(n int) string { return strconv.Itoa(n) }

func readJSON[T any](t *testing.T, path string) T {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return value
}

func tarMemberSet(t *testing.T, path string) map[string]bool {
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
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		members[header.Name] = true
	}
	return members
}

func TestRunOKWithChecksSelftestEquivalent(t *testing.T) {
	realHome := t.TempDir()
	t.Setenv("HOME", realHome)
	fx := newFixture(t, fixtureOptions{
		checks: `#!/usr/bin/env bash
set -euo pipefail
printf '[{"name":"One","score":7,"max":10,"note":"ok"},{"name":"Two","score":3,"max":10}]'
`,
	})
	out := t.TempDir()
	summary, err := execute(t, fx, runOptions{out: out, command: `touch "$HOME/agent-leak"; true`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(summary.Runs) != 1 || summary.Runs[0].Status != model.StatusOK {
		t.Fatalf("summary = %+v", summary)
	}
	dir := runPath(out, "with", 1)

	result := readJSON[model.RunResult](t, filepath.Join(dir, "result.json"))
	if result.Status != model.StatusOK || !result.Complete {
		t.Fatalf("result = %+v", result)
	}
	if result.Mechanical.Score != 10 || result.Mechanical.Max != 20 {
		t.Fatalf("mechanical = %+v", result.Mechanical)
	}
	if result.AgentAttempt != "agent-000001" || result.EvidenceSHA256 == "" {
		t.Fatalf("result binding = %+v", result)
	}
	if result.AgentExit == nil || *result.AgentExit != 0 || result.TimedOut || result.DurationSeconds == nil {
		t.Fatalf("result observability = %+v", result)
	}
	finished := readJSON[model.FinishedAttempt](t, filepath.Join(dir, "attempts", "agent-000001", "finished.json"))
	if finished.AgentExit == nil || *finished.AgentExit != 0 || finished.TimedOut || finished.DurationSeconds == nil {
		t.Fatalf("finished observability = %+v", finished)
	}

	request := readJSON[model.GradingRequest](t, filepath.Join(dir, "grading-request.json"))
	if len(request.CoveredByChecks) != 2 || len(request.JudgementPending) != 0 {
		t.Fatalf("grading request = %+v", request)
	}
	if request.AgentAttempt != "agent-000001" || request.EvidenceSHA256 != result.EvidenceSHA256 {
		t.Fatalf("request binding = %+v", request)
	}

	current := readJSON[model.CurrentPointer](t, filepath.Join(dir, "current.json"))
	if current.AgentAttempt != "agent-000001" || current.AgentEvidenceSHA256 != result.EvidenceSHA256 {
		t.Fatalf("current = %+v", current)
	}
	if _, err := os.Stat(filepath.Join(dir, "attempts", "agent-000001", "evidence.json")); err != nil {
		t.Fatalf("evidence manifest: %v", err)
	}
	usage := readJSON[model.Usage](t, filepath.Join(dir, "attempts", "agent-000001", "usage.json"))
	if usage.State != model.UsageUnknown {
		t.Fatalf("usage = %+v", usage)
	}
	members := tarMemberSet(t, filepath.Join(dir, "attempts", "agent-000001", "snapshot.tar"))
	if !members["./.claude/skills/good/SKILL.md"] {
		t.Fatalf("with arm missing skill: %v", members)
	}
	for name := range members {
		if strings.HasPrefix(name, "./home") {
			t.Fatalf("archive contains home: %q", name)
		}
	}
	entries, err := os.ReadDir(realHome)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("real HOME was written: %v", entries)
	}
}

func TestRunArmsAndUnitSelection(t *testing.T) {
	fx := newFixture(t, fixtureOptions{units: []string{"skills/good", "commands/greet"}})
	out := t.TempDir()
	summary, err := execute(t, fx, runOptions{out: out, arm: "both"})
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Runs) != 2 {
		t.Fatalf("runs = %+v", summary.Runs)
	}
	if summary.Runs[0].Arm != "without" || summary.Runs[1].Arm != "with" {
		t.Fatalf("arm order = %+v", summary.Runs)
	}
	withMembers := tarMemberSet(t, filepath.Join(runPath(out, "with", 1), "attempts", "agent-000001", "snapshot.tar"))
	withoutMembers := tarMemberSet(t, filepath.Join(runPath(out, "without", 1), "attempts", "agent-000001", "snapshot.tar"))
	if !withMembers["./.claude/skills/good/SKILL.md"] || !withMembers["./.claude/commands/demo/greet.md"] {
		t.Fatalf("with members = %v", withMembers)
	}
	for name := range withoutMembers {
		if strings.Contains(name, ".claude/skills") || strings.Contains(name, ".claude/commands") {
			t.Fatalf("without arm installed units: %q", name)
		}
	}

	out2 := t.TempDir()
	if _, err := execute(t, fx, runOptions{out: out2, units: []string{"commands/greet"}}); err != nil {
		t.Fatal(err)
	}
	selected := tarMemberSet(t, filepath.Join(runPath(out2, "with", 1), "attempts", "agent-000001", "snapshot.tar"))
	if !selected["./.claude/commands/demo/greet.md"] {
		t.Fatalf("selected command missing: %v", selected)
	}
	if selected["./.claude/skills/good/SKILL.md"] {
		t.Fatalf("unselected skill installed: %v", selected)
	}
}

func TestRunSetupFailure(t *testing.T) {
	fx := newFixture(t, fixtureOptions{setup: "#!/usr/bin/env bash\nset -euo pipefail\nexit 3\n"})
	out := t.TempDir()
	summary, err := execute(t, fx, runOptions{out: out})
	if err == nil || errs.Code(err) != 1 {
		t.Fatalf("err = %v", err)
	}
	if summary.Runs[0].Status != model.StatusSetupFailed {
		t.Fatalf("outcome = %+v", summary.Runs[0])
	}
	dir := runPath(out, "with", 1)
	result := readJSON[model.RunResult](t, filepath.Join(dir, "result.json"))
	if result.Status != model.StatusSetupFailed || result.Complete {
		t.Fatalf("result = %+v", result)
	}
	attemptDir := filepath.Join(dir, "attempts", "agent-000001")
	if _, err := os.Stat(filepath.Join(attemptDir, "evidence.json")); err == nil {
		t.Fatal("setup failure wrote evidence")
	}
	if _, err := os.Stat(filepath.Join(dir, "grading-request.json")); err == nil {
		t.Fatal("setup failure wrote a grading request")
	}
	if _, err := os.Stat(filepath.Join(dir, "current.json")); err == nil {
		t.Fatal("setup failure was accepted")
	}
	usage := readJSON[model.Usage](t, filepath.Join(attemptDir, "usage.json"))
	if usage.State != model.UsageNotApplicable {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestRunAgentFailure(t *testing.T) {
	fx := newFixture(t, fixtureOptions{})
	out := t.TempDir()
	summary, err := execute(t, fx, runOptions{out: out, command: "exit 7"})
	if err == nil {
		t.Fatal("expected operational error")
	}
	if summary.Runs[0].Status != model.StatusAgentFailed {
		t.Fatalf("outcome = %+v", summary.Runs[0])
	}
	dir := runPath(out, "with", 1)
	attemptDir := filepath.Join(dir, "attempts", "agent-000001")
	if _, err := os.Stat(filepath.Join(attemptDir, "evidence.json")); err != nil {
		t.Fatalf("failed agent missing evidence: %v", err)
	}
	request := readJSON[model.GradingRequest](t, filepath.Join(dir, "grading-request.json"))
	if len(request.JudgementPending) != 2 {
		t.Fatalf("request = %+v", request)
	}
	if _, err := os.Stat(filepath.Join(dir, "current.json")); err == nil {
		t.Fatal("failed agent was accepted")
	}
	result := readJSON[model.RunResult](t, filepath.Join(dir, "result.json"))
	if result.Status != model.StatusAgentFailed || result.Complete {
		t.Fatalf("result = %+v", result)
	}
	if result.AgentExit == nil || *result.AgentExit != 7 || result.TimedOut || result.DurationSeconds == nil {
		t.Fatalf("result observability = %+v", result)
	}
	usage := readJSON[model.Usage](t, filepath.Join(attemptDir, "usage.json"))
	if usage.State != model.UsageUnknown {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestRunTimeout(t *testing.T) {
	fx := newFixture(t, fixtureOptions{})
	start := time.Now()
	summary, err := execute(t, fx, runOptions{out: t.TempDir(), command: "sleep 5", timeout: 200 * time.Millisecond})
	if err == nil {
		t.Fatal("expected operational error")
	}
	if summary.Runs[0].Status != model.StatusTimedOut {
		t.Fatalf("outcome = %+v", summary.Runs[0])
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("timeout took %v", elapsed)
	}
	result := readJSON[model.RunResult](t, filepath.Join(summary.Runs[0].Dir, "result.json"))
	if !result.TimedOut || result.Status != model.StatusTimedOut {
		t.Fatalf("result = %+v", result)
	}
	if result.AgentExit == nil || result.DurationSeconds == nil {
		t.Fatalf("result observability = %+v", result)
	}
}

func TestRunCommandStartFailure(t *testing.T) {
	fx := newFixture(t, fixtureOptions{})
	out := t.TempDir()
	summary, err := execute(t, fx, runOptions{out: out, shellPath: filepath.Join(t.TempDir(), "missing-shell")})
	if err == nil {
		t.Fatal("expected operational error")
	}
	if summary.Runs[0].Status != model.StatusAgentFailed {
		t.Fatalf("outcome = %+v", summary.Runs[0])
	}
	result := readJSON[model.RunResult](t, filepath.Join(summary.Runs[0].Dir, "result.json"))
	if result.Status != model.StatusAgentFailed || result.AgentExit != nil {
		t.Fatalf("result = %+v", result)
	}
	if result.DurationSeconds == nil || result.TimedOut {
		t.Fatalf("result observability = %+v", result)
	}
}

func TestRunCancellation(t *testing.T) {
	fx := newFixture(t, fixtureOptions{})
	out := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	summary, err := executeCtx(t, fx, runOptions{out: out, command: "sleep 5", timeout: 10 * time.Second}, ctx)
	if err == nil {
		t.Fatal("expected operational error")
	}
	if summary.Runs[0].Status != model.StatusCancelled {
		t.Fatalf("outcome = %+v", summary.Runs[0])
	}
	result := readJSON[model.RunResult](t, filepath.Join(summary.Runs[0].Dir, "result.json"))
	if result.Status != model.StatusCancelled || result.Complete {
		t.Fatalf("result = %+v", result)
	}
}

func TestRunStopsStartingRunsAfterCancellation(t *testing.T) {
	fx := newFixture(t, fixtureOptions{})
	out := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	summary, err := executeCtx(t, fx, runOptions{
		out: out, arm: "both", runs: 3, command: "sleep 5", timeout: 10 * time.Second,
	}, ctx)
	if err == nil {
		t.Fatal("expected operational error")
	}
	if len(summary.Runs) != 1 || summary.Runs[0].Status != model.StatusCancelled {
		t.Fatalf("runs after cancellation = %+v, want only the cancelled one", summary.Runs)
	}
	for _, arm := range []string{"without", "with"} {
		for run := 1; run <= 3; run++ {
			dir := runPath(out, arm, run)
			if dir == summary.Runs[0].Dir {
				continue
			}
			if _, err := os.Stat(dir); err == nil {
				t.Errorf("%s/run-%d was started after cancellation", arm, run)
			}
		}
	}
}

func TestRunAlreadyCancelledStartsNothing(t *testing.T) {
	fx := newFixture(t, fixtureOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	summary, err := executeCtx(t, fx, runOptions{out: t.TempDir(), runs: 2}, ctx)
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("err = %v, want cancelled", err)
	}
	if len(summary.Runs) != 0 {
		t.Fatalf("runs = %+v, want none", summary.Runs)
	}
}

func TestRunEvidenceFailure(t *testing.T) {
	fx := newFixture(t, fixtureOptions{})
	out := t.TempDir()
	summary, err := execute(t, fx, runOptions{
		out:     out,
		command: `head -c 2000000 /dev/zero > big.bin`,
		maxSize: 1024,
	})
	if err == nil {
		t.Fatal("expected operational error")
	}
	if summary.Runs[0].Status != model.StatusEvidenceFailed {
		t.Fatalf("outcome = %+v", summary.Runs[0])
	}
	dir := runPath(out, "with", 1)
	if _, err := os.Stat(filepath.Join(dir, "grading-request.json")); err == nil {
		t.Fatal("evidence failure wrote a grading request")
	}
	if _, err := os.Stat(filepath.Join(dir, "attempts", "agent-000001", "evidence.json")); err == nil {
		t.Fatal("evidence failure wrote a manifest")
	}
	result := readJSON[model.RunResult](t, filepath.Join(dir, "result.json"))
	if result.Status != model.StatusEvidenceFailed || result.Complete {
		t.Fatalf("result = %+v", result)
	}
}

func TestRunChecksFailure(t *testing.T) {
	for name, checks := range map[string]string{
		"nonzero": "#!/usr/bin/env bash\nexit 4\n",
		"invalid": "#!/usr/bin/env bash\nprintf 'not json'\n",
		"unknown": "#!/usr/bin/env bash\nprintf '[{\"name\":\"Ghost\",\"score\":1,\"max\":10}]'\n",
		"bad-max": "#!/usr/bin/env bash\nprintf '[{\"name\":\"One\",\"score\":1,\"max\":5}]'\n",
	} {
		t.Run(name, func(t *testing.T) {
			fx := newFixture(t, fixtureOptions{checks: checks})
			out := t.TempDir()
			summary, err := execute(t, fx, runOptions{out: out})
			if err == nil {
				t.Fatal("expected operational error")
			}
			if summary.Runs[0].Status != model.StatusChecksFailed {
				t.Fatalf("outcome = %+v", summary.Runs[0])
			}
			dir := runPath(out, "with", 1)
			request := readJSON[model.GradingRequest](t, filepath.Join(dir, "grading-request.json"))
			if len(request.JudgementPending) != 2 {
				t.Fatalf("checks failure did not leave criteria pending: %+v", request)
			}
			if _, err := os.Stat(filepath.Join(dir, "attempts", "agent-000001", "mechanical.json")); err == nil {
				t.Fatal("invalid checks wrote mechanical.json")
			}
			if _, err := os.Stat(filepath.Join(dir, "current.json")); err == nil {
				t.Fatal("checks-failed was accepted")
			}
			result := readJSON[model.RunResult](t, filepath.Join(dir, "result.json"))
			if result.Status != model.StatusChecksFailed || result.Complete {
				t.Fatalf("result = %+v", result)
			}
		})
	}
}

func TestRunChecksFailureKeepsAgentFailure(t *testing.T) {
	fx := newFixture(t, fixtureOptions{checks: "#!/usr/bin/env bash\nexit 4\n"})
	out := t.TempDir()
	summary, err := execute(t, fx, runOptions{out: out, command: "exit 3"})
	if err == nil {
		t.Fatal("expected operational error")
	}
	if summary.Runs[0].Status != model.StatusAgentFailed {
		t.Fatalf("outcome = %+v, want agent_failed", summary.Runs[0])
	}
	result := readJSON[model.RunResult](t, filepath.Join(runPath(out, "with", 1), "result.json"))
	if result.Status != model.StatusAgentFailed || result.StopReason == model.StopChecksError {
		t.Fatalf("result = %+v", result)
	}
	if !strings.Contains(result.Error, "checks:") {
		t.Fatalf("error %q does not mention the checks failure", result.Error)
	}
}

func TestRunChecksFailureKeepsTimeout(t *testing.T) {
	fx := newFixture(t, fixtureOptions{checks: "#!/usr/bin/env bash\nexit 4\n"})
	out := t.TempDir()
	summary, err := execute(t, fx, runOptions{out: out, command: "sleep 5", timeout: 300 * time.Millisecond})
	if err == nil {
		t.Fatal("expected operational error")
	}
	if summary.Runs[0].Status != model.StatusTimedOut {
		t.Fatalf("outcome = %+v, want timed_out", summary.Runs[0])
	}
	result := readJSON[model.RunResult](t, filepath.Join(runPath(out, "with", 1), "result.json"))
	if !result.TimedOut || result.StopReason != model.StopTimeout {
		t.Fatalf("result = %+v", result)
	}
}

func TestInterruptedAttemptRestart(t *testing.T) {
	fx := newFixture(t, fixtureOptions{})
	out := t.TempDir()
	crash := errors.New("injected crash after allocation")
	_, err := execute(t, fx, runOptions{out: out, hooks: attempts.Hooks{
		AfterAllocate: func(string) error { return crash },
	}})
	if err == nil {
		t.Fatal("expected crash error")
	}
	dir := runPath(out, "with", 1)
	store := attempts.New(dir)
	all, err := store.Enumerate()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != "agent-000001" || !all[0].Interrupted() {
		t.Fatalf("attempts after crash = %+v", all)
	}

	summary, err := execute(t, fx, runOptions{out: out})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Runs[0].Status != model.StatusOK {
		t.Fatalf("restart outcome = %+v", summary.Runs[0])
	}
	current := readJSON[model.CurrentPointer](t, filepath.Join(dir, "current.json"))
	if current.AgentAttempt != "agent-000002" {
		t.Fatalf("current = %+v", current)
	}
	all, err = store.Enumerate()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || !all[0].Interrupted() || all[1].Status != model.StatusOK {
		t.Fatalf("attempts after restart = %+v", all)
	}
}

func TestFailedRerunKeepsAcceptedCurrent(t *testing.T) {
	fx := newFixture(t, fixtureOptions{})
	out := t.TempDir()
	if _, err := execute(t, fx, runOptions{out: out}); err != nil {
		t.Fatal(err)
	}
	dir := runPath(out, "with", 1)
	before, err := os.ReadFile(filepath.Join(dir, "current.json"))
	if err != nil {
		t.Fatal(err)
	}
	beforeResult, err := os.ReadFile(filepath.Join(dir, "result.json"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := execute(t, fx, runOptions{out: out, command: "exit 9"}); err == nil {
		t.Fatal("expected failed rerun")
	}
	after, err := os.ReadFile(filepath.Join(dir, "current.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("failed rerun replaced the accepted pointer")
	}
	afterResult, err := os.ReadFile(filepath.Join(dir, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeResult) != string(afterResult) {
		t.Fatal("failed rerun replaced the accepted result projection")
	}
	all, err := attempts.New(dir).Enumerate()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("attempts = %+v", all)
	}
}

func TestFunctionalHookScenarioStagesSource(t *testing.T) {
	fx := newFixture(t, fixtureOptions{
		mode:  model.ModeFunctional,
		units: []string{"hooks/hook"},
		setup: `#!/usr/bin/env bash
set -euo pipefail
[[ -f "$EVAL_HOOK_SOURCE/hooks/hook.sh" ]] || { echo "missing hook source" >&2; exit 1; }
mkdir -p hooks
cp "$EVAL_HOOK_SOURCE/hooks/hook.sh" hooks/hook.sh
echo staged
`,
	})
	out := t.TempDir()
	summary, err := execute(t, fx, runOptions{out: out, arm: "both"})
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Runs) != 1 || summary.Runs[0].Arm != "with" || summary.Runs[0].Status != model.StatusOK {
		t.Fatalf("summary = %+v", summary.Runs)
	}
	members := tarMemberSet(t, filepath.Join(runPath(out, "with", 1), "attempts", "agent-000001", "snapshot.tar"))
	if !members["./.eval-plugin/hooks/hook.sh"] {
		t.Fatalf("hook source not staged: %v", members)
	}
	if _, err := New(Config{
		Root: fx.root, Plugin: fx.plugin, Scenario: fx.scenario, Arm: "without",
		SkillDir: ".claude/skills", Out: out, Executor: mustExecutor(t),
	}); err == nil || errs.Code(err) != 2 {
		t.Fatalf("functional without arm error = %v", err)
	}
}

func mustExecutor(t *testing.T) *command.Executor {
	t.Helper()
	exec, err := command.New(command.Config{Command: "true"})
	if err != nil {
		t.Fatal(err)
	}
	return exec
}

func TestDirectHookScenarioRejected(t *testing.T) {
	fx := newFixture(t, fixtureOptions{
		mode:      model.ModeFunctional,
		units:     []string{"hooks/hook"},
		execution: &model.Execution{Backend: model.ExecutionHook, Cases: "hook-cases.json"},
	})
	_, err := New(Config{
		Root: fx.root, Plugin: fx.plugin, Scenario: fx.scenario,
		SkillDir: ".claude/skills", Out: t.TempDir(), Executor: mustExecutor(t),
	})
	if err == nil {
		t.Fatal("expected direct-hook rejection")
	}
	if errs.Code(err) != 2 || !strings.Contains(err.Error(), "milestone C") {
		t.Fatalf("error = %v (code %d)", err, errs.Code(err))
	}
}

func TestAttemptArtifactsRetention(t *testing.T) {
	for _, mode := range []string{"failed", "all", "none"} {
		t.Run(mode, func(t *testing.T) {
			fx := newFixture(t, fixtureOptions{})
			out := t.TempDir()
			if _, err := execute(t, fx, runOptions{out: out, attemptArtifacts: mode}); err != nil {
				t.Fatal(err)
			}
			if _, err := execute(t, fx, runOptions{out: out, attemptArtifacts: mode}); err != nil {
				t.Fatal(err)
			}
			if _, err := execute(t, fx, runOptions{out: out, command: "exit 9", attemptArtifacts: mode}); err == nil {
				t.Fatal("expected failed third run")
			}
			dir := runPath(out, "with", 1)
			store := attempts.New(dir)
			all, err := store.Enumerate()
			if err != nil {
				t.Fatal(err)
			}
			if len(all) != 3 {
				t.Fatalf("attempts = %d, want 3", len(all))
			}
			wantPruned := map[string]bool{}
			switch mode {
			case "all":
			case "failed":
				wantPruned["agent-000001"] = true
			case "none":
				wantPruned["agent-000001"] = true
				wantPruned["agent-000003"] = true
			}
			for _, attempt := range all {
				_, snapErr := os.Stat(filepath.Join(attempt.Dir, "snapshot.tar"))
				_, markerErr := os.Stat(filepath.Join(attempt.Dir, "evidence_pruned.json"))
				if wantPruned[attempt.ID] {
					if snapErr == nil {
						t.Fatalf("%s snapshot retained", attempt.ID)
					}
					if markerErr != nil {
						t.Fatalf("%s pruning marker missing: %v", attempt.ID, markerErr)
					}
					marker := readJSON[attempts.PrunedEvidence](t, filepath.Join(attempt.Dir, "evidence_pruned.json"))
					finished := readJSON[model.FinishedAttempt](t, filepath.Join(attempt.Dir, "finished.json"))
					if marker.EvidenceSHA256 != finished.EvidenceSHA256 {
						t.Fatalf("%s marker digest %q != %q", attempt.ID, marker.EvidenceSHA256, finished.EvidenceSHA256)
					}
				} else {
					if snapErr != nil {
						t.Fatalf("%s snapshot missing: %v", attempt.ID, snapErr)
					}
					if markerErr == nil {
						t.Fatalf("%s unexpectedly pruned", attempt.ID)
					}
				}
				if _, err := os.Stat(filepath.Join(attempt.Dir, "usage.json")); err != nil {
					t.Fatalf("%s usage removed: %v", attempt.ID, err)
				}
				if _, err := os.Stat(filepath.Join(attempt.Dir, "finished.json")); err != nil {
					t.Fatalf("%s finished removed: %v", attempt.ID, err)
				}
			}
			current := readJSON[model.CurrentPointer](t, filepath.Join(dir, "current.json"))
			if current.AgentAttempt != "agent-000002" {
				t.Fatalf("current = %+v", current)
			}
		})
	}
}

func TestNoChecksLeavesCriteriaPending(t *testing.T) {
	fx := newFixture(t, fixtureOptions{})
	out := t.TempDir()
	if _, err := execute(t, fx, runOptions{out: out}); err != nil {
		t.Fatal(err)
	}
	dir := runPath(out, "with", 1)
	request := readJSON[model.GradingRequest](t, filepath.Join(dir, "grading-request.json"))
	if len(request.CoveredByChecks) != 0 || len(request.JudgementPending) != 2 {
		t.Fatalf("request = %+v", request)
	}
	finished := readJSON[model.FinishedAttempt](t, filepath.Join(dir, "attempts", "agent-000001", "finished.json"))
	if finished.ChecksStatus != model.ChecksNotApplicable {
		t.Fatalf("checks status = %q", finished.ChecksStatus)
	}
}

// logBuffer collects progress logs. It is safe for the concurrent workers the
// suite uses.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func testLogger(buffer *logBuffer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(buffer, &slog.HandlerOptions{Level: level}))
}

// logFields parses one slog text line into its key=value fields, honouring
// quoted values that contain spaces.
func logFields(line string) map[string]string {
	fields := map[string]string{}
	var current strings.Builder
	inQuotes := false
	flush := func() {
		token := current.String()
		current.Reset()
		key, value, ok := strings.Cut(token, "=")
		if !ok {
			return
		}
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		fields[key] = value
	}
	remaining := line
	for len(remaining) > 0 {
		char := remaining[0]
		switch {
		case char == '"':
			inQuotes = !inQuotes
			current.WriteByte(char)
			remaining = remaining[1:]
		case char == '\\' && inQuotes && len(remaining) > 1:
			current.WriteByte(char)
			current.WriteByte(remaining[1])
			remaining = remaining[2:]
		case char == ' ' && !inQuotes:
			flush()
			remaining = remaining[1:]
		default:
			current.WriteByte(char)
			remaining = remaining[1:]
		}
	}
	flush()
	return fields
}

// findLog returns the fields of the first progress line whose msg and wanted
// attributes all match.
func findLog(t *testing.T, logged, msg string, wanted map[string]string) map[string]string {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(logged), "\n") {
		fields := logFields(line)
		if fields["msg"] != msg {
			continue
		}
		matched := true
		for key, value := range wanted {
			if fields[key] != value {
				matched = false
				break
			}
		}
		if matched {
			return fields
		}
	}
	t.Fatalf("no %q progress line matching %v in:\n%s", msg, wanted, logged)
	return nil
}

func countLog(logged, msg string) int {
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(logged), "\n") {
		if logFields(line)["msg"] == msg {
			count++
		}
	}
	return count
}

func logDuration(t *testing.T, fields map[string]string) int64 {
	t.Helper()
	value, ok := fields["duration_seconds"]
	if !ok {
		t.Fatalf("no duration_seconds attribute in %v", fields)
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		t.Fatalf("duration_seconds %q: %v", value, err)
	}
	if seconds < 0 {
		t.Fatalf("duration_seconds = %d, want a rounded whole number of seconds", seconds)
	}
	return seconds
}

func TestRunLogsStartedAndFinishedPair(t *testing.T) {
	fx := newFixture(t, fixtureOptions{})
	buffer := &logBuffer{}
	summary, err := execute(t, fx, runOptions{out: t.TempDir(), logger: testLogger(buffer, slog.LevelInfo)})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(summary.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(summary.Runs))
	}
	run := summary.Runs[0]
	logged := buffer.String()
	wanted := map[string]string{
		"plugin":   fx.plugin,
		"scenario": fx.scenario,
		"arm":      run.Arm,
		"run":      itoa(run.Run),
	}
	started := findLog(t, logged, "run started", wanted)
	if started["status"] != "" {
		t.Fatalf("run started carries status %q", started["status"])
	}
	if progress, ok := started["progress"]; ok {
		t.Fatalf("run started carries progress %q without a Progress function", progress)
	}
	finished := findLog(t, logged, "run finished", wanted)
	if finished["status"] != string(run.Status) {
		t.Fatalf("run finished status = %q, want %q", finished["status"], run.Status)
	}
	logDuration(t, finished)
	if message, ok := finished["error"]; ok {
		t.Fatalf("successful run logged error %q", message)
	}
	if strings.Index(logged, "run started") > strings.Index(logged, "run finished") {
		t.Fatalf("run finished preceded run started:\n%s", logged)
	}
	if countLog(logged, "run started") != 1 || countLog(logged, "run finished") != 1 {
		t.Fatalf("progress pair was not logged exactly once:\n%s", logged)
	}
}

func TestRunLogsFailureStatusAndError(t *testing.T) {
	fx := newFixture(t, fixtureOptions{})
	buffer := &logBuffer{}
	summary, err := execute(t, fx, runOptions{
		out:     t.TempDir(),
		command: "exit 7",
		logger:  testLogger(buffer, slog.LevelInfo),
	})
	if err == nil {
		t.Fatal("expected operational error")
	}
	if len(summary.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(summary.Runs))
	}
	run := summary.Runs[0]
	if run.Status == model.StatusOK || run.Error == "" {
		t.Fatalf("outcome = %+v", run)
	}
	finished := findLog(t, buffer.String(), "run finished", map[string]string{
		"plugin": fx.plugin, "scenario": fx.scenario, "arm": run.Arm, "run": itoa(run.Run),
	})
	if finished["status"] != string(run.Status) {
		t.Fatalf("run finished status = %q, want %q", finished["status"], run.Status)
	}
	if finished["error"] != run.Error {
		t.Fatalf("run finished error = %q, want %q", finished["error"], run.Error)
	}
	logDuration(t, finished)
}

func TestRunLogsSkippedRunsAtDebug(t *testing.T) {
	fx := newFixture(t, fixtureOptions{})
	out := t.TempDir()
	for _, tc := range []struct {
		name  string
		level slog.Level
		want  int
	}{
		{name: "info hides skipped runs", level: slog.LevelInfo, want: 0},
		{name: "debug shows skipped runs", level: slog.LevelDebug, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buffer := &logBuffer{}
			r, err := New(Config{
				Root:     fx.root,
				Plugin:   fx.plugin,
				Scenario: fx.scenario,
				Arm:      "with",
				Runs:     1,
				Out:      out,
				SkillDir: ".claude/skills",
				Executor: mustExecutor(t),
				Logger:   testLogger(buffer, tc.level),
				SkipRun:  func(string, int) bool { return true },
			})
			if err != nil {
				t.Fatal(err)
			}
			summary, err := r.Run(context.Background())
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if len(summary.Runs) != 0 {
				t.Fatalf("runs = %d, want 0", len(summary.Runs))
			}
			logged := buffer.String()
			if got := countLog(logged, "run skipped"); got != tc.want {
				t.Fatalf("run skipped lines = %d, want %d:\n%s", got, tc.want, logged)
			}
			if got := countLog(logged, "run started"); got != 0 {
				t.Fatalf("skipped run was started %d time(s):\n%s", got, logged)
			}
			if tc.want == 1 {
				findLog(t, logged, "run skipped", map[string]string{
					"plugin": fx.plugin, "scenario": fx.scenario, "arm": "with", "run": "1",
				})
			}
		})
	}
}

// TestRunLogsProgressPositions covers the suite-supplied progress function: the
// runner calls it once per executed arm/run, so a resumed run that skips most
// of its work consumes no position, and both records of a run carry the same
// position.
func TestRunLogsProgressPositions(t *testing.T) {
	fx := newFixture(t, fixtureOptions{})
	buffer := &logBuffer{}
	var mu sync.Mutex
	calls, position := 0, 0
	// The suite derives the denominator from the run decisions; here run-1 is
	// skipped, so this pass executes two runs.
	const total = 2
	r, err := New(Config{
		Root:     fx.root,
		Plugin:   fx.plugin,
		Scenario: fx.scenario,
		Arm:      "with",
		Runs:     3,
		Out:      t.TempDir(),
		SkillDir: ".claude/skills",
		Executor: mustExecutor(t),
		Logger:   testLogger(buffer, slog.LevelInfo),
		SkipRun:  func(_ string, run int) bool { return run == 1 },
		Progress: func(string, int) (int, int) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			position++
			return position, total
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	summary, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(summary.Runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(summary.Runs))
	}
	if calls != 2 {
		t.Fatalf("progress calls = %d, want one per executed run", calls)
	}
	logged := buffer.String()
	for _, tc := range []struct {
		run      int
		progress string
	}{
		{run: 2, progress: "1/2"},
		{run: 3, progress: "2/2"},
	} {
		wanted := map[string]string{"arm": "with", "run": itoa(tc.run)}
		started := findLog(t, logged, "run started", wanted)
		if started["progress"] != tc.progress {
			t.Fatalf("run %d started progress = %q, want %q", tc.run, started["progress"], tc.progress)
		}
		finished := findLog(t, logged, "run finished", wanted)
		if finished["progress"] != tc.progress {
			t.Fatalf("run %d finished progress = %q, want %q", tc.run, finished["progress"], tc.progress)
		}
	}
	if got := countLog(logged, "run started"); got != 2 {
		t.Fatalf("run started lines = %d, want 2 (the skipped run takes no position):\n%s", got, logged)
	}
}

func TestRunNilLoggerDoesNotPanic(t *testing.T) {
	fx := newFixture(t, fixtureOptions{})
	summary, err := execute(t, fx, runOptions{out: t.TempDir(), logger: nil})
	if err != nil {
		t.Fatalf("run with nil logger: %v", err)
	}
	if len(summary.Runs) != 1 || summary.Runs[0].Status != model.StatusOK {
		t.Fatalf("outcome = %+v", summary.Runs)
	}
}

func TestRunRecordsSkillUseInResult(t *testing.T) {
	fx := newFixture(t, fixtureOptions{})
	used := `echo '{"type":"tool_use","part":{"tool":"skill","state":{"input":{"name":"good"}}}}'`
	idle := `echo '{"type":"text","part":{"text":"no skill here"}}'`
	cases := []struct {
		name    string
		arm     string
		command string
		want    string
	}{
		{"skill invoked", "with", used, model.SkillInvoked},
		{"skill not invoked", "with", idle, model.SkillNotInvoked},
		{"unrecognised log", "with", "echo hello", model.SkillUnknown},
		{"without arm records nothing", "without", used, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := t.TempDir()
			if _, err := execute(t, fx, runOptions{out: out, arm: tc.arm, command: tc.command}); err != nil {
				t.Fatalf("run: %v", err)
			}
			result := readJSON[model.RunResult](t, filepath.Join(runPath(out, tc.arm, 1), "result.json"))
			switch {
			case tc.want == "" && result.SkillUse != nil:
				t.Fatalf("skill_use = %+v, want none", result.SkillUse)
			case tc.want != "" && (result.SkillUse == nil || result.SkillUse.State != tc.want):
				t.Fatalf("skill_use = %+v, want %s", result.SkillUse, tc.want)
			}
		})
	}
}
