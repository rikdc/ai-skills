package runner

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
