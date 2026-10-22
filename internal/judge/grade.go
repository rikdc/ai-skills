package judge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rikdc/ai-skills/internal/executor"
	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/platform/atomicfile"
	"github.com/rikdc/ai-skills/internal/platform/errs"
	"github.com/rikdc/ai-skills/internal/runner/attempts"
)

// Outcome classifies one grade invocation.
type Outcome string

const (
	// OutcomeGraded means a new judgement was accepted.
	OutcomeGraded Outcome = "graded"
	// OutcomeAlreadyComplete means a valid, bound judgement already exists.
	OutcomeAlreadyComplete Outcome = "already-complete"
	// OutcomeNothingPending means the grading request has no pending criteria.
	OutcomeNothingPending Outcome = "nothing-pending"
	// OutcomeSuperseded means a newer failed rerun replaced the accepted
	// attempt, so there is no current result to grade.
	OutcomeSuperseded Outcome = "superseded"
)

// Result is the outcome of grading one run directory.
type Result struct {
	RunDir   string  `json:"run_dir"`
	Outcome  Outcome `json:"outcome"`
	Attempt  string  `json:"judge_attempt,omitempty"`
	Score    float64 `json:"score"`
	Max      float64 `json:"max"`
	Evidence string  `json:"-"`
}

// Config describes one `evalctl grade` invocation. RunDir is used by Grade and
// SuiteDir by GradeSuite; exactly one is required.
type Config struct {
	RunDir   string
	SuiteDir string

	// Command is the exact command string passed to /bin/bash -c and hashed
	// into the canonical judge identity.
	Command string
	Timeout time.Duration
	Regrade bool

	// Executor executes the command judge. It is required.
	Executor executor.AgentExecutor
	// Store overrides the attempt store; tests use it.
	Store *attempts.Store
}

// SuiteResult aggregates one suite grading pass.
type SuiteResult struct {
	Runs            []RunResult
	Total           int
	Graded          int
	AlreadyComplete int
	Superseded      int
	Failed          int
}

// RunResult pairs one run with its outcome or error.
type RunResult struct {
	Result Result
	Err    error
}

const (
	judgePromptName    = "judge-prompt.md"
	judgeLogName       = "judge.log"
	judgementName      = "judgement.json"
	judgeResultName    = "judge-result.json"
	judgeUsageName     = "judge-usage.json"
	requestName        = "grading-request.json"
	prunedEvidenceName = "evidence_pruned.json"
)

// Grade grades one run directory. The accepted agent attempt and grading
// request are authoritative; a judgement is reused only when its identity and
// all three evidence bindings still match.
func Grade(ctx context.Context, cfg Config) (Result, error) {
	if strings.TrimSpace(cfg.RunDir) == "" {
		return Result{}, errs.Usage("--run-dir is required")
	}
	if strings.TrimSpace(cfg.Command) == "" {
		return Result{}, errs.Usage("--judge-cmd is required for the command judge")
	}
	if cfg.Executor == nil {
		return Result{}, errs.Usage("judge executor is required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Minute
	}
	runDir, err := filepath.Abs(cfg.RunDir)
	if err != nil {
		return Result{}, errs.Usage("resolve --run-dir: %v", err)
	}
	cfg.RunDir = runDir
	store := cfg.Store
	if store == nil {
		store = attempts.New(runDir)
	}
	empty := Result{RunDir: runDir}

	pointer, _, err := store.LoadCurrent()
	if err != nil {
		return empty, errs.Operational("%s: no accepted execution attempt: %v", runDir, err)
	}
	agent, err := store.LoadAttempt(pointer.AgentAttempt)
	if err != nil {
		return empty, errs.Operational("%s: load agent attempt: %v", runDir, err)
	}
	if agent.Kind != model.KindAgent && agent.Kind != model.KindHook {
		return empty, errs.Operational("%s: accepted attempt %s is not an execution attempt",
			runDir, agent.ID)
	}
	// Grading a replaced result would pay the judge for a score no reader shows.
	list, err := store.Enumerate()
	if err != nil {
		return empty, errs.Operational("%s: list attempts: %v", runDir, err)
	}
	if attempts.SupersedingAttempt(list, agent) != nil {
		return Result{RunDir: runDir, Outcome: OutcomeSuperseded}, nil
	}
	if agent.Finished == nil || agent.Finished.EvidenceSHA256 != pointer.AgentEvidenceSHA256 ||
		agent.Finished.GradingRequestSHA256 != pointer.AgentGradingRequestSHA256 {
		return empty, provenanceError(runDir, fmt.Errorf(
			"accepted execution attempt bindings do not match current.json"))
	}

	request, rawRequest, requestSHA, err := store.ReadGradingRequest()
	if err != nil {
		return empty, errs.Operational("%s: no readable grading-request.json: %v", runDir, err)
	}
	if requestSHA != pointer.AgentGradingRequestSHA256 ||
		request.AgentAttempt != pointer.AgentAttempt ||
		request.EvidenceSHA256 != pointer.AgentEvidenceSHA256 {
		return empty, provenanceError(runDir, fmt.Errorf(
			"grading-request.json bindings do not match current.json"))
	}
	if len(request.JudgementPending) == 0 {
		if err := refreshResult(store, request, agent); err != nil {
			return empty, errs.Operational("%s: refresh result projection: %v", runDir, err)
		}
		return Result{RunDir: runDir, Outcome: OutcomeNothingPending}, nil
	}

	identity := CommandIdentity(cfg.Command)
	identitySHA, err := IdentitySHA256(identity)
	if err != nil {
		return empty, errs.Operational("hash judge identity: %v", err)
	}

	pending := request.JudgementPending
	// A changed evidence file invalidates any previous judgement, even though
	// the pointer still carries the recorded digests.
	if err := attempts.VerifyEvidence(agent); err != nil {
		if cfg.Regrade || hasJudgeHistory(store) {
			return empty, provenanceError(runDir, err)
		}
		return empty, errs.Operational("%s: evidence is not verifiable: %v", runDir, err)
	}
	if !cfg.Regrade {
		if reused, ok, err := reuseJudgement(store, pointer, identitySHA, pending, request, agent); err != nil {
			return empty, err
		} else if ok {
			reused.RunDir = runDir
			return reused, nil
		}
	}

	// A fresh judge run needs intact evidence. Pruned evidence cannot be
	// regraded: the judge would be grading files that no longer exist.
	if pruned, err := prunedEvidence(agent.Dir); err != nil {
		return empty, errs.Operational("%s: read pruning marker: %v", runDir, err)
	} else if len(pruned) > 0 {
		return empty, errs.Operational(
			"%s: evidence was pruned (%s); cannot grade or regrade without it",
			runDir, strings.Join(pruned, ", "))
	}

	return runJudge(ctx, cfg, store, pointer, agent, request, rawRequest, identity, identitySHA)
}

func provenanceError(runDir string, err error) error {
	return errs.Usage("judgement provenance differs for %s: %v; rerun with --regrade", runDir, err)
}

// reuseJudgement returns an existing valid judgement when it is bound to the
// current execution and produced by the configured judge. A candidate that
// exists but does not match is a provenance error, never a silent rerun.
func reuseJudgement(store *attempts.Store, pointer model.CurrentPointer,
	identitySHA string, pending []model.Criterion,
	request model.GradingRequest, agent *attempts.Attempt) (Result, bool, error) {
	if pointer.JudgeAttempt != "" {
		judge, err := store.LoadAttempt(pointer.JudgeAttempt)
		if err != nil {
			return Result{}, true, provenanceError(store.RunDir,
				fmt.Errorf("load judge attempt %s: %v", pointer.JudgeAttempt, err))
		}
		judgement, err := checkJudgeReusable(judge, pointer, identitySHA, pending)
		if err != nil {
			return Result{}, true, provenanceError(store.RunDir, err)
		}
		if err := refreshResult(store, request, agent); err != nil {
			return Result{}, true, errs.Operational("%s: refresh result projection: %v", store.RunDir, err)
		}
		score, max := JudgementScore(judgement)
		return Result{Outcome: OutcomeAlreadyComplete, Attempt: judge.ID, Score: score, Max: max}, true, nil
	}

	latest, err := latestAcceptedJudge(store)
	if err != nil {
		return Result{}, false, errs.Operational("%s: enumerate attempts: %v", store.RunDir, err)
	}
	if latest == nil {
		return Result{}, false, nil
	}
	judgement, err := checkJudgeReusable(latest, pointer, identitySHA, pending)
	if err != nil {
		return Result{}, true, provenanceError(store.RunDir, err)
	}
	// A crash may have finished a valid judge attempt without replacing the
	// pointer. Adopting it is the same attempt, not a new invocation.
	if _, err := store.Accept(pointer.AgentAttempt, attempts.AcceptOptions{
		JudgeAttempt:        latest.ID,
		JudgeIdentitySHA256: identitySHA,
	}); err != nil {
		return Result{}, true, errs.Operational("%s: adopt judge attempt %s: %v",
			store.RunDir, latest.ID, err)
	}
	if err := refreshResult(store, request, agent); err != nil {
		return Result{}, true, errs.Operational("%s: refresh result projection: %v", store.RunDir, err)
	}
	score, max := JudgementScore(judgement)
	return Result{Outcome: OutcomeAlreadyComplete, Attempt: latest.ID, Score: score, Max: max}, true, nil
}

// latestAcceptedJudge returns the newest finished-ok judge attempt on disk, or
// nil when none exists.
func latestAcceptedJudge(store *attempts.Store) (*attempts.Attempt, error) {
	list, err := store.Enumerate()
	if err != nil {
		return nil, err
	}
	var latest *attempts.Attempt
	for _, attempt := range list {
		if attempt.Kind != model.KindJudge || attempt.Finished == nil {
			continue
		}
		if attempt.Finished.Status != model.StatusOK {
			continue
		}
		if latest == nil || attempt.Sequence > latest.Sequence {
			latest = attempt
		}
	}
	return latest, nil
}

func checkJudgeReusable(judge *attempts.Attempt, pointer model.CurrentPointer,
	identitySHA string, pending []model.Criterion) (model.Judgement, error) {
	if judge.Finished == nil {
		return nil, fmt.Errorf("judge attempt %s is interrupted", judge.ID)
	}
	if judge.Finished.Status != model.StatusOK {
		return nil, fmt.Errorf("judge attempt %s has status %q", judge.ID, judge.Finished.Status)
	}
	if judge.Finished.GradedAgentAttempt != pointer.AgentAttempt {
		return nil, fmt.Errorf("judge attempt %s graded agent attempt %q, not %q",
			judge.ID, judge.Finished.GradedAgentAttempt, pointer.AgentAttempt)
	}
	if judge.Finished.GradedEvidenceSHA256 != pointer.AgentEvidenceSHA256 {
		return nil, fmt.Errorf("judge attempt %s evidence binding does not match", judge.ID)
	}
	if judge.Finished.GradingRequestSHA256 != pointer.AgentGradingRequestSHA256 {
		return nil, fmt.Errorf("judge attempt %s grading-request binding does not match", judge.ID)
	}
	if judge.Finished.JudgeIdentitySHA256 != identitySHA {
		return nil, fmt.Errorf("judge attempt %s identity differs from the configured judge", judge.ID)
	}
	if judge.Finished.JudgementSHA256 == "" {
		return nil, fmt.Errorf("judge attempt %s has no judgement digest", judge.ID)
	}
	if err := attempts.VerifyJudgement(judge); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(judge.Dir, judgementName))
	if err != nil {
		return nil, fmt.Errorf("read judgement: %v", err)
	}
	return ValidateJudgement(raw, pending)
}

func hasJudgeHistory(store *attempts.Store) bool {
	list, err := store.Enumerate()
	if err != nil {
		return false
	}
	for _, attempt := range list {
		if attempt.Kind == model.KindJudge && attempt.Finished != nil {
			return true
		}
	}
	return false
}

func runJudge(ctx context.Context, cfg Config, store *attempts.Store,
	pointer model.CurrentPointer, agent *attempts.Attempt, request model.GradingRequest,
	rawRequest []byte, identity model.JudgeIdentity, identitySHA string) (Result, error) {
	runDir := cfg.RunDir
	empty := Result{RunDir: runDir}
	fingerprint := ""
	executorKind := "command"
	modelName := ""
	if agent.Started != nil {
		fingerprint = agent.Started.Fingerprint
	}
	if agent.Finished != nil {
		if agent.Finished.Fingerprint != "" {
			fingerprint = agent.Finished.Fingerprint
		}
	}
	started := model.StartedAttempt{
		Kind:         model.KindJudge,
		RunID:        fmt.Sprintf("%s/%s/%s/run-%d", request.Plugin, request.Scenario, request.Arm, request.Run),
		Fingerprint:  fingerprint,
		ConfigSHA256: identitySHA,
		Executor:     executorKind,
		Model:        modelName,
		Command:      cfg.Command,
	}
	attempt, err := store.Allocate(started)
	if err != nil {
		return empty, errs.Operational("%s: allocate judge attempt: %v", runDir, err)
	}

	prompt := BuildPrompt(request)
	if err := atomicfile.Write(filepath.Join(attempt.Dir, judgePromptName), []byte(prompt), 0o600); err != nil {
		return empty, errs.Operational("%s: write judge prompt: %v", runDir, err)
	}

	// The command judge runs in the accepted agent attempt's evidence
	// directory, exactly like the legacy contract. Make the grading request
	// readable there for judges that inspect it, and never inherit a stale
	// judgement.
	requestCopy := filepath.Join(agent.Dir, requestName)
	judgementInEvidence := filepath.Join(agent.Dir, judgementName)
	if err := atomicfile.Write(requestCopy, rawRequest, 0o600); err != nil {
		return empty, errs.Operational("%s: stage grading request for judge: %v", runDir, err)
	}
	if err := os.Remove(judgementInEvidence); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return empty, errs.Operational("%s: remove stale judgement: %v", runDir, err)
	}
	defer func() {
		_ = os.Remove(requestCopy)
		_ = os.Remove(judgementInEvidence)
	}()

	runResult, runErr := cfg.Executor.Run(ctx, executor.AgentRunRequest{
		Task:      prompt,
		Workspace: agent.Dir,
		HomeDir:   filepath.Join(attempt.Dir, "home"),
		LogDir:    attempt.Dir,
		LogFile:   judgeLogName,
		Kind:      model.KindJudge,
		Arm:       request.Arm,
		Run:       request.Run,
		Limits:    model.Limits{Timeout: cfg.Timeout},
	})

	usage := runResult.Usage.NormalizeState()
	if usage.State == "" {
		usage = model.NotApplicable("command")
	}

	judgementRaw, readErr := os.ReadFile(judgementInEvidence)
	judgementSHA := ""
	if readErr == nil {
		judgementSHA = attempts.SHA256Hex(judgementRaw)
		if err := atomicfile.Write(filepath.Join(attempt.Dir, judgementName), judgementRaw, 0o600); err != nil {
			return empty, errs.Operational("%s: store judgement: %v", runDir, err)
		}
	}
	// The command adapter writes judge.log; make sure a failed start still
	// leaves the diagnostic file the contract promises.
	if _, err := os.Stat(filepath.Join(attempt.Dir, judgeLogName)); errors.Is(err, fs.ErrNotExist) {
		_ = atomicfile.Write(filepath.Join(attempt.Dir, judgeLogName), nil, 0o600)
	}

	var (
		judgement     model.Judgement
		validationErr error
	)
	if runErr != nil {
		validationErr = runErr
	} else if runResult.Status != model.StatusOK {
		validationErr = fmt.Errorf("judge invocation failed: %s", judgeError(runResult))
	} else if readErr != nil {
		validationErr = fmt.Errorf("judge wrote no valid %s (see %s)", judgementName, judgeLogName)
	} else {
		judgement, validationErr = ValidateJudgement(judgementRaw, request.JudgementPending)
	}
	if validationErr != nil {
		appendJudgeLog(attempt.Dir, validationErr)
	}

	valid := validationErr == nil
	errorText := ""
	if validationErr != nil {
		errorText = validationErr.Error()
	}
	status := runResult.Status
	switch {
	case valid:
		status = model.StatusOK
	case status == "" || status == model.StatusOK:
		status = model.StatusAgentFailed
	}
	stopReason := ""
	switch {
	case valid:
		stopReason = model.StopCompleted
	case runResult.TimedOut || status == model.StatusTimedOut:
		stopReason = model.StopTimeout
	case runResult.Cancelled || status == model.StatusCancelled:
		stopReason = model.StopCancelled
	default:
		stopReason = "agent_error"
	}

	exitCode := -1
	if runResult.ExitCode != nil {
		exitCode = *runResult.ExitCode
	}
	duration := runResult.Duration.Seconds()
	judgeResult := model.JudgeResult{
		SchemaVersion:        model.SchemaVersion,
		ExitCode:             exitCode,
		TimedOut:             runResult.TimedOut,
		DurationSeconds:      &duration,
		Valid:                valid,
		Identity:             identity,
		JudgeIdentitySHA256:  identitySHA,
		JudgeCmd:             cfg.Command,
		Attempt:              attempt.ID,
		GradedAgentAttempt:   pointer.AgentAttempt,
		GradedEvidenceSHA256: pointer.AgentEvidenceSHA256,
		GradingRequestSHA256: pointer.AgentGradingRequestSHA256,
		JudgementSHA256:      judgementSHA,
		Error:                errorText,
	}
	if err := writeJSON(filepath.Join(attempt.Dir, judgeResultName), judgeResult); err != nil {
		return empty, errs.Operational("%s: write judge result: %v", runDir, err)
	}
	if err := writeJSON(filepath.Join(attempt.Dir, judgeUsageName), usage); err != nil {
		return empty, errs.Operational("%s: write judge usage: %v", runDir, err)
	}

	finished := model.FinishedAttempt{
		Status:               status,
		StopReason:           stopReason,
		Error:                errorText,
		Fingerprint:          fingerprint,
		Usage:                usage,
		ChecksStatus:         model.ChecksNotApplicable,
		GradedAgentAttempt:   pointer.AgentAttempt,
		GradedEvidenceSHA256: pointer.AgentEvidenceSHA256,
		GradingRequestSHA256: pointer.AgentGradingRequestSHA256,
		JudgeIdentitySHA256:  identitySHA,
		JudgementSHA256:      judgementSHA,
		TimedOut:             runResult.TimedOut,
		AgentExit:            runResult.ExitCode,
		DurationSeconds:      &duration,
	}
	if _, err := store.Finish(attempt.ID, finished); err != nil {
		return empty, errs.Operational("%s: finish judge attempt %s: %v", runDir, attempt.ID, err)
	}
	result := Result{RunDir: runDir, Attempt: attempt.ID}
	if !valid {
		return result, errs.Operational("%s: judge failed: %s (see %s)", runDir, errorText,
			filepath.Join(attempt.Dir, judgeLogName))
	}

	if _, err := store.Accept(pointer.AgentAttempt, attempts.AcceptOptions{
		JudgeAttempt:        attempt.ID,
		JudgeIdentitySHA256: identitySHA,
	}); err != nil {
		return result, errs.Operational("%s: accept judge attempt %s: %v", runDir, attempt.ID, err)
	}
	if err := refreshResult(store, request, agent); err != nil {
		return result, errs.Operational("%s: refresh result projection: %v", runDir, err)
	}

	score, max := JudgementScore(judgement)
	result.Outcome = OutcomeGraded
	result.Score = score
	result.Max = max
	return result, nil
}

func judgeError(result executor.AgentRunResult) string {
	if result.Error != "" {
		return result.Error
	}
	if result.ExitCode != nil {
		return fmt.Sprintf("exit %d", *result.ExitCode)
	}
	return string(result.Status)
}

func appendJudgeLog(attemptDir string, err error) {
	path := filepath.Join(attemptDir, judgeLogName)
	file, openErr := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if openErr != nil {
		return
	}
	defer file.Close()
	fmt.Fprintf(file, "\njudgement validation failed: %v\n", err)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicfile.Write(path, data, 0o600)
}

// refreshResult rebuilds result.json from current.json after a judgement is
// accepted (or reused). Scenario metadata that the grading request does not
// carry (mode) is preserved from the previous projection.
func refreshResult(store *attempts.Store, request model.GradingRequest, agent *attempts.Attempt) error {
	ctx := attempts.RunContext{
		Plugin:   request.Plugin,
		Scenario: request.Scenario,
		Arm:      request.Arm,
		Run:      request.Run,
	}
	var previous model.RunResult
	if data, err := os.ReadFile(store.ResultPath()); err == nil {
		if json.Unmarshal(data, &previous) == nil {
			ctx.Mode = previous.Mode
			ctx.ChecksRequired = previous.ChecksRequired
			ctx.EvidencePath = previous.Evidence
			// Skill use was detected when the agent ran; the log may since
			// have been pruned, so carry the recorded answer forward.
			ctx.SkillUse = previous.SkillUse
		}
	}
	// A suite run's evidence link is relative to the suite root (the parent of
	// its `runs` directory), matching reportio and the legacy report. A plain
	// run keeps the evidence path its previous projection used.
	if rel := suiteRelativePath(store.RunDir); rel != "" {
		ctx.EvidencePath = rel
	}
	if agent.Finished != nil &&
		(agent.Finished.ChecksStatus == model.ChecksOK || agent.Finished.ChecksStatus == model.ChecksFailed) {
		ctx.ChecksRequired = true
	}
	_, err := store.EnsureResultProjection(ctx)
	return err
}

// suiteRelativePath returns a run directory's path relative to its suite root
// when the run lives under a `runs/` directory.
func suiteRelativePath(runDir string) string {
	dir := filepath.Clean(runDir)
	for {
		if filepath.Base(dir) == "runs" {
			rel, err := filepath.Rel(filepath.Dir(dir), runDir)
			if err != nil {
				return ""
			}
			return filepath.ToSlash(rel)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// prunedEvidence returns the paths recorded in evidence_pruned.json.
func prunedEvidence(attemptDir string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(attemptDir, prunedEvidenceName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var marker struct {
		Paths []string `json:"paths"`
	}
	if err := json.Unmarshal(data, &marker); err != nil {
		return nil, err
	}
	return marker.Paths, nil
}

// GradeSuite grades every run under a suite directory that has a grading
// request. It continues past failures and reports counts; provenance failures
// are surfaced with exit code 2.
func GradeSuite(ctx context.Context, cfg Config) (SuiteResult, error) {
	if strings.TrimSpace(cfg.SuiteDir) == "" {
		return SuiteResult{}, errs.Usage("--suite is required")
	}
	suiteDir, err := filepath.Abs(cfg.SuiteDir)
	if err != nil {
		return SuiteResult{}, errs.Usage("resolve --suite: %v", err)
	}
	info, err := os.Stat(suiteDir)
	if err != nil || !info.IsDir() {
		return SuiteResult{}, errs.Usage("no such suite directory: %s", suiteDir)
	}
	patterns := []string{
		filepath.Join(suiteDir, "runs", "*", "*", "*", "run-*", requestName),
		filepath.Join(suiteDir, "*", "*", "*", "run-*", requestName),
		filepath.Join(suiteDir, "*", "*", "run-*", requestName),
	}
	seen := map[string]bool{}
	var matches []string
	for _, pattern := range patterns {
		found, err := filepath.Glob(pattern)
		if err != nil {
			return SuiteResult{}, errs.Operational("scan suite: %v", err)
		}
		for _, match := range found {
			runDir := filepath.Dir(match)
			if seen[runDir] {
				continue
			}
			seen[runDir] = true
			matches = append(matches, runDir)
		}
	}
	sort.Strings(matches)

	var suite SuiteResult
	provenance := false
	firstProvenance := ""
	for _, runDir := range matches {
		// Grading more runs after cancellation would only record cancelled
		// judge attempts, which later read as judge history.
		if ctx.Err() != nil {
			break
		}
		runCfg := cfg
		runCfg.RunDir = runDir
		runCfg.SuiteDir = ""
		result, gradeErr := Grade(ctx, runCfg)
		suite.Total++
		suite.Runs = append(suite.Runs, RunResult{Result: result, Err: gradeErr})
		if gradeErr != nil {
			suite.Failed++
			if errs.Code(gradeErr) == errs.CodeUsage {
				provenance = true
				if firstProvenance == "" {
					firstProvenance = gradeErr.Error()
				}
			}
			continue
		}
		switch result.Outcome {
		case OutcomeGraded:
			suite.Graded++
		case OutcomeSuperseded:
			suite.Superseded++
		default:
			suite.AlreadyComplete++
		}
	}
	switch {
	case ctx.Err() != nil:
		return suite, errs.Operational("grading cancelled after %d of %d runs: %v",
			suite.Total, len(matches), ctx.Err())
	case provenance:
		return suite, errs.WithCode(errs.CodeUsage, fmt.Errorf(
			"%s (graded: %d, already complete: %d, superseded: %d, failed: %d)",
			firstProvenance, suite.Graded, suite.AlreadyComplete, suite.Superseded, suite.Failed))
	case suite.Failed > 0:
		return suite, errs.Operational("grading failed for %d of %d runs",
			suite.Failed, suite.Total)
	}
	return suite, nil
}
