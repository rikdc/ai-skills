// Package selftest runs deterministic end-to-end checks for evalctl and
// exercises every repository fixture setup script without a model, network, or
// credentials.
package selftest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rikdc/ai-skills/internal/executor"
	"github.com/rikdc/ai-skills/internal/judge"
	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/platform/errs"
	"github.com/rikdc/ai-skills/internal/provider/command"
	"github.com/rikdc/ai-skills/internal/report"
	"github.com/rikdc/ai-skills/internal/reportio"
	"github.com/rikdc/ai-skills/internal/runner"
	"github.com/rikdc/ai-skills/internal/runner/attempts"
	"github.com/rikdc/ai-skills/internal/scenario"
	"github.com/rikdc/ai-skills/internal/suite"
)

// Options configures one selftest pass.
type Options struct {
	// Root is the repository whose plugin fixtures are exercised. It may be
	// empty when Quick is true.
	Root string
	// Quick skips repository fixture coverage.
	Quick bool
	// Keep retains the temporary selftest directory.
	Keep bool
	// Jobs bounds concurrent fixture scenarios. Zero means one.
	Jobs int
	// Out receives progress lines. Nil discards them.
	Out io.Writer
}

// Result summarises a successful selftest pass.
type Result struct {
	Checks   int
	Fixtures int
	Duration time.Duration
	BaseDir  string
}

// Run executes the synthetic end-to-end flow and, unless Quick is set, every
// repository fixture setup script.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if opts.Jobs < 1 {
		opts.Jobs = 1
	}
	if !opts.Quick {
		if strings.TrimSpace(opts.Root) == "" {
			return nil, errs.Usage("selftest fixture coverage requires a repository root (use --quick to skip it)")
		}
		abs, err := filepath.Abs(opts.Root)
		if err != nil {
			return nil, errs.Usage("resolve repository root: %v", err)
		}
		opts.Root = abs
	}
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	logf := func(format string, args ...any) {
		fmt.Fprintf(out, format+"\n", args...)
	}

	base, err := os.MkdirTemp("", "evalctl-selftest-")
	if err != nil {
		return nil, fmt.Errorf("create selftest directory: %w", err)
	}
	if !opts.Keep {
		defer os.RemoveAll(base)
	}
	logf("selftest: work directory %s", base)

	started := time.Now()
	checks, err := runEndToEnd(ctx, filepath.Join(base, "end-to-end"), logf)
	if err != nil {
		return nil, err
	}

	fixtures := 0
	if !opts.Quick {
		fixtures, err = runFixtures(ctx, opts.Root, filepath.Join(base, "fixtures"), opts.Jobs, logf)
		if err != nil {
			return nil, err
		}
	}
	return &Result{
		Checks:   checks,
		Fixtures: fixtures,
		Duration: time.Since(started),
		BaseDir:  base,
	}, nil
}

func runEndToEnd(ctx context.Context, dir string, logf func(string, ...any)) (int, error) {
	repoRoot := filepath.Join(dir, "repo")
	if err := writeSyntheticRepo(repoRoot); err != nil {
		return 0, err
	}
	logf("selftest: synthetic repository %s", repoRoot)
	checks := 0
	ok := func(name string) {
		checks++
		logf("  ok  %s", name)
	}

	exec, err := command.New(command.Config{Command: "true", Timeout: 2 * time.Minute})
	if err != nil {
		return checks, fmt.Errorf("selftest executor: %w", err)
	}
	judgeCmd := filepath.Join(dir, "judge.sh")
	if err := os.WriteFile(judgeCmd, []byte(judgeScript), 0o755); err != nil {
		return checks, fmt.Errorf("write selftest judge: %w", err)
	}
	judgeExec, err := command.New(command.Config{Command: judgeCmd, Timeout: 2 * time.Minute})
	if err != nil {
		return checks, fmt.Errorf("selftest judge executor: %w", err)
	}

	runDir := filepath.Join(dir, "run")
	single, err := runner.New(runner.Config{
		Root:             repoRoot,
		Plugin:           "selftest",
		Scenario:         "basic",
		Arm:              "both",
		Runs:             1,
		SkillDir:         ".claude/skills",
		Out:              runDir,
		Timeout:          2 * time.Minute,
		SetupTimeout:     time.Minute,
		ChecksTimeout:    time.Minute,
		MaxSnapshotBytes: 64 << 20,
		AttemptArtifacts: "all",
		Executor:         exec,
	})
	if err != nil {
		return checks, fmt.Errorf("selftest run: %w", err)
	}
	summary, err := single.Run(ctx)
	if err != nil {
		return checks, fmt.Errorf("selftest run: %w", err)
	}
	if len(summary.Runs) != 2 {
		return checks, fmt.Errorf("selftest run: %d arm runs, want 2", len(summary.Runs))
	}
	for _, outcome := range summary.Runs {
		if outcome.Status != model.StatusOK {
			return checks, fmt.Errorf("selftest run %s/run-%d: status %s", outcome.Arm, outcome.Run, outcome.Status)
		}
		attemptDir := filepath.Join(outcome.Dir, "attempts", "agent-000001")
		if err := requireFiles(attemptDir,
			"started.json", "finished.json", "evidence.json", "setup.log",
			"before.txt", "after.txt", "snapshot.tar"); err != nil {
			return checks, fmt.Errorf("selftest run %s: %w", outcome.Arm, err)
		}
		if err := requireFiles(outcome.Dir, "current.json", "result.json", "grading-request.json"); err != nil {
			return checks, fmt.Errorf("selftest run %s: %w", outcome.Arm, err)
		}
	}
	ok("run produced immutable agent attempts, evidence, and projections")

	suiteDir := filepath.Join(dir, "suite")
	suiteRunner, err := newSuite(repoRoot, suiteDir, exec, nil, "", false, "", false)
	if err != nil {
		return checks, fmt.Errorf("selftest suite: %w", err)
	}
	suiteResult, err := suiteRunner.Run(ctx)
	if err != nil {
		return checks, fmt.Errorf("selftest suite: %w", err)
	}
	if suiteResult.Executed != 1 || suiteResult.RunnerFailures != 0 {
		return checks, fmt.Errorf("selftest suite: executed=%d failures=%d, want 1/0",
			suiteResult.Executed, suiteResult.RunnerFailures)
	}
	if err := requireFiles(suiteDir, "manifest.json", "suite.json", "results.jsonl", "summary.md"); err != nil {
		return checks, fmt.Errorf("selftest suite: %w", err)
	}
	ok("suite planned and executed both arms")

	gradeResult, err := judge.GradeSuite(ctx, judge.Config{
		SuiteDir: suiteDir,
		Command:  judgeCmd,
		Timeout:  2 * time.Minute,
		Executor: judgeExec,
	})
	if err != nil {
		return checks, fmt.Errorf("selftest grade: %w", err)
	}
	if gradeResult.Total != 2 || gradeResult.Graded != 2 || gradeResult.Failed != 0 {
		return checks, fmt.Errorf("selftest grade: total=%d graded=%d failed=%d, want 2/2/0",
			gradeResult.Total, gradeResult.Graded, gradeResult.Failed)
	}
	// Projections are derived: rebuild them so report sees the accepted judge.
	for _, arm := range []string{"without", "with"} {
		store := attempts.New(filepath.Join(suiteDir, "runs", "selftest", "basic", arm, "run-1"))
		if _, err := store.EnsureResultProjection(attempts.RunContext{
			Plugin:         "selftest",
			Scenario:       "basic",
			Arm:            arm,
			Mode:           string(model.ModeComparison),
			Run:            1,
			ChecksRequired: true,
		}); err != nil {
			return checks, fmt.Errorf("selftest grade: rebuild %s projection: %w", arm, err)
		}
	}
	ok("grade bound judgements to both accepted agent attempts")

	loaded, err := reportio.Load(suiteDir, reportio.Options{RepoRoot: repoRoot})
	if err != nil {
		return checks, fmt.Errorf("selftest report load: %w", err)
	}
	analysis := report.Build(loaded)
	if len(analysis.Scenarios) != 1 || analysis.Totals.N != 1 || analysis.Totals.PendingPoints != 0 {
		return checks, fmt.Errorf("selftest report: scenarios=%d n=%d pending=%v, want 1/1/0",
			len(analysis.Scenarios), analysis.Totals.N, analysis.Totals.PendingPoints)
	}
	if analysis.Totals.With == nil || *analysis.Totals.With != 100 {
		return checks, fmt.Errorf("selftest report: with percent = %v, want 100", analysis.Totals.With)
	}
	reportDir := filepath.Join(suiteDir, "report")
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return checks, fmt.Errorf("selftest report directory: %w", err)
	}
	rendered := map[string]string{
		"report.json": mustJSON(analysis),
		"report.md":   report.RenderMarkdown(analysis),
		"report.html": report.RenderHTML(analysis),
	}
	for name, content := range rendered {
		if strings.TrimSpace(content) == "" {
			return checks, fmt.Errorf("selftest report: %s is empty", name)
		}
		if err := os.WriteFile(filepath.Join(reportDir, name), []byte(content), 0o644); err != nil {
			return checks, fmt.Errorf("selftest report: %w", err)
		}
	}
	if cli := report.RenderCLI(analysis, false); !strings.Contains(cli, "selftest/basic") {
		return checks, fmt.Errorf("selftest report: CLI output does not mention the scenario")
	}
	ok("report rendered CLI, JSON, Markdown, and HTML with no pending points")

	withRun := filepath.Join(suiteDir, "runs", "selftest", "basic", "with", "run-1")
	resumeRunner, err := newSuite(repoRoot, suiteDir, exec, nil, "", true, suite.PolicyRetryFailed, false)
	if err != nil {
		return checks, fmt.Errorf("selftest resume: %w", err)
	}
	resumed, err := resumeRunner.Run(ctx)
	if err != nil {
		return checks, fmt.Errorf("selftest resume: %w", err)
	}
	if resumed.Executed != 0 || resumed.Skipped != 1 {
		return checks, fmt.Errorf("selftest resume: executed=%d skipped=%d, want 0/1",
			resumed.Executed, resumed.Skipped)
	}
	ok("resume reused the accepted attempts without rerunning")

	rerunRunner, err := newSuite(repoRoot, suiteDir, exec, nil, "", true, suite.PolicyRerunAll, false)
	if err != nil {
		return checks, fmt.Errorf("selftest rerun: %w", err)
	}
	rerun, err := rerunRunner.Run(ctx)
	if err != nil {
		return checks, fmt.Errorf("selftest rerun: %w", err)
	}
	if rerun.Executed != 1 {
		return checks, fmt.Errorf("selftest rerun: executed=%d, want 1", rerun.Executed)
	}
	if err := requireFiles(withRun, filepath.Join("attempts", "agent-000003", "finished.json")); err != nil {
		return checks, fmt.Errorf("selftest rerun: %w", err)
	}
	ok("--rerun created fresh agent attempts")

	store := attempts.New(withRun)
	pointerBefore, _, err := store.LoadCurrent()
	if err != nil {
		return checks, fmt.Errorf("selftest regrade: %w", err)
	}
	regraded, err := judge.Grade(ctx, judge.Config{
		RunDir:   withRun,
		Command:  judgeCmd,
		Timeout:  2 * time.Minute,
		Regrade:  true,
		Executor: judgeExec,
	})
	if err != nil {
		return checks, fmt.Errorf("selftest regrade: %w", err)
	}
	if regraded.Outcome != judge.OutcomeGraded {
		return checks, fmt.Errorf("selftest regrade: outcome %s", regraded.Outcome)
	}
	pointerAfter, _, err := store.LoadCurrent()
	if err != nil {
		return checks, fmt.Errorf("selftest regrade: %w", err)
	}
	if pointerAfter.JudgeAttempt == "" || pointerAfter.JudgeAttempt == pointerBefore.JudgeAttempt {
		return checks, fmt.Errorf("selftest regrade: judge attempt was not replaced (%q)", pointerAfter.JudgeAttempt)
	}
	if _, err := os.Stat(filepath.Join(withRun, "attempts", pointerBefore.JudgeAttempt)); err != nil {
		return checks, fmt.Errorf("selftest regrade: prior judge attempt was removed: %w", err)
	}
	ok("regrade created a new immutable judge attempt and preserved the prior one")

	return checks, nil
}

func newSuite(root, out string, agent, judgeExec executor.AgentExecutor, judgeCmd string, resume bool, policy string, regrade bool) (*suite.Runner, error) {
	return suite.New(suite.Config{
		Root: root,
		Selection: scenario.Selection{
			Plugins: []string{"selftest"},
		},
		Arm:              "both",
		Runs:             1,
		Jobs:             2,
		Out:              out,
		Resume:           resume,
		ResumePolicy:     policy,
		Regrade:          regrade,
		SkillDir:         ".claude/skills",
		Timeout:          2 * time.Minute,
		SetupTimeout:     time.Minute,
		ChecksTimeout:    time.Minute,
		JudgeTimeout:     2 * time.Minute,
		MaxSnapshotBytes: 64 << 20,
		AttemptArtifacts: "all",
		AgentExec:        agent,
		JudgeExec:        judgeExec,
		JudgeCmd:         judgeCmd,
	})
}

const judgeScript = `#!/usr/bin/env bash
set -euo pipefail
cat > judgement.json <<'EOF'
[{"name":"judged","score":12,"max":12,"note":"selftest"}]
EOF
`

func writeSyntheticRepo(root string) error {
	scenarioDir := filepath.Join(root, "plugins", "selftest", "evals", "basic")
	files := map[string]string{
		filepath.Join(root, "tessl.json"): `{"name":"selftest/repo"}`,
		filepath.Join(root, "plugins", "selftest", "skills", "selftest", "SKILL.md"): `---
name: selftest
description: Deterministic selftest skill used by evalctl selftest.
---
# Selftest
`,
		filepath.Join(scenarioDir, "scenario.json"): `{
  "description": "Deterministic synthetic scenario used by evalctl selftest",
  "include": ["./inputs"],
  "unit": "skills/selftest"
}`,
		filepath.Join(scenarioDir, "task.md"): "# Task\n\nCreate the selftest fixture file.\n",
		filepath.Join(scenarioDir, "criteria.json"): `{
  "context": "selftest",
  "type": "weighted_checklist",
  "checklist": [
    {"name": "mechanical", "description": "decided by checks.sh", "max_score": 8},
    {"name": "judged", "description": "decided by the command judge", "max_score": 12}
  ]
}`,
		filepath.Join(scenarioDir, "inputs", "setup.sh"): `#!/usr/bin/env bash
set -euo pipefail
printf 'selftest fixture\n' > fixture.txt
`,
		filepath.Join(scenarioDir, "checks.sh"): `#!/usr/bin/env bash
set -euo pipefail
printf '[{"name":"mechanical","score":8,"max":8,"note":"selftest check"}]\n'
`,
	}
	for path, content := range files {
		mode := os.FileMode(0o644)
		if strings.HasSuffix(path, ".sh") {
			mode = 0o755
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			return err
		}
	}
	return nil
}

func requireFiles(dir string, names ...string) error {
	for _, name := range names {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("%s is missing", filepath.Join(dir, name))
			}
			return err
		}
		if info.IsDir() {
			return fmt.Errorf("%s is a directory", filepath.Join(dir, name))
		}
	}
	return nil
}

func mustJSON(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return ""
	}
	return string(data) + "\n"
}
