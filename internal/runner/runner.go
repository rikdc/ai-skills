// Package runner executes one scenario: arms, runs, setup, the command
// executor, checks, evidence, and immutable attempts.
//
// One run directory is runs/<plugin>/<scenario>/<arm>/run-N (the CLI passes the
// output root). Immutable attempts and current.json are the source of truth;
// result.json and grading-request.json are projections.
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rikdc/ai-skills/internal/executor"
	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/platform/atomicfile"
	"github.com/rikdc/ai-skills/internal/platform/errs"
	"github.com/rikdc/ai-skills/internal/runner/attempts"
	"github.com/rikdc/ai-skills/internal/scenario"
	"github.com/rikdc/ai-skills/internal/workspace"
)

// Config describes one `evalctl run` invocation.
type Config struct {
	Root     string
	Plugin   string
	Scenario string

	Arm   string
	Runs  int
	Units []string

	SkillDir string
	Out      string

	Timeout       time.Duration
	SetupTimeout  time.Duration
	ChecksTimeout time.Duration

	Keep             bool
	MaxSnapshotBytes int64
	AttemptArtifacts string

	PassEnv []string

	Executor executor.AgentExecutor
	Logger   *slog.Logger

	// SkipRun, when non-nil, reports whether an arm/run pair is already
	// satisfied and must not be executed again. Suite resume uses it; direct
	// `evalctl run` leaves it nil.
	SkipRun func(arm string, run int) bool

	// StoreHooks injects deterministic attempt-store failpoints. Tests use it;
	// production leaves it zero.
	StoreHooks attempts.Hooks
}

// Runner executes a single scenario.
type Runner struct {
	cfg      Config
	scenario *scenario.Scenario
	rubric   model.Rubric
	task     string
	fp       string
	arms     []string
}

// Summary is the outcome of one scenario invocation.
type Summary struct {
	Plugin   string       `json:"plugin"`
	Scenario string       `json:"scenario"`
	Mode     string       `json:"mode"`
	Runs     []RunOutcome `json:"runs"`
	Path     string       `json:"-"`
}

// RunOutcome is one executed arm/run pair.
type RunOutcome struct {
	Arm        string          `json:"arm"`
	Run        int             `json:"run"`
	Status     model.RunStatus `json:"status"`
	StopReason string          `json:"stop_reason,omitempty"`
	Score      float64         `json:"score"`
	Max        float64         `json:"max"`
	Dir        string          `json:"dir"`
}

// New validates the invocation and loads the scenario.
func New(cfg Config) (*Runner, error) {
	if strings.TrimSpace(cfg.Plugin) == "" || strings.TrimSpace(cfg.Scenario) == "" {
		return nil, errs.Usage("--plugin and --scenario are required")
	}
	if cfg.Runs == 0 {
		cfg.Runs = 1
	}
	if cfg.Runs < 1 {
		return nil, errs.Usage("--runs must be a positive integer")
	}
	if cfg.SkillDir == "" {
		cfg.SkillDir = ".claude/skills"
	}
	if err := workspace.ValidateSkillDir(cfg.SkillDir); err != nil {
		return nil, errs.Usage("%v", err)
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
	if cfg.MaxSnapshotBytes == 0 {
		cfg.MaxSnapshotBytes = 256 << 20
	}
	switch cfg.AttemptArtifacts {
	case "", "failed", "all", "none":
	default:
		return nil, errs.Usage("--attempt-artifacts must be failed, all or none")
	}

	sc, err := scenario.Load(cfg.Root, cfg.Plugin, cfg.Scenario)
	if err != nil {
		return nil, errs.Usage("%v", err)
	}
	if sc.DirectHook() {
		return nil, errs.Usage("%s: direct-hook execution is milestone C and not implemented",
			sc.Pair())
	}

	arm := cfg.Arm
	if arm == "" {
		arm = "both"
	}
	switch arm {
	case "both", "with", "without":
	default:
		return nil, errs.Usage("--arm must be both, with or without")
	}
	if sc.Mode() == model.ModeFunctional {
		switch arm {
		case "both":
			arm = "with"
		case "without":
			return nil, errs.Usage("functional scenarios have no without arm; use --arm with or --arm both")
		}
	}
	cfg.Arm = arm

	units, err := resolveUnits(sc, cfg.Units)
	if err != nil {
		return nil, err
	}
	cfg.Units = units

	rubric, err := scenario.ParseCriteria(filepath.Join(sc.Dir, "criteria.json"))
	if err != nil {
		return nil, errs.Usage("%s: criteria.json: %v", sc.Pair(), err)
	}
	if problems := scenario.ValidateRubric(rubric); len(problems) > 0 {
		return nil, errs.Usage("%s: criteria.json: %s", sc.Pair(), strings.Join(problems, "; "))
	}
	task, err := os.ReadFile(filepath.Join(sc.Dir, "task.md"))
	if err != nil {
		return nil, errs.Usage("%s: task.md: %v", sc.Pair(), err)
	}
	fp, err := scenario.Fingerprint(cfg.Root, cfg.Plugin, cfg.Scenario)
	if err != nil {
		return nil, errs.Operational("fingerprint %s: %v", sc.Pair(), err)
	}

	var arms []string
	if arm == "both" {
		arms = []string{"without", "with"}
	} else {
		arms = []string{arm}
	}
	return &Runner{
		cfg:      cfg,
		scenario: sc,
		rubric:   rubric,
		task:     string(task),
		fp:       fp,
		arms:     arms,
	}, nil
}

func resolveUnits(sc *scenario.Scenario, requested []string) ([]string, error) {
	declared := sc.DeclaredUnits()
	if len(requested) == 0 {
		return nil, nil
	}
	seen := map[string]bool{}
	var narrowed []string
	for _, unit := range requested {
		if seen[unit] {
			continue
		}
		seen[unit] = true
		if !scenario.UnitPattern(unit) {
			return nil, errs.Usage("invalid --unit %q (expected skills/<name>, commands/<name> or hooks/<name>)", unit)
		}
		if !scenario.UnitExists(sc.Root, sc.Plugin, unit) {
			return nil, errs.Usage("unknown unit %q for plugin %q", unit, sc.Plugin)
		}
		for _, d := range declared {
			if d == unit {
				narrowed = append(narrowed, unit)
				break
			}
		}
	}
	if len(narrowed) == 0 {
		return nil, errs.Usage("no requested unit is declared by %s (declared: %s)",
			sc.Pair(), strings.Join(declared, " "))
	}
	return narrowed, nil
}

// Run executes every arm/run pair and writes the scenario summary.
func (r *Runner) Run(ctx context.Context) (*Summary, error) {
	outDir := r.outputDir()
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return nil, errs.Operational("create output directory: %v", err)
	}
	summary := &Summary{
		Plugin:   r.cfg.Plugin,
		Scenario: r.cfg.Scenario,
		Mode:     string(r.scenario.Mode()),
	}
	var failed int
	for _, arm := range r.arms {
		for run := 1; run <= r.cfg.Runs; run++ {
			if r.cfg.SkipRun != nil && r.cfg.SkipRun(arm, run) {
				continue
			}
			outcome, err := r.runOnce(ctx, arm, run)
			if err != nil {
				return nil, err
			}
			summary.Runs = append(summary.Runs, outcome)
			if outcome.Status != model.StatusOK {
				failed++
			}
		}
	}
	summary.Path = filepath.Join(outDir, "summary.md")
	if err := writeSummary(summary); err != nil {
		return nil, errs.Operational("write summary: %v", err)
	}
	if failed > 0 {
		return summary, errs.Operational("%s/%s: %d run(s) did not complete ok; see %s",
			r.cfg.Plugin, r.cfg.Scenario, failed, summary.Path)
	}
	return summary, nil
}

// SummaryText renders the summary document for stdout.
func SummaryText(summary *Summary) string {
	data, err := os.ReadFile(summary.Path)
	if err != nil {
		return ""
	}
	return string(data)
}

func (r *Runner) outputDir() string {
	base := r.cfg.Out
	if base == "" {
		base = filepath.Join("results", "eval-run")
	}
	return filepath.Join(base, r.cfg.Plugin, r.cfg.Scenario)
}

func (r *Runner) runDir(arm string, run int) string {
	return filepath.Join(r.outputDir(), arm, fmt.Sprintf("run-%d", run))
}

type runState struct {
	store        *attempts.Store
	attempt      *attempts.Attempt
	ws           *workspace.Workspace
	runCtx       attempts.RunContext
	checksStatus string
	mechanical   model.Mechanical
	mechanicalOK bool
	usage        model.Usage
	exit         *int
	duration     *float64
}

func (r *Runner) runOnce(ctx context.Context, arm string, run int) (RunOutcome, error) {
	runDir := r.runDir(arm, run)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return RunOutcome{}, errs.Operational("create run directory: %v", err)
	}
	ws, err := workspace.New(r.cfg.Scenario, r.cfg.PassEnv)
	if err != nil {
		return RunOutcome{}, errs.Operational("%v", err)
	}
	defer func() {
		if !r.cfg.Keep {
			_ = ws.Cleanup()
		}
	}()

	if err := ws.StageIncludes(r.scenario.Dir, r.scenario.Meta.Spec.Include); err != nil {
		return RunOutcome{}, errs.Operational("stage %s: %v", r.scenario.Pair(), err)
	}
	hookScenario := hasHookUnit(r.scenario.DeclaredUnits())
	if hookScenario {
		pluginDir := filepath.Join(r.cfg.Root, "plugins", r.cfg.Plugin)
		if err := ws.StageHookSource(pluginDir, arm); err != nil {
			return RunOutcome{}, errs.Operational("stage hook source: %v", err)
		}
	}

	store := attempts.New(runDir)
	store.Hooks = r.cfg.StoreHooks
	identity := r.cfg.Executor.Identity()
	started := model.StartedAttempt{
		Kind:         model.KindAgent,
		RunID:        fmt.Sprintf("%s/%s/%s/run-%d", r.cfg.Plugin, r.cfg.Scenario, arm, run),
		Fingerprint:  r.fp,
		ConfigSHA256: r.configSHA(),
		Executor:     identity.Kind,
		Model:        identity.Model,
		Command:      identity.Command,
	}
	attempt, err := store.Allocate(started)
	if err != nil {
		return RunOutcome{}, errs.Operational("allocate attempt in %s: %v", runDir, err)
	}

	state := &runState{
		store:   store,
		attempt: attempt,
		ws:      ws,
		usage:   model.NotApplicable("none"),
		runCtx: attempts.RunContext{
			Plugin:         r.cfg.Plugin,
			Scenario:       r.cfg.Scenario,
			Arm:            arm,
			Mode:           string(r.scenario.Mode()),
			Run:            run,
			ChecksRequired: r.checksPresent(),
		},
	}

	pluginDir := filepath.Join(r.cfg.Root, "plugins", r.cfg.Plugin)
	scenarioDir := r.scenario.Dir
	criteriaPath := filepath.Join(scenarioDir, "criteria.json")
	setupScript := filepath.Join(ws.Dir, "inputs", "setup.sh")
	setupLog := filepath.Join(attempt.Dir, "setup.log")

	setupResult, setupRunErr := ws.RunSetup(ctx, scenarioDir, pluginDir, setupScript, setupLog, r.cfg.SetupTimeout)
	if setupRunErr != nil || setupResult.ExitCode != 0 {
		message := "setup failed"
		if setupRunErr != nil {
			message = setupRunErr.Error()
		} else if setupResult.ExitCode >= 0 {
			message = fmt.Sprintf("setup exited with status %d", setupResult.ExitCode)
		}
		if ctx.Err() != nil {
			return r.finishFailure(ctx, state, model.StatusCancelled, model.StopCancelled, message)
		}
		return r.finishFailure(ctx, state, model.StatusSetupFailed, model.StopSetupError, message)
	}

	beforePath := filepath.Join(attempt.Dir, "before.txt")
	if err := ws.Snapshot(beforePath); err != nil {
		return r.finishFailure(ctx, state, model.StatusEvidenceFailed, model.StopEvidenceError, "write before snapshot: "+err.Error())
	}

	if arm == "with" {
		if err := ws.InstallUnits(pluginDir, r.cfg.Plugin, r.cfg.SkillDir, r.cfg.Units); err != nil {
			return r.finishFailure(ctx, state, model.StatusEvidenceFailed, model.StopEvidenceError, "install units: "+err.Error())
		}
	}

	limits := model.DefaultLimits()
	limits.Timeout = r.cfg.Timeout
	res, runErr := r.cfg.Executor.Run(ctx, executor.AgentRunRequest{
		Task:      r.task,
		Workspace: ws.Dir,
		HomeDir:   ws.Home,
		LogDir:    attempt.Dir,
		Limits:    limits,
		LogFile:   "agent.log",
		Kind:      model.KindAgent,
		Arm:       arm,
		Run:       run,
	})
	state.usage = res.Usage.NormalizeState()
	if state.usage.State == "" {
		state.usage = model.Unknown("command", "executor returned no usage")
	}
	if res.ExitCode != nil {
		code := *res.ExitCode
		state.exit = &code
	}
	if res.Duration > 0 {
		seconds := res.Duration.Seconds()
		state.duration = &seconds
	}
	if err := writeJSON(filepath.Join(attempt.Dir, "usage.json"), state.usage); err != nil {
		return r.finishFailure(ctx, state, model.StatusEvidenceFailed, model.StopEvidenceError, "write usage: "+err.Error())
	}

	status := res.Status
	if status == "" {
		switch {
		case ctx.Err() != nil:
			status = model.StatusCancelled
		default:
			status = model.StatusAgentFailed
		}
	}
	stopReason := res.StopReason
	if stopReason == "" {
		if status == model.StatusOK {
			stopReason = model.StopCompleted
		} else {
			stopReason = model.StopProviderError
		}
	}
	message := res.Error
	if runErr != nil && message == "" {
		message = runErr.Error()
	}

	afterPath := filepath.Join(attempt.Dir, "after.txt")
	if err := ws.Snapshot(afterPath); err != nil {
		return r.finishFailure(ctx, state, model.StatusEvidenceFailed, model.StopEvidenceError, "write after snapshot: "+err.Error())
	}
	snapshotPath := filepath.Join(attempt.Dir, "snapshot.tar")
	if err := ws.Archive(snapshotPath, r.cfg.MaxSnapshotBytes); err != nil {
		return r.finishFailure(ctx, state, model.StatusEvidenceFailed, model.StopEvidenceError, "write evidence archive: "+err.Error())
	}

	state.checksStatus = model.ChecksNotApplicable
	covered := []string{}
	if status != model.StatusCancelled {
		checksResult, handled, checksErr := r.runChecks(ctx, state, scenarioDir, pluginDir, criteriaPath)
		if checksErr != nil {
			return r.finishFailure(ctx, state, model.StatusEvidenceFailed, model.StopEvidenceError, checksErr.Error())
		}
		if handled {
			if checksResult.ok {
				covered = checksResult.covered
				state.mechanical = checksResult.mechanical
				state.mechanicalOK = true
				state.checksStatus = model.ChecksOK
			} else {
				status = model.StatusChecksFailed
				stopReason = model.StopChecksError
				message = checksResult.error
				state.checksStatus = model.ChecksFailed
			}
		} else {
			state.checksStatus = model.ChecksNotApplicable
		}
	}

	manifestPaths := existingEvidence(attempt.Dir)
	evidenceSHA, err := store.WriteEvidenceManifest(attempt.ID, manifestPaths)
	if err != nil {
		return r.finishFailure(ctx, state, model.StatusEvidenceFailed, model.StopEvidenceError, "write evidence manifest: "+err.Error())
	}

	request := model.GradingRequest{
		SchemaVersion:    model.SchemaVersion,
		Plugin:           r.cfg.Plugin,
		Scenario:         r.cfg.Scenario,
		Arm:              arm,
		Run:              run,
		AgentAttempt:     attempt.ID,
		EvidenceSHA256:   evidenceSHA,
		Criteria:         r.rubric,
		CoveredByChecks:  covered,
		JudgementPending: pendingCriteria(r.rubric, covered),
	}
	requestSHA, err := store.WriteGradingRequest(request)
	if err != nil {
		return r.finishFailure(ctx, state, model.StatusEvidenceFailed, model.StopEvidenceError, "write grading request: "+err.Error())
	}

	mechanicalSHA := ""
	if state.mechanicalOK {
		mechanicalSHA = fileSHA256(filepath.Join(attempt.Dir, "mechanical.json"))
	}
	finished := model.FinishedAttempt{
		Status:               status,
		StopReason:           stopReason,
		Error:                message,
		Fingerprint:          r.fp,
		Usage:                state.usage,
		ChecksStatus:         state.checksStatus,
		MechanicalSHA256:     mechanicalSHA,
		EvidenceSHA256:       evidenceSHA,
		GradingRequestSHA256: requestSHA,
		AgentExit:            state.exit,
		TimedOut:             status == model.StatusTimedOut,
		DurationSeconds:      state.duration,
	}
	finishedAttempt, err := store.Finish(attempt.ID, finished)
	if err != nil {
		return RunOutcome{}, errs.Operational("finish attempt %s: %v", attempt.ID, err)
	}
	state.attempt = finishedAttempt

	outcome := RunOutcome{
		Arm:        arm,
		Run:        run,
		Status:     status,
		StopReason: stopReason,
		Score:      state.mechanical.Score,
		Max:        state.mechanical.Max,
		Dir:        runDir,
	}

	if status == model.StatusOK {
		if _, err := store.Accept(attempt.ID, attempts.AcceptOptions{}); err != nil {
			return outcome, errs.Operational("accept attempt %s: %v", attempt.ID, err)
		}
		if _, err := store.EnsureResultProjection(state.runCtx); err != nil {
			return outcome, errs.Operational("write result projection: %v", err)
		}
		if _, err := store.EnsureGradingRequestProjection(request); err != nil {
			return outcome, errs.Operational("write grading-request projection: %v", err)
		}
	} else if err := r.writeDiagnostic(state, evidenceSHA, outcome); err != nil {
		return outcome, err
	}
	if err := r.applyRetention(store); err != nil {
		return outcome, errs.Operational("apply --attempt-artifacts retention: %v", err)
	}
	return outcome, nil
}

func (r *Runner) finishFailure(ctx context.Context, state *runState, status model.RunStatus, stopReason, message string) (RunOutcome, error) {
	if state.usage.State == "" {
		state.usage = model.NotApplicable("none")
	}
	if state.checksStatus == "" {
		state.checksStatus = model.ChecksNotApplicable
	}
	if err := writeJSON(filepath.Join(state.attempt.Dir, "usage.json"), state.usage); err != nil {
		return RunOutcome{}, errs.Operational("write usage for %s: %v", state.attempt.ID, err)
	}
	finished := model.FinishedAttempt{
		Status:          status,
		StopReason:      stopReason,
		Error:           message,
		Fingerprint:     r.fp,
		Usage:           state.usage,
		ChecksStatus:    state.checksStatus,
		AgentExit:       state.exit,
		TimedOut:        status == model.StatusTimedOut,
		DurationSeconds: state.duration,
	}
	attempt, err := state.store.Finish(state.attempt.ID, finished)
	if err != nil {
		return RunOutcome{}, errs.Operational("finish attempt %s: %v", state.attempt.ID, err)
	}
	state.attempt = attempt
	outcome := RunOutcome{
		Arm:        state.runCtx.Arm,
		Run:        state.runCtx.Run,
		Status:     status,
		StopReason: stopReason,
		Dir:        filepath.Dir(filepath.Dir(attempt.Dir)),
	}
	if err := r.writeDiagnostic(state, "", outcome); err != nil {
		return outcome, err
	}
	if err := r.applyRetention(state.store); err != nil {
		return outcome, errs.Operational("apply --attempt-artifacts retention: %v", err)
	}
	return outcome, nil
}

type checksOutcome struct {
	ok         bool
	covered    []string
	mechanical model.Mechanical
	error      string
}

func (r *Runner) runChecks(ctx context.Context, state *runState, scenarioDir, pluginDir, criteriaPath string) (checksOutcome, bool, error) {
	script := filepath.Join(scenarioDir, "checks.sh")
	info, err := os.Stat(script)
	if errors.Is(err, fs.ErrNotExist) {
		return checksOutcome{}, false, nil
	}
	if err != nil {
		return checksOutcome{}, false, fmt.Errorf("stat checks.sh: %w", err)
	}
	if info.Mode()&0o111 == 0 {
		return checksOutcome{}, false, nil
	}
	rawPath := filepath.Join(state.attempt.Dir, "checks.raw.json")
	logPath := filepath.Join(state.attempt.Dir, "checks.log")
	result, runErr := state.ws.RunChecks(ctx, scenarioDir, pluginDir, criteriaPath, script, rawPath, logPath, r.cfg.ChecksTimeout)
	if runErr != nil || result.ExitCode != 0 {
		message := "checks.sh failed"
		if runErr != nil {
			message = runErr.Error()
		} else {
			message = fmt.Sprintf("checks.sh exited with status %d", result.ExitCode)
		}
		return checksOutcome{error: message}, true, nil
	}
	data, err := os.ReadFile(rawPath)
	if err != nil {
		return checksOutcome{error: "read checks output: " + err.Error()}, true, nil
	}
	items, err := validateChecks(data, r.rubric)
	if err != nil {
		return checksOutcome{error: err.Error()}, true, nil
	}
	mechanical := model.Mechanical{}
	covered := make([]string, 0, len(items))
	for _, item := range items {
		mechanical.Score += item.Score
		mechanical.Max += item.Max
		covered = append(covered, item.Name)
	}
	mechanical.Items = items
	if err := writeJSON(filepath.Join(state.attempt.Dir, "mechanical.json"), mechanical); err != nil {
		return checksOutcome{}, false, fmt.Errorf("write mechanical.json: %w", err)
	}
	return checksOutcome{ok: true, covered: covered, mechanical: mechanical}, true, nil
}

// validateChecks enforces the v3 mechanical-output contract. Any invalid item
// invalidates the whole output.
func validateChecks(data []byte, rubric model.Rubric) ([]model.JudgedItem, error) {
	type rawItem struct {
		Name  *string  `json:"name"`
		Score *float64 `json:"score"`
		Max   *float64 `json:"max"`
		Note  string   `json:"note"`
	}
	var raw []rawItem
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("checks.sh output is not a JSON array: %w", err)
	}
	maxByName := make(map[string]float64, len(rubric.Checklist))
	for _, criterion := range rubric.Checklist {
		maxByName[criterion.Name] = criterion.MaxScore
	}
	seen := map[string]bool{}
	items := make([]model.JudgedItem, 0, len(raw))
	for i, item := range raw {
		if item.Name == nil || strings.TrimSpace(*item.Name) == "" {
			return nil, fmt.Errorf("checks.sh item %d has no name", i)
		}
		if seen[*item.Name] {
			return nil, fmt.Errorf("checks.sh item %q is duplicated", *item.Name)
		}
		seen[*item.Name] = true
		want, ok := maxByName[*item.Name]
		if !ok {
			return nil, fmt.Errorf("checks.sh item %q is not in the rubric", *item.Name)
		}
		if item.Max == nil {
			return nil, fmt.Errorf("checks.sh item %q has no max", *item.Name)
		}
		if math.IsNaN(*item.Max) || math.IsInf(*item.Max, 0) || *item.Max != want {
			return nil, fmt.Errorf("checks.sh item %q max %v does not match rubric max %v", *item.Name, *item.Max, want)
		}
		if item.Score == nil {
			return nil, fmt.Errorf("checks.sh item %q has no score", *item.Name)
		}
		if math.IsNaN(*item.Score) || math.IsInf(*item.Score, 0) || *item.Score < 0 || *item.Score > want {
			return nil, fmt.Errorf("checks.sh item %q score %v is outside [0,%v]", *item.Name, *item.Score, want)
		}
		items = append(items, model.JudgedItem{Name: *item.Name, Score: *item.Score, Max: *item.Max, Note: item.Note})
	}
	return items, nil
}

// heavyEvidence is the retention policy's pruneable set. Metadata, usage,
// evidence.json, finished/started records, snapshots (before/after), and
// mechanical.json are never pruned.
var heavyEvidence = []string{
	"snapshot.tar",
	"agent.log",
	"transcript.jsonl",
	"checks.raw.json",
	"checks.log",
	"judge.log",
}

// applyRetention implements --attempt-artifacts:
//   - all:   retain everything.
//   - failed: retain failed/interrupted attempts; prune heavy evidence of
//     superseded successful attempts.
//   - none:  prune heavy evidence of every non-current finished attempt.
//
// The accepted attempt and accepted judge attempt are never pruned. A pruning
// marker preserves the original evidence digest so regrade can detect missing
// evidence instead of pretending it exists.
func (r *Runner) applyRetention(store *attempts.Store) error {
	mode := r.cfg.AttemptArtifacts
	if mode == "" {
		mode = "failed"
	}
	if mode == "all" {
		return nil
	}
	agentID, judgeID := "", ""
	if pointer, _, err := store.LoadCurrent(); err == nil {
		agentID = pointer.AgentAttempt
		judgeID = pointer.JudgeAttempt
	}
	all, err := store.Enumerate()
	if err != nil {
		return err
	}
	for _, attempt := range all {
		if attempt.Finished == nil {
			continue
		}
		if attempt.ID == agentID || attempt.ID == judgeID {
			continue
		}
		if mode == "failed" && attempt.Status != model.StatusOK {
			continue
		}
		var paths []string
		for _, name := range heavyEvidence {
			if info, err := os.Stat(filepath.Join(attempt.Dir, name)); err == nil && info.Mode().IsRegular() {
				paths = append(paths, name)
			}
		}
		if len(paths) == 0 {
			continue
		}
		if _, err := store.PruneEvidence(attempt.ID, paths, "attempt-artifacts="+mode); err != nil {
			return fmt.Errorf("prune %s: %w", attempt.ID, err)
		}
	}
	return nil
}

func pendingCriteria(rubric model.Rubric, covered []string) []model.Criterion {
	set := make(map[string]bool, len(covered))
	for _, name := range covered {
		set[name] = true
	}
	pending := make([]model.Criterion, 0, len(rubric.Checklist))
	for _, criterion := range rubric.Checklist {
		if !set[criterion.Name] {
			pending = append(pending, criterion)
		}
	}
	return pending
}

func existingEvidence(attemptDir string) []string {
	candidates := []string{
		"setup.log",
		"before.txt",
		"after.txt",
		"snapshot.tar",
		"agent.log",
		"transcript.jsonl",
		"checks.raw.json",
		"checks.log",
		"mechanical.json",
		"hook-results.json",
	}
	var paths []string
	for _, name := range candidates {
		if info, err := os.Stat(filepath.Join(attemptDir, name)); err == nil && info.Mode().IsRegular() {
			paths = append(paths, name)
		}
	}
	return paths
}

// writeDiagnostic writes a non-accepted result projection when current.json is
// absent. It never overwrites an accepted result.
func (r *Runner) writeDiagnostic(state *runState, evidenceSHA string, outcome RunOutcome) error {
	if _, _, err := state.store.LoadCurrent(); err == nil {
		return nil
	}
	usage := state.usage
	if usage.State == "" {
		usage = model.NotApplicable("none")
	}
	result := model.RunResult{
		SchemaVersion:        model.SchemaVersion,
		Plugin:               r.cfg.Plugin,
		Scenario:             r.cfg.Scenario,
		Arm:                  outcome.Arm,
		Mode:                 string(r.scenario.Mode()),
		Run:                  outcome.Run,
		AgentAttempt:         state.attempt.ID,
		CurrentSHA256:        "",
		Complete:             false,
		Status:               outcome.Status,
		StopReason:           outcome.StopReason,
		Backend:              r.cfg.Executor.Identity().Kind,
		Model:                r.cfg.Executor.Identity().Model,
		Fingerprint:          r.fp,
		ChecksRequired:       r.checksPresent(),
		TimedOut:             outcome.Status == model.StatusTimedOut,
		AgentExit:            state.exit,
		DurationSeconds:      state.duration,
		Mechanical:           state.mechanical,
		Usage:                usage,
		AgentUsage:           usage,
		JudgeUsage:           model.NotApplicable("no judge attempt"),
		CombinedUsage:        usage,
		Evidence:             evidencePath(state.attempt, evidenceSHA),
		EvidenceSHA256:       evidenceSHA,
		GradingRequestSHA256: state.attempt.Finished.GradingRequestSHA256,
		Error:                state.attempt.Finished.Error,
	}
	if state.attempt.Started != nil {
		result.StartedAt = state.attempt.Started.StartedAt
	}
	if state.attempt.Finished != nil {
		result.FinishedAt = state.attempt.Finished.FinishedAt
	}
	if evidenceSHA == "" {
		result.Evidence = ""
	}
	return writeJSON(state.store.ResultPath(), result)
}

func evidencePath(attempt *attempts.Attempt, digest string) string {
	if digest == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Join("attempts", attempt.ID, "evidence.json"))
}

func hasHookUnit(units []string) bool {
	for _, unit := range units {
		if strings.HasPrefix(unit, "hooks/") {
			return true
		}
	}
	return false
}

func (r *Runner) checksPresent() bool {
	info, err := os.Stat(filepath.Join(r.scenario.Dir, "checks.sh"))
	return err == nil && info.Mode()&0o111 != 0
}

func (r *Runner) configSHA() string {
	type configDigest struct {
		Backend       string   `json:"backend"`
		Command       string   `json:"command,omitempty"`
		Model         string   `json:"model,omitempty"`
		SkillDir      string   `json:"skill_dir"`
		Units         []string `json:"units"`
		PassEnv       []string `json:"pass_env,omitempty"`
		Timeout       string   `json:"timeout"`
		SetupTimeout  string   `json:"setup_timeout"`
		ChecksTimeout string   `json:"checks_timeout"`
		SnapshotLimit int64    `json:"snapshot_limit"`
	}
	identity := r.cfg.Executor.Identity()
	digest := configDigest{
		Backend:       identity.Kind,
		Command:       identity.Command,
		Model:         identity.Model,
		SkillDir:      r.cfg.SkillDir,
		Units:         r.cfg.Units,
		PassEnv:       r.cfg.PassEnv,
		Timeout:       r.cfg.Timeout.String(),
		SetupTimeout:  r.cfg.SetupTimeout.String(),
		ChecksTimeout: r.cfg.ChecksTimeout.String(),
		SnapshotLimit: r.cfg.MaxSnapshotBytes,
	}
	data, err := attempts.CanonicalJSON(digest)
	if err != nil {
		return ""
	}
	return attempts.SHA256Hex(data)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(path, append(data, '\n'), 0o600)
}

func fileSHA256(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return attempts.SHA256Hex(data)
}

func writeSummary(summary *Summary) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s/%s\n\n", summary.Plugin, summary.Scenario)
	fmt.Fprintf(&b, "Mode: %s  ·  runs: %d\n\n", summary.Mode, len(summary.Runs))
	fmt.Fprintln(&b, "| arm | run | status | mechanical |")
	fmt.Fprintln(&b, "| --- | --- | --- | --- |")
	for _, run := range summary.Runs {
		fmt.Fprintf(&b, "| %s | %d | %s | %g/%g |\n", run.Arm, run.Run, run.Status, run.Score, run.Max)
	}
	if summary.Mode == string(model.ModeFunctional) {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "Functional scenario: the with arm exercises the current plugin source;")
		fmt.Fprintln(&b, "there is no without baseline and no contribution delta.")
	}
	return atomicfile.Write(summary.Path, []byte(b.String()), 0o600)
}
