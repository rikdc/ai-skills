// Package reportio loads suite artifacts into model values for the pure report
// analysis. It is read-only: it never creates, truncates, or rewrites a suite
// file, and it never runs an agent, judge, or network call.
package reportio

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rikdc/ai-skills/internal/model"
)

// Options controls loading. RepoRoot is only used for the legacy rubric
// fallback (plugins/<plugin>/evals/<scenario>/criteria.json); an empty root
// disables the fallback.
type Options struct {
	RepoRoot string
}

type manifestFile struct {
	Version   int            `json:"version"`
	CreatedAt string         `json:"created_at"`
	Tool      model.ToolInfo `json:"tool"`
	Repo      repoFile       `json:"repo"`
	Config    configFile     `json:"config"`
	Scenarios []plannedFile  `json:"scenarios"`
}

type repoFile struct {
	Root     string `json:"root"`
	GitSHA   string `json:"git_sha"`
	GitDirty bool   `json:"git_dirty"`
}

type configFile struct {
	Backend        string            `json:"backend"`
	Model          string            `json:"model"`
	AgentCmd       string            `json:"agent_cmd"`
	JudgeCmd       string            `json:"judge_cmd"`
	Arm            string            `json:"arm"`
	Runs           int               `json:"runs"`
	Units          []string          `json:"units"`
	SkillDir       string            `json:"skill_dir"`
	GitSHA         string            `json:"git_sha"`
	GitDirty       bool              `json:"git_dirty"`
	HarnessVersion string            `json:"harness_version"`
	Judge          model.JudgeConfig `json:"judge"`
	UsageCmd       string            `json:"usage_cmd"`
	JudgeUsageCmd  string            `json:"judge_usage_cmd"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	AuthCLI        bool              `json:"auth_cli"`
	Keychain       bool              `json:"keychain"`
	AuthPaths      []string          `json:"auth_paths"`
}

type plannedFile struct {
	Plugin      string   `json:"plugin"`
	Scenario    string   `json:"scenario"`
	Fingerprint string   `json:"fingerprint"`
	Units       []string `json:"units"`
	Mode        string   `json:"mode"`
	Execution   string   `json:"execution"`
}

type suiteFile struct {
	Counts          map[string]json.Number `json:"counts"`
	DurationSeconds *float64               `json:"duration_seconds"`
}

type resultsRow struct {
	Plugin   string `json:"plugin"`
	Scenario string `json:"scenario"`
	Status   string `json:"status"`
}

type legacyResult struct {
	SchemaVersion   int              `json:"schema_version"`
	Plugin          string           `json:"plugin"`
	Scenario        string           `json:"scenario"`
	Arm             string           `json:"arm"`
	Mode            string           `json:"mode"`
	Run             int              `json:"run"`
	AgentAttempt    string           `json:"agent_attempt"`
	JudgeAttempt    string           `json:"judge_attempt"`
	CurrentSHA256   string           `json:"current_sha256"`
	Complete        bool             `json:"complete"`
	Status          string           `json:"status"`
	StopReason      string           `json:"stop_reason"`
	AgentExit       *json.RawMessage `json:"agent_exit"`
	TimedOut        bool             `json:"timed_out"`
	DurationSeconds *float64         `json:"duration_seconds"`
	Mechanical      model.Mechanical `json:"mechanical"`
	Usage           model.Usage      `json:"usage"`
	AgentUsage      model.Usage      `json:"agent_usage"`
	JudgeUsage      model.Usage      `json:"judge_usage"`
	CombinedUsage   model.Usage      `json:"combined_usage"`
	Evidence        string           `json:"evidence"`
}

type judgeResultFile struct {
	ExitCode        *int     `json:"exit_code"`
	TimedOut        bool     `json:"timed_out"`
	DurationSeconds *float64 `json:"duration_seconds"`
	Valid           bool     `json:"valid"`
}

type startedFile struct {
	AttemptID string            `json:"attempt_id"`
	Kind      model.AttemptKind `json:"kind"`
	StartedAt string            `json:"started_at"`
}

type finishedFile struct {
	AttemptID            string            `json:"attempt_id"`
	Kind                 model.AttemptKind `json:"kind"`
	FinishedAt           string            `json:"finished_at"`
	Status               model.RunStatus   `json:"status"`
	StopReason           string            `json:"stop_reason"`
	Usage                model.Usage       `json:"usage"`
	GradedAgentAttempt   string            `json:"graded_agent_attempt"`
	GradedEvidenceSHA256 string            `json:"graded_evidence_sha256"`
	GradingRequestSHA256 string            `json:"grading_request_sha256"`
	JudgeIdentitySHA256  string            `json:"judge_identity_sha256"`
}

// Load reads a suite directory, accepting v1/v2 (legacy) and v3 layouts.
func Load(suiteDir string, opts Options) (*model.ReportInput, error) {
	abs, err := filepath.Abs(suiteDir)
	if err != nil {
		return nil, fmt.Errorf("resolve suite directory: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("no such suite directory: %s", suiteDir)
	}
	for _, required := range []string{"manifest.json", "suite.json", "results.jsonl"} {
		if _, err := os.Stat(filepath.Join(abs, required)); err != nil {
			return nil, fmt.Errorf("%s is not a suite directory: %s is missing", abs, required)
		}
	}

	var mf manifestFile
	if err := readJSON(filepath.Join(abs, "manifest.json"), &mf); err != nil {
		return nil, fmt.Errorf("manifest.json is not valid JSON: %w", err)
	}
	if len(mf.Scenarios) == 0 {
		return nil, fmt.Errorf("manifest.json lists no scenarios")
	}
	var sf suiteFile
	if err := readJSON(filepath.Join(abs, "suite.json"), &sf); err != nil {
		return nil, fmt.Errorf("suite.json is not valid JSON: %w", err)
	}
	results, err := readResults(filepath.Join(abs, "results.jsonl"))
	if err != nil {
		return nil, err
	}

	runs := mf.Config.Runs
	arm := strings.TrimSpace(mf.Config.Arm)
	if arm == "" {
		arm = "both"
	}

	in := &model.ReportInput{
		SuiteDir:        abs,
		ManifestVersion: mf.Version,
		Manifest: model.SuiteManifest{
			Version:   mf.Version,
			CreatedAt: mf.CreatedAt,
			Tool:      mf.Tool,
			Repo: model.RepoInfo{
				Root:     mf.Repo.Root,
				GitSHA:   firstNonEmpty(mf.Repo.GitSHA, mf.Config.GitSHA, "nogit"),
				GitDirty: mf.Repo.GitDirty || mf.Config.GitDirty,
			},
			Config: model.SuiteConfig{
				Backend:        mf.Config.Backend,
				Model:          mf.Config.Model,
				AgentCmd:       mf.Config.AgentCmd,
				Arm:            arm,
				Runs:           runs,
				Units:          append([]string(nil), mf.Config.Units...),
				SkillDir:       mf.Config.SkillDir,
				Judge:          judgeConfig(mf.Config),
				UsageCmd:       mf.Config.UsageCmd,
				JudgeUsageCmd:  mf.Config.JudgeUsageCmd,
				TimeoutSeconds: mf.Config.TimeoutSeconds,
				AuthCLI:        mf.Config.AuthCLI,
				Keychain:       mf.Config.Keychain,
				AuthPaths:      append([]string(nil), mf.Config.AuthPaths...),
			},
		},
		Results: results,
	}
	in.State.Counts = normalizeCounts(sf.Counts)
	in.State.DurationSeconds = sf.DurationSeconds
	if in.State.DurationSeconds == nil {
		if raw, ok := sf.Counts["duration_seconds"]; ok {
			if f, err := raw.Float64(); err == nil {
				in.State.DurationSeconds = &f
			}
		}
	}

	for _, p := range mf.Scenarios {
		mode := p.Mode
		if mode == "" {
			mode = string(model.ModeComparison)
		}
		rubricPath, err := findCriteria(abs, opts.RepoRoot, p.Plugin, p.Scenario)
		if err != nil {
			return nil, err
		}
		var rubric model.Rubric
		if err := readJSON(rubricPath, &rubric); err != nil {
			return nil, fmt.Errorf("invalid rubric for %s/%s: %v", p.Plugin, p.Scenario, err)
		}
		if len(rubric.Checklist) == 0 {
			return nil, fmt.Errorf("invalid rubric for %s/%s: %s", p.Plugin, p.Scenario, rubricPath)
		}
		for _, item := range rubric.Checklist {
			if item.MaxScore <= 0 {
				return nil, fmt.Errorf("rubric for %s/%s has no positive max_score sum", p.Plugin, p.Scenario)
			}
		}
		sc := model.ScenarioInput{
			Plugin:      p.Plugin,
			Scenario:    p.Scenario,
			Units:       append([]string(nil), p.Units...),
			Mode:        mode,
			Execution:   p.Execution,
			Fingerprint: p.Fingerprint,
			Rubric:      rubric.Checklist,
		}
		declaredRuns := runs
		if declaredRuns <= 0 {
			declaredRuns = 1
		}
		for _, runArm := range armsFor(mode, arm) {
			for run := 1; run <= declaredRuns; run++ {
				ri, err := loadRun(abs, p.Plugin, p.Scenario, runArm, run, rubric.Checklist)
				if err != nil {
					return nil, err
				}
				sc.Runs = append(sc.Runs, ri)
			}
		}
		in.Scenarios = append(in.Scenarios, sc)
	}

	// Attempts are enumerated after scenarios so a v1 suite can synthesize
	// exactly the attempts its run artifacts represent.
	for _, sc := range in.Scenarios {
		for _, run := range sc.Runs {
			runDir := filepath.Join(abs, "runs", sc.Plugin, sc.Scenario, run.Arm,
				fmt.Sprintf("run-%d", run.Run))
			attempts, err := loadAttempts(runDir)
			if err != nil {
				return nil, err
			}
			if attempts != nil {
				in.Attempts = append(in.Attempts, attempts...)
				continue
			}
			// Legacy v1/v2: synthesize the read-only attempts the artifacts
			// represent. They are never written back or used for resume.
			if run.Status == string(model.StatusMissing) {
				continue
			}
			in.Attempts = append(in.Attempts, model.AttemptInput{
				Kind:  model.KindAgent,
				Usage: run.AgentUsage,
			})
			if _, err := os.Stat(filepath.Join(runDir, "judge-result.json")); err == nil {
				in.Attempts = append(in.Attempts, model.AttemptInput{
					Kind:  model.KindJudge,
					Usage: run.JudgeUsage,
				})
			} else if _, err := os.Stat(filepath.Join(runDir, "judgement.json")); err == nil {
				in.Attempts = append(in.Attempts, model.AttemptInput{
					Kind:  model.KindJudge,
					Usage: run.JudgeUsage,
				})
			}
		}
	}
	return in, nil
}

func judgeConfig(config configFile) model.JudgeConfig {
	judge := config.Judge
	if judge.Cmd == "" {
		judge.Cmd = config.JudgeCmd
	}
	return judge
}

func armsFor(mode, arm string) []string {
	if mode == string(model.ModeFunctional) {
		return []string{"with"}
	}
	switch arm {
	case "with":
		return []string{"with"}
	case "without":
		return []string{"without"}
	default:
		return []string{"with", "without"}
	}
}

func findCriteria(suiteDir, repoRoot, plugin, scenario string) (string, error) {
	candidates := []string{filepath.Join(suiteDir, "criteria", plugin, scenario+".json")}
	if repoRoot != "" {
		candidates = append(candidates, filepath.Join(repoRoot, "plugins", plugin, "evals", scenario, "criteria.json"))
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no criteria.json for %s/%s (looked in the suite and the repository)", plugin, scenario)
}

func loadRun(suiteDir, plugin, scenario, arm string, run int, rubric []model.Criterion) (model.RunInput, error) {
	ri := model.RunInput{
		Arm:           arm,
		Run:           run,
		Status:        string(model.StatusMissing),
		Usage:         notApplicableUsage(),
		JudgeUsage:    notApplicableUsage(),
		CombinedUsage: notApplicableUsage(),
	}
	rel := filepath.Join("runs", plugin, scenario, arm, fmt.Sprintf("run-%d", run))
	ri.Evidence = filepath.ToSlash(rel)
	runDir := filepath.Join(suiteDir, rel)
	info, err := os.Stat(runDir)
	if err != nil || !info.IsDir() {
		return ri, nil
	}
	if _, err := os.Stat(filepath.Join(runDir, "current.json")); err == nil {
		return loadV3Run(runDir, rel, ri, rubric)
	}
	if _, err := os.Stat(filepath.Join(runDir, "attempts")); err == nil {
		// v3 run without an acceptance pointer: unaccepted, incomplete.
		ri.Status = string(model.StatusIncomplete)
		ri.Usage = unknownUsage("agent")
		ri.JudgeUsage = notApplicableUsage()
		ri.CombinedUsage = unknownUsage("combined")
		return ri, nil
	}
	return loadLegacyRun(runDir, rel, ri, rubric)
}

func loadLegacyRun(runDir, rel string, ri model.RunInput, rubric []model.Criterion) (model.RunInput, error) {
	var result legacyResult
	resultPath := filepath.Join(runDir, "result.json")
	if err := readJSON(resultPath, &result); err != nil {
		if os.IsNotExist(err) {
			return ri, nil
		}
		return ri, fmt.Errorf("invalid result.json in %s: %w", runDir, err)
	}
	ri.Status = string(model.StatusOK)
	ri.Complete = true
	ri.AgentExit = parseIntRaw(result.AgentExit)
	ri.TimedOut = result.TimedOut
	ri.DurationSeconds = result.DurationSeconds
	ri.StopReason = result.StopReason
	ri.AgentAttempt = result.AgentAttempt
	ri.JudgeAttempt = result.JudgeAttempt

	if judgeDuration, err := readJudgeDuration(runDir); err == nil {
		ri.JudgeDurationSeconds = judgeDuration
	}

	items, err := readMechanical(runDir, result.Mechanical.Items)
	if err != nil {
		return ri, err
	}
	ri.MechanicalItems = items

	if judged, ok, err := readValidJudgement(
		filepath.Join(runDir, "judgement.json"),
		filepath.Join(runDir, "grading-request.json"),
		rubric,
	); err != nil {
		return ri, err
	} else if ok {
		ri.JudgedItems = judged
	}

	ri.AgentUsage = readUsageFile(filepath.Join(runDir, "usage.json"), true)
	ri.Usage = ri.AgentUsage
	judgeInvoked := fileExists(filepath.Join(runDir, "judge-result.json")) ||
		fileExists(filepath.Join(runDir, "judgement.json"))
	if judgeInvoked {
		ri.JudgeUsage = readUsageFile(filepath.Join(runDir, "judge-usage.json"), true)
	} else {
		ri.JudgeUsage = notApplicableUsage()
	}
	ri.CombinedUsage = combineUsage(ri.AgentUsage, ri.JudgeUsage)
	return ri, nil
}

func loadV3Run(runDir, rel string, ri model.RunInput, rubric []model.Criterion) (model.RunInput, error) {
	var current model.CurrentPointer
	if err := readJSON(filepath.Join(runDir, "current.json"), &current); err != nil {
		return ri, fmt.Errorf("invalid current.json in %s: %w", runDir, err)
	}
	ri.AgentAttempt = current.AgentAttempt
	ri.JudgeAttempt = current.JudgeAttempt
	ri.Complete = true
	ri.Status = string(model.StatusOK)

	agentDir := filepath.Join(runDir, "attempts", current.AgentAttempt)
	started, finished, err := readAttempt(agentDir)
	if err != nil {
		return ri, err
	}
	_ = started
	if finished == nil {
		ri.Status = string(model.StatusInterrupted)
		ri.Complete = false
		ri.StopReason = model.StopInterrupted
		ri.AgentUsage = unknownUsage("agent")
		ri.Usage = ri.AgentUsage
		ri.JudgeUsage = judgeUsageFor(runDir, current.JudgeAttempt)
		ri.CombinedUsage = combineUsage(ri.AgentUsage, ri.JudgeUsage)
		return ri, nil
	}
	ri.Status = string(finished.Status)
	if ri.Status == "" {
		ri.Status = string(model.StatusOK)
	}
	ri.StopReason = finished.StopReason
	ri.TimedOut = finished.Status == model.StatusTimedOut || finished.StopReason == model.StopTimeout

	ri.AgentUsage = readAttemptUsage(agentDir, model.KindAgent, finished)
	ri.Usage = ri.AgentUsage

	// The projection supplies fields the attempt contract does not record.
	var result legacyResult
	resultPath := filepath.Join(runDir, "result.json")
	projectionValid := false
	if err := readJSON(resultPath, &result); err == nil {
		projectionValid = result.AgentAttempt == "" || result.AgentAttempt == current.AgentAttempt
	}
	if projectionValid && result.AgentAttempt == current.AgentAttempt {
		ri.AgentExit = parseIntRaw(result.AgentExit)
		if result.DurationSeconds != nil {
			ri.DurationSeconds = result.DurationSeconds
		}
		if !ri.TimedOut && result.TimedOut {
			ri.TimedOut = true
		}
	}
	if ri.DurationSeconds == nil {
		ri.DurationSeconds = durationBetween(started, finished)
	}

	var fallbackItems []model.JudgedItem
	if projectionValid && result.AgentAttempt == current.AgentAttempt {
		fallbackItems = result.Mechanical.Items
	}
	items, err := readMechanical(agentDir, fallbackItems)
	if err != nil {
		return ri, err
	}
	ri.MechanicalItems = items

	if current.JudgeAttempt != "" {
		judgeDir := filepath.Join(runDir, "attempts", current.JudgeAttempt)
		judgeStarted, judgeFinished, err := readAttempt(judgeDir)
		if err != nil {
			return ri, err
		}
		_ = judgeStarted
		if judgeFinished != nil && judgeBindingValid(current, judgeFinished) {
			if judged, ok, err := readValidJudgement(
				filepath.Join(judgeDir, "judgement.json"),
				filepath.Join(runDir, "grading-request.json"),
				rubric,
			); err != nil {
				return ri, err
			} else if ok {
				ri.JudgedItems = judged
			}
			ri.JudgeUsage = readAttemptUsage(judgeDir, model.KindJudge, judgeFinished)
			if ri.JudgeUsage.State == "" {
				ri.JudgeUsage = unknownUsage("judge")
			}
		} else {
			ri.JudgeUsage = unknownUsage("judge")
		}
		if judgeDuration, err := readJudgeDuration(runDir); err == nil {
			ri.JudgeDurationSeconds = judgeDuration
		} else if judgeFinished != nil {
			ri.JudgeDurationSeconds = durationBetween(judgeStarted, judgeFinished)
		}
	} else {
		ri.JudgeUsage = notApplicableUsage()
	}
	ri.CombinedUsage = combineUsage(ri.AgentUsage, ri.JudgeUsage)
	if result.Evidence != "" {
		ri.Evidence = result.Evidence
	}
	if ri.Status != string(model.StatusOK) {
		// Accepted usage covers only successful accepted runs; a failed
		// attempt still contributes to expenditure.
		ri.Usage = notApplicableUsage()
		ri.AgentUsage = notApplicableUsage()
		ri.JudgeUsage = notApplicableUsage()
		ri.CombinedUsage = notApplicableUsage()
	}
	return ri, nil
}

func judgeBindingValid(current model.CurrentPointer, finished *finishedFile) bool {
	if current.GradedAgentAttempt != "" && finished.GradedAgentAttempt != current.GradedAgentAttempt {
		return false
	}
	if current.AgentEvidenceSHA256 != "" && finished.GradedEvidenceSHA256 != current.AgentEvidenceSHA256 {
		return false
	}
	wantRequest := firstNonEmpty(current.GradingRequestSHA256, current.AgentGradingRequestSHA256)
	if wantRequest != "" && finished.GradingRequestSHA256 != wantRequest {
		return false
	}
	if current.JudgeIdentitySHA256 != "" && finished.JudgeIdentitySHA256 != current.JudgeIdentitySHA256 {
		return false
	}
	return true
}

func judgeUsageFor(runDir, judgeAttempt string) model.Usage {
	if judgeAttempt == "" {
		return notApplicableUsage()
	}
	judgeDir := filepath.Join(runDir, "attempts", judgeAttempt)
	_, finished, err := readAttempt(judgeDir)
	if err != nil || finished == nil {
		return unknownUsage("judge")
	}
	usage := readAttemptUsage(judgeDir, model.KindJudge, finished)
	if usage.State == "" {
		return unknownUsage("judge")
	}
	return usage
}

func readAttempt(dir string) (*startedFile, *finishedFile, error) {
	var started startedFile
	startedExists := false
	if err := readJSON(filepath.Join(dir, "started.json"), &started); err == nil {
		startedExists = true
	} else if !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("invalid started.json in %s: %w", dir, err)
	}
	var finished finishedFile
	if err := readJSON(filepath.Join(dir, "finished.json"), &finished); err == nil {
		return &started, &finished, nil
	} else if !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("invalid finished.json in %s: %w", dir, err)
	}
	if startedExists {
		return &started, nil, nil
	}
	return nil, nil, nil
}

// loadAttempts enumerates immutable attempt directories for v3 runs. A nil
// slice means the run is not a v3 attempt layout.
func loadAttempts(runDir string) ([]model.AttemptInput, error) {
	attemptsDir := filepath.Join(runDir, "attempts")
	entries, err := os.ReadDir(attemptsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read attempts in %s: %w", runDir, err)
	}
	var out []model.AttemptInput
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(attemptsDir, entry.Name())
		kind := kindFromDirName(entry.Name())
		started, finished, err := readAttempt(dir)
		if err != nil {
			return nil, err
		}
		if started != nil && started.Kind != "" {
			kind = started.Kind
		}
		if kind == "" {
			kind = model.KindAgent
		}
		usage := model.Usage{State: model.UsageUnknown, Known: false, Source: "attempt"}
		if finished == nil {
			// Started without finished: interrupted, unknown, never zero.
			out = append(out, model.AttemptInput{Kind: kind, Usage: usage})
			continue
		}
		usage = readAttemptUsage(dir, kind, finished)
		if usage.State == "" {
			usage = unknownUsage(string(kind))
		}
		out = append(out, model.AttemptInput{Kind: kind, Usage: usage})
	}
	return out, nil
}

func kindFromDirName(name string) model.AttemptKind {
	if i := strings.LastIndex(name, "-"); i > 0 {
		switch model.AttemptKind(name[:i]) {
		case model.KindAgent, model.KindHook, model.KindJudge:
			return model.AttemptKind(name[:i])
		}
	}
	switch {
	case strings.HasPrefix(name, "judge-"):
		return model.KindJudge
	case strings.HasPrefix(name, "hook-"):
		return model.KindHook
	case strings.HasPrefix(name, "agent-"):
		return model.KindAgent
	}
	return ""
}

func readAttemptUsage(dir string, kind model.AttemptKind, finished *finishedFile) model.Usage {
	names := []string{"usage.json"}
	if kind == model.KindJudge {
		names = []string{"judge-usage.json", "usage.json"}
	}
	for _, name := range names {
		if usage, ok := readUsageFileFound(filepath.Join(dir, name)); ok {
			return usage
		}
	}
	if finished != nil && finished.Usage.State != "" {
		return finished.Usage
	}
	return model.Usage{}
}

func readMechanical(runDir string, fallback []model.JudgedItem) ([]model.JudgedItem, error) {
	path := filepath.Join(runDir, "mechanical.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fallback, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var items []model.JudgedItem
	if err := json.Unmarshal(data, &items); err != nil {
		return fallback, nil
	}
	return items, nil
}

func readValidJudgement(judgementPath, requestPath string, rubric []model.Criterion) ([]model.JudgedItem, bool, error) {
	var judgement []model.JudgedItem
	if err := readJSON(judgementPath, &judgement); err != nil {
		return nil, false, nil
	}
	var request model.GradingRequest
	if err := readJSON(requestPath, &request); err != nil {
		return nil, false, nil
	}
	if len(judgement) == 0 || !judgementMatchesRequest(judgement, request.JudgementPending, rubric) {
		return nil, false, nil
	}
	return judgement, true, nil
}

func judgementMatchesRequest(judgement []model.JudgedItem, pending []model.Criterion, rubric []model.Criterion) bool {
	if len(judgement) != len(pending) {
		return false
	}
	pendingByName := make(map[string]float64, len(pending))
	for _, item := range pending {
		pendingByName[item.Name] = item.MaxScore
	}
	rubricByName := make(map[string]float64, len(rubric))
	for _, item := range rubric {
		rubricByName[item.Name] = item.MaxScore
	}
	seen := make(map[string]bool, len(judgement))
	for _, item := range judgement {
		if item.Name == "" || seen[item.Name] {
			return false
		}
		seen[item.Name] = true
		wantMax, ok := pendingByName[item.Name]
		if !ok || item.Max != wantMax {
			return false
		}
		if rubricMax, ok := rubricByName[item.Name]; ok && rubricMax != item.Max {
			return false
		}
		if item.Score < 0 || item.Score > item.Max {
			return false
		}
	}
	return true
}

func readJudgeDuration(runDir string) (*float64, error) {
	var result judgeResultFile
	if err := readJSON(filepath.Join(runDir, "judge-result.json"), &result); err != nil {
		return nil, err
	}
	return result.DurationSeconds, nil
}

func readUsageFile(path string, agent bool) model.Usage {
	usage, ok := readUsageFileFound(path)
	if !ok {
		if agent {
			return unknownUsage("agent")
		}
		return unknownUsage("judge")
	}
	return usage
}

func readUsageFileFound(path string) (model.Usage, bool) {
	var usage model.Usage
	if err := readJSON(path, &usage); err != nil {
		return model.Usage{}, false
	}
	if usage.State == "" {
		if usage.Known {
			usage.State = model.UsageKnown
		} else {
			usage.State = model.UsageUnknown
		}
	}
	return usage, true
}

func combineUsage(agent, judge model.Usage) model.Usage {
	if judge.State == model.UsageNotApplicable {
		return agent
	}
	if agent.State == model.UsageNotApplicable {
		return judge
	}
	if agent.State != model.UsageKnown || judge.State != model.UsageKnown {
		return unknownUsage("combined")
	}
	out := model.Usage{State: model.UsageKnown, Known: true, Source: "combined"}
	out.InputTokens = sumInt64(agent.InputTokens, judge.InputTokens)
	out.OutputTokens = sumInt64(agent.OutputTokens, judge.OutputTokens)
	out.ReasoningTokens = sumInt64(agent.ReasoningTokens, judge.ReasoningTokens)
	out.CacheReadTokens = sumInt64(agent.CacheReadTokens, judge.CacheReadTokens)
	out.CacheWriteTokens = sumInt64(agent.CacheWriteTokens, judge.CacheWriteTokens)
	out.TotalTokens = sumInt64(agent.TotalTokens, judge.TotalTokens)
	out.CostUSD = sumFloat64(agent.CostUSD, judge.CostUSD)
	out.Currency = firstNonEmpty(agent.Currency, judge.Currency)
	return out
}

func sumInt64(a, b *int64) *int64 {
	if a == nil || b == nil {
		return nil
	}
	v := *a + *b
	return &v
}

func sumFloat64(a, b *float64) *float64 {
	if a == nil || b == nil {
		return nil
	}
	v := *a + *b
	return &v
}

func notApplicableUsage() model.Usage {
	return model.Usage{State: model.UsageNotApplicable, Known: false, Source: "none"}
}

func unknownUsage(source string) model.Usage {
	return model.Usage{State: model.UsageUnknown, Known: false, Source: source}
}

func parseIntRaw(raw *json.RawMessage) *int {
	if raw == nil {
		return nil
	}
	text := strings.TrimSpace(string(*raw))
	if text == "" || text == "null" {
		return nil
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil || value != float64(int(value)) {
		return nil
	}
	v := int(value)
	return &v
}

func durationBetween(started *startedFile, finished *finishedFile) *float64 {
	if started == nil || finished == nil || started.StartedAt == "" || finished.FinishedAt == "" {
		return nil
	}
	start, err := time.Parse(time.RFC3339Nano, started.StartedAt)
	if err != nil {
		return nil
	}
	end, err := time.Parse(time.RFC3339Nano, finished.FinishedAt)
	if err != nil {
		return nil
	}
	seconds := end.Sub(start).Seconds()
	if seconds < 0 {
		return nil
	}
	return &seconds
}

func readResults(path string) ([]model.ResultRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read results.jsonl: %w", err)
	}
	defer file.Close()
	var out []model.ResultRecord
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var row resultsRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("results.jsonl is not valid JSON: %w", err)
		}
		out = append(out, model.ResultRecord{Plugin: row.Plugin, Scenario: row.Scenario, Status: row.Status})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read results.jsonl: %w", err)
	}
	return out, nil
}

func normalizeCounts(raw map[string]json.Number) map[string]int {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]int, len(raw))
	for key, value := range raw {
		i, err := value.Int64()
		if err != nil {
			f, ferr := value.Float64()
			if ferr != nil {
				continue
			}
			i = int64(f)
		}
		out[key] = int(i)
	}
	return out
}

func readJSON(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return err
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
