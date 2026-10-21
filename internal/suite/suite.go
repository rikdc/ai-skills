// Package suite plans, runs, resumes, grades, and aggregates an eval suite.
//
// Suite orchestration never reimplements execution or judging: every arm/run is
// delegated to internal/runner and every judgement to internal/judge. The
// suite owns selection, the manifest/config contract, immutable-attempt
// resumption, the worker pool, the writer lock, and the aggregate artifacts.
package suite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rikdc/ai-skills/internal/executor"
	"github.com/rikdc/ai-skills/internal/judge"
	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/platform/atomicfile"
	"github.com/rikdc/ai-skills/internal/platform/errs"
	"github.com/rikdc/ai-skills/internal/runner"
	"github.com/rikdc/ai-skills/internal/runner/attempts"
	"github.com/rikdc/ai-skills/internal/scenario"
	"github.com/rikdc/ai-skills/internal/skilluse"
	"github.com/rikdc/ai-skills/internal/usage"
)

// Resume policies.
const (
	PolicyRetryFailed  = "retry-failed"
	PolicyKeepFailures = "keep-failures"
	PolicyRerunAll     = "rerun-all"
)

// Config is one `evalctl suite` invocation.
type Config struct {
	Root      string
	Selection scenario.Selection

	Arm    string
	Runs   int
	Jobs   int
	Out    string
	Keep   bool
	Strict bool

	Resume       bool
	ResumePolicy string
	Regrade      bool

	SkillDir string

	Timeout       time.Duration
	SetupTimeout  time.Duration
	ChecksTimeout time.Duration
	JudgeTimeout  time.Duration

	MaxSnapshotBytes int64
	AttemptArtifacts string
	PassEnv          []string
	UsageCmd         string
	JudgeUsageCmd    string
	PricingSHA256    string

	AuthCLI   bool
	Keychain  bool
	AuthPaths []string

	AgentExec executor.AgentExecutor
	JudgeExec executor.AgentExecutor
	JudgeCmd  string

	Logger *slog.Logger
	Now    func() time.Time
}

// Result summarises one suite pass.
type Result struct {
	Out            string
	Planned        int
	Executed       int
	Skipped        int
	RunnerFailures int
	PendingRuns    int
	StrictFailures []string
	SummaryPath    string
	SuiteJSONPath  string
	ResultsPath    string
	GradingIndex   string
}

// Runner is a configured suite.
type Runner struct {
	cfg      Config
	plan     *scenario.Plan
	warnings []string
	created  string
}

// PlanOptions builds a plan for `list`, `plan`, and `suite --plan`.
type PlanOptions struct {
	Selection       scenario.Selection
	Runs            int
	Arm             string
	Jobs            int
	TimeoutSeconds  int
	JudgeConfigured bool
}

// BuildPlan is the shared suite plan used by `evalctl plan`.
func BuildPlan(root string, opts PlanOptions) (*scenario.Plan, []string, error) {
	return scenario.BuildPlan(root, scenario.PlanOptions{
		Selection:       opts.Selection,
		Runs:            opts.Runs,
		Arm:             opts.Arm,
		Jobs:            opts.Jobs,
		TimeoutSeconds:  opts.TimeoutSeconds,
		JudgeConfigured: opts.JudgeConfigured,
	})
}

// New validates a suite invocation and computes the deterministic plan.
func New(cfg Config) (*Runner, error) {
	if strings.TrimSpace(cfg.Root) == "" {
		return nil, errs.Usage("repository root is required")
	}
	cfg.Root, _ = filepath.Abs(cfg.Root)

	if cfg.Runs < 1 {
		return nil, errs.Usage("--runs must be a positive integer")
	}
	if cfg.Jobs < 1 {
		return nil, errs.Usage("--jobs must be a positive integer")
	}
	if cfg.Arm == "" {
		cfg.Arm = "both"
	}
	if cfg.SkillDir == "" {
		cfg.SkillDir = ".claude/skills"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = model.DefaultTimeout
	}
	if cfg.SetupTimeout <= 0 {
		cfg.SetupTimeout = 5 * time.Minute
	}
	if cfg.ChecksTimeout <= 0 {
		cfg.ChecksTimeout = 2 * time.Minute
	}
	if cfg.JudgeTimeout <= 0 {
		cfg.JudgeTimeout = 10 * time.Minute
	}
	if cfg.MaxSnapshotBytes == 0 {
		cfg.MaxSnapshotBytes = 256 << 20
	}
	switch cfg.AttemptArtifacts {
	case "":
		cfg.AttemptArtifacts = "failed"
	case "failed", "all", "none":
	default:
		return nil, errs.Usage("--attempt-artifacts must be failed, all or none")
	}
	switch cfg.ResumePolicy {
	case "":
		cfg.ResumePolicy = PolicyRetryFailed
	case PolicyRetryFailed, PolicyKeepFailures, PolicyRerunAll:
	default:
		return nil, errs.Usage("--resume-policy must be retry-failed, keep-failures or rerun-all")
	}
	if cfg.ResumePolicy != PolicyRetryFailed && !cfg.Resume {
		return nil, errs.Usage("--resume-policy, --keep-failures and --rerun require --resume")
	}
	if cfg.Resume && strings.TrimSpace(cfg.Out) == "" {
		return nil, errs.Usage("--resume needs --out pointing at a previous suite")
	}
	if cfg.Regrade && !cfg.Resume {
		return nil, errs.Usage("--regrade needs --resume and --out")
	}
	if cfg.Regrade && strings.TrimSpace(cfg.JudgeCmd) == "" {
		return nil, errs.Usage("--regrade needs --judge-cmd")
	}
	if strings.TrimSpace(cfg.JudgeCmd) != "" && cfg.JudgeExec == nil {
		return nil, errs.Usage("judge executor is required for --judge-cmd")
	}
	if cfg.AgentExec == nil {
		return nil, errs.Usage("--agent-cmd is required for the command backend")
	}

	plan, warnings, err := scenario.BuildPlan(cfg.Root, scenario.PlanOptions{
		Selection:       cfg.Selection,
		Runs:            cfg.Runs,
		Arm:             cfg.Arm,
		Jobs:            cfg.Jobs,
		TimeoutSeconds:  int(cfg.Timeout / time.Second),
		JudgeConfigured: cfg.JudgeCmd != "",
	})
	if err != nil {
		if scenario.IsUsageError(err) {
			return nil, errs.Usage("%s", scenario.UsageMessage(err))
		}
		return nil, err
	}
	if len(plan.Selected) == 0 {
		if len(plan.Unsupported) > 0 {
			return nil, errs.Usage("direct-hook execution is milestone C and not implemented")
		}
		return nil, errs.Usage("no runnable scenarios selected")
	}
	for _, warning := range warnings {
		if cfg.Logger != nil {
			cfg.Logger.Warn(warning)
		}
	}

	now := time.Now
	if cfg.Now != nil {
		now = cfg.Now
	}
	return &Runner{
		cfg:      cfg,
		plan:     plan,
		warnings: warnings,
		created:  now().UTC().Format(time.RFC3339),
	}, nil
}

// Plan exposes the computed plan.
func (r *Runner) Plan() *scenario.Plan { return r.plan }

// Warnings returns non-fatal selection warnings.
func (r *Runner) Warnings() []string { return append([]string(nil), r.warnings...) }

// Run executes the suite, or resumes it. It returns a Result and an error
// carrying the exit code contract: 0 complete/pending, 1 runner or strict
// failure, 2 configuration/provenance. Signal cancellation is left to the
// caller's signal-to-exit mapping.
func (r *Runner) Run(ctx context.Context) (*Result, error) {
	cfg := r.cfg
	out, err := r.prepareOut()
	if err != nil {
		return nil, err
	}
	cfg.Out = out
	r.cfg.Out = out
	result := &Result{
		Out:           out,
		Planned:       len(r.plan.Selected),
		SummaryPath:   filepath.Join(out, "summary.md"),
		SuiteJSONPath: filepath.Join(out, "suite.json"),
		ResultsPath:   filepath.Join(out, "results.jsonl"),
		GradingIndex:  filepath.Join(out, "grading", "index.tsv"),
	}

	release, err := lockFile(out)
	if err != nil {
		return result, err
	}
	defer release()

	fingerprints := map[string]string{}
	selected := r.plan.Selected
	for _, item := range selected {
		fp, err := scenario.Fingerprint(cfg.Root, item.Scenario.Plugin, item.Scenario.Name)
		if err != nil {
			return result, errs.Operational("fingerprint %s: %v", item.Pair(), err)
		}
		fingerprints[item.Pair()] = fp
	}

	manifest, err := r.loadOrCreateManifest(fingerprints)
	if err != nil {
		return result, err
	}
	if err := r.writeManifest(manifest); err != nil {
		return result, err
	}
	if err := r.writePlanTSV(manifest); err != nil {
		return result, err
	}
	if err := r.copyCriteria(selected); err != nil {
		return result, err
	}
	if cfg.Regrade {
		r.invalidateLegacyJudgements(selected)
	}
	judgeIdentity := ""
	if cfg.JudgeCmd != "" {
		identity := judge.CommandIdentity(cfg.JudgeCmd)
		judgeIdentity, err = judge.IdentitySHA256(identity)
		if err != nil {
			return result, errs.Operational("hash judge identity: %v", err)
		}
		if !cfg.Regrade {
			if err := r.checkJudgeProvenance(selected, judgeIdentity); err != nil {
				return result, err
			}
		}
	}

	decisions, err := r.planRuns(selected, fingerprints, judgeIdentity)
	if err != nil {
		return result, err
	}
	if err := r.prepareDirs(out); err != nil {
		return result, err
	}

	// The plan is final once the resume decisions exist. Announce its shape
	// before any worker starts, so the position inside a long campaign is
	// visible from the first line of output.
	runsToRun := expectedRuns(decisions)
	scenariosToRun := r.executingScenarios(selected, decisions)
	r.logPlan(len(selected), runsToRun, scenariosToRun)

	started := time.Now()
	type scenarioJob struct {
		item scenario.Selected
	}
	jobs := make(chan scenarioJob)
	var mu sync.Mutex
	progress := newProgressTracker(&mu, scenariosToRun, runsToRun)
	gradedInline := map[string]bool{}
	var wg sync.WaitGroup

	workers := cfg.Jobs
	if workers > len(selected) {
		workers = len(selected)
	}
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				executed, runErr := r.runScenario(ctx, job.item, decisions, gradedInline, &mu, progress)
				mu.Lock()
				if executed {
					result.Executed++
				} else {
					result.Skipped++
				}
				if runErr != nil {
					result.RunnerFailures++
				}
				mu.Unlock()
			}
		}()
	}
feed:
	for _, item := range selected {
		select {
		case <-ctx.Done():
			break feed
		case jobs <- scenarioJob{item: item}:
		}
	}
	close(jobs)
	wg.Wait()

	fatal := r.finalGrading(ctx, selected, gradedInline)
	if fatal != nil {
		return result, fatal
	}

	stats := r.aggregate(selected, fingerprints, decisions, started, result.Executed, result.Skipped)
	result.RunnerFailures = stats.RunnerFailures
	result.PendingRuns = stats.PendingRuns
	if err := r.writeArtifacts(manifest, stats); err != nil {
		return result, errs.Operational("%v", err)
	}
	result.StrictFailures = stats.StrictFailures

	if ctx.Err() != nil {
		return result, errs.Operational("suite cancelled: %v", ctx.Err())
	}
	if stats.RunnerFailures > 0 {
		message := fmt.Sprintf("%d scenario(s) had runner failures", stats.RunnerFailures)
		if detail := firstRunFailure(stats); detail != "" {
			message += "; first: " + detail
		}
		message += fmt.Sprintf("; retry with --resume --resume-policy %s --out %s", PolicyRetryFailed, out)
		return result, errs.Operational("%s; see %s", message, result.SummaryPath)
	}
	if len(stats.StrictFailures) > 0 {
		return result, errs.Operational("strict: %s", strings.Join(stats.StrictFailures, "; "))
	}
	return result, nil
}

// prepareOut resolves --out, checks empty/non-empty rules, and creates the
// suite directory tree.
func (r *Runner) prepareOut() (string, error) {
	out := r.cfg.Out
	if out == "" {
		out = filepath.Join(r.cfg.Root, "results", "eval-suite",
			time.Now().UTC().Format("20060102T150405Z")+"-"+gitShortSHA(r.cfg.Root))
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		return "", errs.Usage("resolve --out: %v", err)
	}
	if r.cfg.Resume {
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			return "", errs.Usage("--resume found no suite directory at %s", abs)
		}
		if _, err := os.Stat(filepath.Join(abs, "manifest.json")); err != nil {
			return "", errs.Usage("--resume found no manifest in %s (is that a suite directory?)", abs)
		}
	} else {
		entries, err := os.ReadDir(abs)
		if err == nil && len(entries) > 0 {
			return "", errs.Usage("%s is not empty; use --resume or a new --out", abs)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", errs.Operational("read --out: %v", err)
		}
	}
	if err := r.prepareDirs(abs); err != nil {
		return "", err
	}
	return abs, nil
}

func (r *Runner) prepareDirs(out string) error {
	for _, dir := range []string{
		out,
		filepath.Join(out, "runs"),
		filepath.Join(out, "logs"),
		filepath.Join(out, "grading"),
		filepath.Join(out, "criteria"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return errs.Operational("create suite directory %s: %v", dir, err)
		}
	}
	return nil
}

func gitShortSHA(root string) string {
	out, err := exec.Command("git", "-C", root, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "nogit"
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" {
		return "nogit"
	}
	return sha
}

// runKey identifies one run.
type runKey struct {
	plugin   string
	scenario string
	arm      string
	run      int
}

func (k runKey) runDir(root string) string {
	return filepath.Join(root, "runs", k.plugin, k.scenario, k.arm, fmt.Sprintf("run-%d", k.run))
}

// decision is the resume verdict for one run.
type decision struct {
	execute bool
	retain  bool
	retried bool
	reason  string
}

func armsFor(mode model.Mode, arm string) []string {
	if mode == model.ModeFunctional {
		return []string{"with"}
	}
	switch arm {
	case "with":
		return []string{"with"}
	case "without":
		return []string{"without"}
	default:
		return []string{"without", "with"}
	}
}

// logPlan emits the run-start plan record and the serial warning. The record
// names the selection and the invocation counts this pass will actually spend
// (resume decisions already removed reusable and retained work), so a long
// campaign is legible before it starts. It goes through the configured logger:
// never on stdout, silenceable with -q, and a nil logger is a no-op.
func (r *Runner) logPlan(scenarios, runs, executingScenarios int) {
	judge := any("none")
	if r.cfg.JudgeCmd != "" {
		judge = runs
	}
	logInfo(r.cfg.Logger, "suite plan",
		"scenarios", scenarios,
		"agent_invocations", runs,
		"judge_invocations", judge,
		"jobs", r.cfg.Jobs,
		"timeout_seconds", int(r.cfg.Timeout/time.Second),
	)
	if r.cfg.Jobs == 1 && executingScenarios > 1 {
		logWarn(r.cfg.Logger,
			"scenarios run one after another; use --jobs N (JOBS=N with make) to run them concurrently",
			"scenarios", executingScenarios, "jobs", r.cfg.Jobs)
	}
}

// expectedRuns counts the arm/run pairs this pass will execute.
func expectedRuns(decisions map[runKey]decision) int {
	runs := 0
	for _, d := range decisions {
		if d.execute {
			runs++
		}
	}
	return runs
}

// hasExecutableRun reports whether this pass executes at least one arm/run for
// the scenario.
func (r *Runner) hasExecutableRun(item scenario.Selected, decisions map[runKey]decision) bool {
	for _, arm := range armsFor(item.Mode(), r.cfg.Arm) {
		for n := 1; n <= r.cfg.Runs; n++ {
			if decisions[runKey{item.Scenario.Plugin, item.Scenario.Name, arm, n}].execute {
				return true
			}
		}
	}
	return false
}

// executingScenarios counts the scenarios this pass will run.
func (r *Runner) executingScenarios(selected []scenario.Selected, decisions map[runKey]decision) int {
	scenarios := 0
	for _, item := range selected {
		if r.hasExecutableRun(item, decisions) {
			scenarios++
		}
	}
	return scenarios
}

// progressTracker numbers the suite's scenario and run progress records. The
// totals come from the run decisions, so they describe what this pass
// executes; the counters increment under the suite's run mutex because the
// worker goroutines share them.
type progressTracker struct {
	mu            *sync.Mutex
	scenarioTotal int
	runTotal      int
	scenarios     int
	runs          int
}

func newProgressTracker(mu *sync.Mutex, scenarios, runs int) *progressTracker {
	return &progressTracker{mu: mu, scenarioTotal: scenarios, runTotal: runs}
}

// nextScenario returns the position of the scenario about to start, as
// "i/N".
func (p *progressTracker) nextScenario() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.scenarios++
	return fmt.Sprintf("%d/%d", p.scenarios, p.scenarioTotal)
}

// nextRun returns the position of the arm/run about to execute, in the shape
// runner.Config.Progress expects.
func (p *progressTracker) nextRun(string, int) (int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.runs++
	return p.runs, p.runTotal
}

func (r *Runner) planRuns(selected []scenario.Selected, fingerprints map[string]string, judgeIdentity string) (map[runKey]decision, error) {
	decisions := map[runKey]decision{}
	for _, item := range selected {
		mode := item.Mode()
		for _, arm := range armsFor(mode, r.cfg.Arm) {
			for n := 1; n <= r.cfg.Runs; n++ {
				key := runKey{plugin: item.Scenario.Plugin, scenario: item.Scenario.Name, arm: arm, run: n}
				decisions[key] = decision{execute: true}
				if !r.cfg.Resume || r.cfg.ResumePolicy == PolicyRerunAll {
					continue
				}
				dir := key.runDir(r.cfg.Out)
				store := attempts.New(dir)
				fp := fingerprints[item.Pair()]
				if store.Reusable(fp) {
					r.ensureProjections(dir, item, arm, n)
					decisions[key] = decision{reason: "reusable"}
					continue
				}
				if _, changed, err := store.Adopt(attempts.AdoptOptions{
					Fingerprint:         fp,
					JudgeIdentitySHA256: judgeIdentity,
					AcceptedBy:          "manual-adopt/" + model.Version,
				}); err != nil {
					return nil, errs.Operational("adopt %s: %v", dir, err)
				} else if changed {
					r.ensureProjections(dir, item, arm, n)
					decisions[key] = decision{reason: "adopted"}
					continue
				}
				if r.cfg.ResumePolicy == PolicyKeepFailures {
					list, err := store.Enumerate()
					if err != nil {
						return nil, errs.Operational("enumerate %s: %v", dir, err)
					}
					// Retain interrupted and non-ok work, including a failed
					// rerun of an accepted run; stale successful work is
					// rerun (it is not a failure).
					if latest := attempts.LatestExecution(list); latest != nil &&
						(latest.Finished == nil || latest.Finished.Status != model.StatusOK) {
						decisions[key] = decision{retain: true, reason: "kept-failure"}
						continue
					}
				}
				if r.cfg.ResumePolicy == PolicyRetryFailed {
					list, err := store.Enumerate()
					if err == nil && len(list) > 0 {
						decisions[key] = decision{execute: true, retried: true, reason: "retried"}
					}
				}
			}
		}
	}
	return decisions, nil
}

// ensureProjections rebuilds result.json and grading-request.json when they
// are absent. The pointer wins; this never reruns a model or judge.
func (r *Runner) ensureProjections(dir string, item scenario.Selected, arm string, run int) {
	store := attempts.New(dir)
	pointer, _, err := store.LoadCurrent()
	if err != nil {
		return
	}
	checksRequired := false
	if info, err := os.Stat(filepath.Join(item.Scenario.Dir, "checks.sh")); err == nil {
		checksRequired = info.Mode()&0o111 != 0
	}
	runCtx := attempts.RunContext{
		Plugin:         item.Scenario.Plugin,
		Scenario:       item.Scenario.Name,
		Arm:            arm,
		Mode:           string(item.Mode()),
		Run:            run,
		ChecksRequired: checksRequired,
		SkillDir:       r.cfg.SkillDir,
	}
	if arm == "with" {
		runCtx.Skills = skilluse.Installed(
			filepath.Join(r.cfg.Root, "plugins", item.Scenario.Plugin), item.InstallUnits())
	}
	if _, err := os.Stat(store.ResultPath()); errors.Is(err, os.ErrNotExist) {
		_, _ = store.EnsureResultProjection(runCtx)
	}
	if _, err := os.Stat(store.GradingRequestPath()); errors.Is(err, os.ErrNotExist) {
		agent, err := store.LoadAttempt(pointer.AgentAttempt)
		if err != nil {
			return
		}
		rubric, err := scenario.ParseCriteria(filepath.Join(item.Scenario.Dir, "criteria.json"))
		if err != nil {
			return
		}
		mechanical := readMechanical(agent.Dir)
		covered := make([]string, 0, len(mechanical.Items))
		seen := map[string]bool{}
		for _, judged := range mechanical.Items {
			covered = append(covered, judged.Name)
			seen[judged.Name] = true
		}
		var pending []model.Criterion
		for _, criterion := range rubric.Checklist {
			if !seen[criterion.Name] {
				pending = append(pending, criterion)
			}
		}
		_, _ = store.EnsureGradingRequestProjection(model.GradingRequest{
			SchemaVersion:    model.SchemaVersion,
			Plugin:           item.Scenario.Plugin,
			Scenario:         item.Scenario.Name,
			Arm:              arm,
			Run:              run,
			AgentAttempt:     pointer.AgentAttempt,
			EvidenceSHA256:   pointer.AgentEvidenceSHA256,
			Criteria:         rubric,
			CoveredByChecks:  covered,
			JudgementPending: pending,
		})
	}
}

func (r *Runner) runScenario(ctx context.Context, item scenario.Selected,
	decisions map[runKey]decision, gradedInline map[string]bool,
	mu *sync.Mutex, progress *progressTracker) (bool, error) {

	logPath := filepath.Join(r.cfg.Out, "logs", item.Scenario.Plugin, item.Scenario.Name+".log")
	statusPath := filepath.Join(r.cfg.Out, "logs", item.Scenario.Plugin, item.Scenario.Name+".status")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return false, errs.Operational("create log directory: %v", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return false, errs.Operational("create log %s: %v", logPath, err)
	}
	defer logFile.Close()
	fmt.Fprintf(logFile, "suite: %s/%s\n", item.Scenario.Plugin, item.Scenario.Name)

	if !r.hasExecutableRun(item, decisions) {
		fmt.Fprintln(logFile, "skipped: all runs are reusable or retained")
		_ = atomicfile.Write(statusPath, []byte("0\n"), 0o600)
		return false, nil
	}

	armArg := r.cfg.Arm
	if item.Mode() == model.ModeFunctional {
		// --arm applies to comparison only; functional scenarios always run
		// their single with arm.
		armArg = "with"
	}
	runCfg := runner.Config{
		Root:             r.cfg.Root,
		Plugin:           item.Scenario.Plugin,
		Scenario:         item.Scenario.Name,
		Arm:              armArg,
		Runs:             r.cfg.Runs,
		Units:            item.Units,
		SkillDir:         r.cfg.SkillDir,
		Out:              filepath.Join(r.cfg.Out, "runs"),
		Timeout:          r.cfg.Timeout,
		SetupTimeout:     r.cfg.SetupTimeout,
		ChecksTimeout:    r.cfg.ChecksTimeout,
		Keep:             r.cfg.Keep,
		MaxSnapshotBytes: r.cfg.MaxSnapshotBytes,
		AttemptArtifacts: r.cfg.AttemptArtifacts,
		PassEnv:          r.cfg.PassEnv,
		Executor:         r.cfg.AgentExec,
		Logger:           r.cfg.Logger,
		Progress:         progress.nextRun,
	}
	runCfg.SkipRun = func(arm string, run int) bool {
		d := decisions[runKey{item.Scenario.Plugin, item.Scenario.Name, arm, run}]
		if !d.execute {
			fmt.Fprintf(logFile, "skip %s/run-%d: %s\n", arm, run, d.reason)
		}
		return !d.execute
	}

	scenarioRunner, err := runner.New(runCfg)
	if err != nil {
		fmt.Fprintf(logFile, "error: %v\n", err)
		_ = atomicfile.Write(statusPath, []byte("1\n"), 0o600)
		return true, err
	}
	scenarioProgress := progress.nextScenario()
	logInfo(r.cfg.Logger, "scenario started",
		"plugin", item.Scenario.Plugin, "scenario", item.Scenario.Name,
		"progress", scenarioProgress)
	scenarioStartedAt := time.Now()
	summary, runErr := scenarioRunner.Run(ctx)
	scenarioAttributes := []any{
		"plugin", item.Scenario.Plugin,
		"scenario", item.Scenario.Name,
		"progress", scenarioProgress,
		"status", scenarioStatus(summary, runErr),
		"duration_seconds", wholeSeconds(time.Since(scenarioStartedAt)),
	}
	if runErr != nil {
		scenarioAttributes = append(scenarioAttributes, "error", runErr.Error())
	}
	logInfo(r.cfg.Logger, "scenario finished", scenarioAttributes...)
	if runErr != nil {
		fmt.Fprintf(logFile, "error: %v\n", runErr)
	}
	if summary != nil {
		for _, run := range summary.Runs {
			line := fmt.Sprintf("%s/run-%d: %s", run.Arm, run.Run, run.Status)
			if run.Error != "" {
				line += ": " + run.Error
			}
			fmt.Fprintln(logFile, line)
		}
		warnUnusedSkills(r.cfg.Logger, item, summary)
	}

	// Inline judging runs only for runs that finished ok in this pass.
	if r.cfg.JudgeCmd != "" && summary != nil && ctx.Err() == nil {
		for _, run := range summary.Runs {
			if ctx.Err() != nil {
				break
			}
			if run.Status != model.StatusOK {
				continue
			}
			key := runKey{item.Scenario.Plugin, item.Scenario.Name, run.Arm, run.Run}
			dir := key.runDir(r.cfg.Out)
			regrade := r.cfg.Regrade
			gradeErr := gradeRun(ctx, r.cfg, key, dir, regrade)
			if gradeErr != nil {
				fmt.Fprintf(logFile, "grade %s: %v\n", dir, gradeErr)
				if errs.Code(gradeErr) == errs.CodeUsage {
					_ = atomicfile.Write(statusPath, []byte("1\n"), 0o600)
					return true, gradeErr
				}
			} else {
				mu.Lock()
				gradedInline[keyString(key)] = true
				mu.Unlock()
			}
		}
	}

	if ctx.Err() != nil {
		_ = atomicfile.Write(statusPath, []byte("1\n"), 0o600)
		return true, ctx.Err()
	}
	if runErr != nil {
		_ = atomicfile.Write(statusPath, []byte("1\n"), 0o600)
		return true, runErr
	}
	_ = atomicfile.Write(statusPath, []byte("0\n"), 0o600)
	return true, nil
}

func keyString(k runKey) string {
	return fmt.Sprintf("%s/%s/%s/%d", k.plugin, k.scenario, k.arm, k.run)
}

// gradeRun grades one run and emits its progress pair. The judge key attributes
// are logged here so inline and final grading share one shape.
func gradeRun(ctx context.Context, cfg Config, key runKey, runDir string, regrade bool) error {
	logInfo(cfg.Logger, "judge started",
		"plugin", key.plugin, "scenario", key.scenario, "arm", key.arm, "run", key.run)
	_, err := judge.Grade(ctx, judge.Config{
		RunDir:   runDir,
		Command:  cfg.JudgeCmd,
		Timeout:  cfg.JudgeTimeout,
		Regrade:  regrade,
		Executor: cfg.JudgeExec,
	})
	attributes := []any{
		"plugin", key.plugin,
		"scenario", key.scenario,
		"arm", key.arm,
		"run", key.run,
	}
	if err != nil {
		attributes = append(attributes, "error", err.Error())
		logWarn(cfg.Logger, "judge failed", attributes...)
		return err
	}
	logInfo(cfg.Logger, "judge finished", attributes...)
	return nil
}

// logInfo emits one-line progress records through the configured logger.
// Progress never goes to stdout, and a nil logger is a no-op.
func logInfo(logger *slog.Logger, msg string, args ...any) {
	if logger != nil {
		logger.Info(msg, args...)
	}
}

// logWarn emits one-line anomaly records through the configured logger, so a
// failed judge is visible at the same level as the pre-existing final-grading
// warning.
func logWarn(logger *slog.Logger, msg string, args ...any) {
	if logger != nil {
		logger.Warn(msg, args...)
	}
}

// warnUnusedSkills emits one warning per with-arm run that finished without
// invoking the scenario's skill, so the miss is visible while the suite runs
// rather than only in the rendered report. It reads the recorded skill use
// from result.json; a run that never recorded one (or recorded "unknown")
// says nothing.
func warnUnusedSkills(logger *slog.Logger, item scenario.Selected, summary *runner.Summary) {
	if summary == nil || item.Mode() == model.ModeFunctional {
		return
	}
	for _, run := range summary.Runs {
		if run.Arm != "with" || run.Status != model.StatusOK {
			continue
		}
		use := recordedSkillUse(run.Dir)
		if use == nil || use.State != model.SkillNotInvoked {
			continue
		}
		logWarn(logger, "skill not invoked; the scenario cannot measure the skill, rerun with RUNS>=3",
			"plugin", item.Scenario.Plugin, "scenario", item.Scenario.Name,
			"arm", run.Arm, "run", run.Run)
	}
}

// wholeSeconds rounds a measured duration to whole seconds for progress logs.
func wholeSeconds(d time.Duration) int64 {
	return int64(math.Round(d.Seconds()))
}

// scenarioStatus mirrors the suite's scenario status vocabulary: every run ok
// means ok, anything else means runner-failed.
func scenarioStatus(summary *runner.Summary, runErr error) string {
	if runErr != nil || summary == nil {
		return "runner-failed"
	}
	for _, run := range summary.Runs {
		if run.Status != model.StatusOK {
			return "runner-failed"
		}
	}
	return "ok"
}

func (r *Runner) finalGrading(ctx context.Context, selected []scenario.Selected, gradedInline map[string]bool) error {
	if r.cfg.JudgeCmd == "" || ctx.Err() != nil {
		return nil
	}
	for _, item := range selected {
		for _, arm := range armsFor(item.Mode(), r.cfg.Arm) {
			for n := 1; n <= r.cfg.Runs; n++ {
				// The caller reports the cancellation; grading more runs would
				// only record cancelled judge attempts.
				if ctx.Err() != nil {
					return nil
				}
				key := runKey{item.Scenario.Plugin, item.Scenario.Name, arm, n}
				dir := key.runDir(r.cfg.Out)
				store := attempts.New(dir)
				if _, err := os.Stat(dir); err != nil {
					continue
				}
				if _, _, err := store.LoadCurrent(); err != nil {
					continue
				}
				regrade := r.cfg.Regrade && !gradedInline[keyString(key)]
				if err := gradeRun(ctx, r.cfg, key, dir, regrade); err != nil {
					if errs.Code(err) == errs.CodeUsage {
						return err
					}
					if r.cfg.Logger != nil {
						r.cfg.Logger.Warn("final grading failed", "run", keyString(key), "error", err)
					}
				}
			}
		}
	}
	return nil
}

func (r *Runner) checkJudgeProvenance(selected []scenario.Selected, identity string) error {
	for _, item := range selected {
		for _, arm := range armsFor(item.Mode(), r.cfg.Arm) {
			for n := 1; n <= r.cfg.Runs; n++ {
				key := runKey{item.Scenario.Plugin, item.Scenario.Name, arm, n}
				dir := key.runDir(r.cfg.Out)
				store := attempts.New(dir)
				pointer, _, err := store.LoadCurrent()
				if err != nil || pointer.JudgeAttempt == "" {
					continue
				}
				// A replaced result's judgement is never shown or reused.
				if superseding, _ := store.Superseded(); superseding != nil {
					continue
				}
				if pointer.JudgeIdentitySHA256 != identity {
					return errs.Usage(
						"%s was judged by a different judge; pass --regrade to replace the judgement", dir)
				}
			}
		}
	}
	return nil
}

func (r *Runner) invalidateLegacyJudgements(selected []scenario.Selected) {
	for _, item := range selected {
		for _, arm := range armsFor(item.Mode(), r.cfg.Arm) {
			for n := 1; n <= r.cfg.Runs; n++ {
				dir := runKey{item.Scenario.Plugin, item.Scenario.Name, arm, n}.runDir(r.cfg.Out)
				_ = os.Remove(filepath.Join(dir, "judgement.json"))
				_ = os.Remove(filepath.Join(dir, "judge-result.json"))
			}
		}
	}
}

// manifest plumbing ---------------------------------------------------------

func (r *Runner) loadOrCreateManifest(fingerprints map[string]string) (model.SuiteManifest, error) {
	config, err := r.suiteConfig()
	if err != nil {
		return model.SuiteManifest{}, err
	}
	if !r.cfg.Resume {
		return model.SuiteManifest{
			Version:   model.ManifestVersion,
			CreatedAt: r.created,
			Tool:      r.toolInfo(),
			Repo:      r.repoInfo(),
			Config:    config,
			Scenarios: r.plannedScenarios(fingerprints),
		}, nil
	}

	var old model.SuiteManifest
	path := filepath.Join(r.cfg.Out, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return model.SuiteManifest{}, errs.Usage("read manifest: %v", err)
	}
	if err := json.Unmarshal(data, &old); err != nil {
		return model.SuiteManifest{}, errs.Usage("manifest.json is not valid JSON: %v", err)
	}
	if old.Version != model.ManifestVersion {
		return model.SuiteManifest{}, errs.Usage(
			"%s: suite --resume requires manifest version %d (found %d); use evalctl report for v1/v2 suites",
			r.cfg.Out, model.ManifestVersion, old.Version)
	}
	if problems := compareConfigs(old.Config, config, r.cfg.Regrade); len(problems) > 0 {
		return model.SuiteManifest{}, errs.Usage(
			"--resume configuration differs from the existing suite: %s", strings.Join(problems, "; "))
	}
	if notice := resumeTimeoutNotice(old.Config, config); notice != "" {
		logInfo(r.cfg.Logger,
			"resume timeouts differ from the recorded suite; only the retried runs use the new caps",
			"changes", notice)
	}
	if old.CreatedAt != "" {
		r.created = old.CreatedAt
	}
	return model.SuiteManifest{
		Version:   model.ManifestVersion,
		CreatedAt: r.created,
		Tool:      r.toolInfo(),
		Repo:      r.repoInfo(),
		Config:    config,
		Scenarios: r.plannedScenarios(fingerprints),
	}, nil
}

func (r *Runner) toolInfo() model.ToolInfo {
	sha := gitShortSHA(r.cfg.Root)
	return model.ToolInfo{
		Name:          "evalctl",
		Version:       model.Version,
		Commit:        sha,
		SchemaVersion: model.SchemaVersion,
		GraderVersion: model.GraderVersion,
	}
}

func (r *Runner) repoInfo() model.RepoInfo {
	sha := gitShortSHA(r.cfg.Root)
	dirty := false
	if out, err := exec.Command("git", "-C", r.cfg.Root, "status", "--porcelain").Output(); err == nil {
		dirty = strings.TrimSpace(string(out)) != ""
	}
	return model.RepoInfo{Root: r.cfg.Root, GitSHA: sha, GitDirty: dirty}
}

func (r *Runner) plannedScenarios(fingerprints map[string]string) []model.PlannedScenario {
	out := make([]model.PlannedScenario, 0, len(r.plan.Selected))
	for _, item := range r.plan.Selected {
		out = append(out, model.PlannedScenario{
			Plugin:      item.Scenario.Plugin,
			Scenario:    item.Scenario.Name,
			Fingerprint: fingerprints[item.Pair()],
			Units:       item.InstallUnits(),
			Mode:        string(item.Mode()),
			Execution:   string(item.Execution()),
		})
	}
	return out
}

func (r *Runner) suiteConfig() (model.SuiteConfig, error) {
	identity := r.cfg.AgentExec.Identity()
	limits := model.DefaultLimits()
	authPaths := append([]string{}, r.cfg.AuthPaths...)
	units := append([]string{}, r.cfg.Selection.Units...)
	judgeConfig := model.JudgeConfig{}
	if r.cfg.JudgeCmd != "" {
		judgeIdentity := judge.CommandIdentity(r.cfg.JudgeCmd)
		sha, err := judge.IdentitySHA256(judgeIdentity)
		if err != nil {
			return model.SuiteConfig{}, errs.Operational("hash judge identity: %v", err)
		}
		judgeConfig = model.JudgeConfig{
			Backend:        "command",
			Cmd:            r.cfg.JudgeCmd,
			IdentitySHA256: sha,
		}
	}
	return model.SuiteConfig{
		Backend:              identity.Kind,
		Model:                identity.Model,
		BaseURL:              identity.BaseURL,
		Temperature:          "0",
		MaxSteps:             limits.MaxSteps,
		MaxTotalTokens:       limits.MaxTotalTokens,
		MaxOutputTokens:      limits.MaxOutputTokens,
		ContextWindow:        limits.ContextWindow,
		KeepRecentGroups:     limits.KeepRecentGroups,
		AgentCmd:             identity.Command,
		Arm:                  r.cfg.Arm,
		Runs:                 r.cfg.Runs,
		TimeoutSeconds:       int(r.cfg.Timeout / time.Second),
		SetupTimeoutSeconds:  int(r.cfg.SetupTimeout / time.Second),
		ChecksTimeoutSeconds: int(r.cfg.ChecksTimeout / time.Second),
		JudgeTimeoutSeconds:  int(r.cfg.JudgeTimeout / time.Second),
		SkillDir:             r.cfg.SkillDir,
		AuthCLI:              r.cfg.AuthCLI,
		Keychain:             r.cfg.Keychain,
		AuthPaths:            authPaths,
		Units:                units,
		Judge:                judgeConfig,
		UsageCmd:             r.cfg.UsageCmd,
		JudgeUsageCmd:        r.cfg.JudgeUsageCmd,
		PricingSHA256:        r.cfg.PricingSHA256,
		PricingFormula:       pricingFormula(r.cfg.PricingSHA256),
		AttemptArtifacts:     r.cfg.AttemptArtifacts,
		HarnessVersion:       model.Version,
	}, nil
}

// resumeTimeoutNotice describes the per-run timeout changes a resume accepts,
// so a widened or tightened cap is visible rather than silent.
func resumeTimeoutNotice(old, current model.SuiteConfig) string {
	var parts []string
	add := func(field string, a, b int) {
		if a != b {
			parts = append(parts, fmt.Sprintf("%s %ds → %ds", field, a, b))
		}
	}
	add("agent", old.TimeoutSeconds, current.TimeoutSeconds)
	add("setup", old.SetupTimeoutSeconds, current.SetupTimeoutSeconds)
	add("checks", old.ChecksTimeoutSeconds, current.ChecksTimeoutSeconds)
	add("judge", old.JudgeTimeoutSeconds, current.JudgeTimeoutSeconds)
	return strings.Join(parts, ", ")
}

// pricingFormula records the cost formula only when a price sheet is in use.
func pricingFormula(pricingSHA256 string) string {
	if pricingSHA256 == "" {
		return ""
	}
	return usage.PricingFormula
}

func compareConfigs(old, current model.SuiteConfig, regrade bool) []string {
	var problems []string
	add := func(field string, a, b any) {
		if fmt.Sprint(a) != fmt.Sprint(b) {
			problems = append(problems, fmt.Sprintf("%s changed (%v → %v)", field, a, b))
		}
	}
	add("backend", old.Backend, current.Backend)
	add("model", old.Model, current.Model)
	add("base_url", old.BaseURL, current.BaseURL)
	add("temperature", old.Temperature, current.Temperature)
	add("seed", old.Seed, current.Seed)
	add("max_steps", old.MaxSteps, current.MaxSteps)
	add("max_total_tokens", old.MaxTotalTokens, current.MaxTotalTokens)
	add("max_output_tokens", old.MaxOutputTokens, current.MaxOutputTokens)
	add("context_window", old.ContextWindow, current.ContextWindow)
	add("keep_recent_groups", old.KeepRecentGroups, current.KeepRecentGroups)
	add("agent_cmd", old.AgentCmd, current.AgentCmd)
	add("arm", old.Arm, current.Arm)
	add("runs", old.Runs, current.Runs)
	// The per-run timeouts are operational caps, not part of the measurement
	// contract: a resume may tighten or widen them for the runs it retries
	// without invalidating accepted attempts. Changing them is reported by
	// resumeTimeoutNotice instead of being refused.
	add("skill_dir", old.SkillDir, current.SkillDir)
	add("auth_cli", old.AuthCLI, current.AuthCLI)
	add("keychain", old.Keychain, current.Keychain)
	add("auth_paths", strings.Join(old.AuthPaths, ","), strings.Join(current.AuthPaths, ","))
	add("units", strings.Join(old.Units, ","), strings.Join(current.Units, ","))
	add("usage_cmd", old.UsageCmd, current.UsageCmd)
	add("judge_usage_cmd", old.JudgeUsageCmd, current.JudgeUsageCmd)
	// Reused runs keep their recorded cost, so a different price sheet would
	// mix cost bases within one suite.
	add("pricing_sha256", old.PricingSHA256, current.PricingSHA256)
	add("pricing_formula", old.PricingFormula, current.PricingFormula)
	add("harness_version", old.HarnessVersion, current.HarnessVersion)
	if !regrade {
		add("judge.backend", old.Judge.Backend, current.Judge.Backend)
		add("judge.model", old.Judge.Model, current.Judge.Model)
		add("judge.base_url", old.Judge.BaseURL, current.Judge.BaseURL)
		add("judge.cmd", old.Judge.Cmd, current.Judge.Cmd)
		add("judge.identity_sha256", old.Judge.IdentitySHA256, current.Judge.IdentitySHA256)
	}
	return problems
}

func (r *Runner) writeManifest(manifest model.SuiteManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return errs.Operational("encode manifest: %v", err)
	}
	if err := atomicfile.Write(filepath.Join(r.cfg.Out, "manifest.json"), append(data, '\n'), 0o600); err != nil {
		return errs.Operational("write manifest: %v", err)
	}
	return nil
}

func (r *Runner) writePlanTSV(manifest model.SuiteManifest) error {
	var b strings.Builder
	for _, scenario := range manifest.Scenarios {
		fmt.Fprintf(&b, "%s\t%s\t%s\n", scenario.Plugin, scenario.Scenario, scenario.Fingerprint)
	}
	if err := atomicfile.Write(filepath.Join(r.cfg.Out, "plan.tsv"), []byte(b.String()), 0o600); err != nil {
		return errs.Operational("write plan.tsv: %v", err)
	}
	return nil
}

func (r *Runner) copyCriteria(selected []scenario.Selected) error {
	for _, item := range selected {
		dir := filepath.Join(r.cfg.Out, "criteria", item.Scenario.Plugin)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return errs.Operational("create criteria directory: %v", err)
		}
		src := filepath.Join(item.Scenario.Dir, "criteria.json")
		data, err := os.ReadFile(src)
		if err != nil {
			return errs.Operational("read %s: %v", src, err)
		}
		dst := filepath.Join(dir, item.Scenario.Name+".json")
		if err := atomicfile.Write(dst, data, 0o600); err != nil {
			return errs.Operational("write %s: %v", dst, err)
		}
	}
	return nil
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(path, append(data, '\n'), 0o600)
}

func writeResultsJSONL(path string, rows []ScenarioRow) error {
	var b strings.Builder
	for _, row := range rows {
		data, err := json.Marshal(row)
		if err != nil {
			return err
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return atomicfile.Write(path, []byte(b.String()), 0o600)
}

func (r *Runner) writeArtifacts(manifest model.SuiteManifest, stats aggregateStats) error {
	doc := suiteDocument{
		Config:          manifest.Config,
		Counts:          stats.Counts,
		DurationSeconds: stats.DurationSeconds,
		Scenarios:       stats.Scenarios,
		Plugins:         stats.Plugins,
	}
	if err := writeResultsJSONL(filepath.Join(r.cfg.Out, "results.jsonl"), stats.Scenarios); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(r.cfg.Out, "suite.json"), doc); err != nil {
		return err
	}
	if err := writeGradingIndex(filepath.Join(r.cfg.Out, "grading", "index.tsv"), stats.Scenarios); err != nil {
		return err
	}
	return atomicfile.Write(filepath.Join(r.cfg.Out, "summary.md"),
		[]byte(renderSummary(manifest, stats, r.cfg.Out)), 0o600)
}

// SummaryText reads a written summary for CLI output.
func SummaryText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
