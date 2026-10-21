package suite

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rikdc/ai-skills/internal/judge"
	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/platform/atomicfile"
	"github.com/rikdc/ai-skills/internal/runner/attempts"
	"github.com/rikdc/ai-skills/internal/scenario"
)

// ScenarioRow is one results.jsonl object (spec §10.5).
type ScenarioRow struct {
	Plugin          string             `json:"plugin"`
	Scenario        string             `json:"scenario"`
	Units           []string           `json:"units"`
	Mode            string             `json:"mode"`
	Functional      bool               `json:"functional"`
	ArmCount        int                `json:"arm_count"`
	Denominator     float64            `json:"denominator"`
	CriteriaCount   int                `json:"criteria_count"`
	ConfiguredRuns  int                `json:"configured_runs"`
	Status          string             `json:"status"`
	Runs            []RunRow           `json:"runs"`
	Arms            ArmPair            `json:"arms"`
	Delta           *float64           `json:"delta"`
	PendingPoints   float64            `json:"pending_points"`
	PendingCriteria int                `json:"pending_criteria"`
	Criteria        []CriterionRow     `json:"criteria"`
	Regressions     []CriterionRow     `json:"regressions"`
	Expenditure     ExpenditureSummary `json:"expenditure"`
	AcceptedUsage   AcceptedUsage      `json:"accepted_usage"`
}

// RunRow is one arm/run in a scenario row.
type RunRow struct {
	Arm                  string           `json:"arm"`
	Run                  int              `json:"run"`
	Status               string           `json:"status"`
	StopReason           string           `json:"stop_reason,omitempty"`
	AgentAttempt         string           `json:"agent_attempt,omitempty"`
	JudgeAttempt         string           `json:"judge_attempt,omitempty"`
	CurrentSHA256        string           `json:"current_sha256,omitempty"`
	Complete             bool             `json:"complete"`
	AgentExit            *int             `json:"agent_exit"`
	TimedOut             bool             `json:"timed_out"`
	DurationSeconds      *float64         `json:"duration_seconds"`
	JudgeDurationSeconds *float64         `json:"judge_duration_seconds"`
	Mechanical           model.Mechanical `json:"mechanical"`
	Judged               *JudgedBlock     `json:"judged"`
	Pending              int              `json:"pending"`
	PendingPoints        float64          `json:"pending_points"`
	Score                *float64         `json:"score"`
	Max                  float64          `json:"max"`
	Percent              *float64         `json:"percent"`
	Usage                model.Usage      `json:"usage"`
	AgentUsage           model.Usage      `json:"agent_usage"`
	JudgeUsage           model.Usage      `json:"judge_usage"`
	CombinedUsage        model.Usage      `json:"combined_usage"`
	Evidence             string           `json:"evidence"`
	Error                string           `json:"error,omitempty"`
}

// JudgedBlock is the accepted judgement for one run.
type JudgedBlock struct {
	Score float64            `json:"score"`
	Max   float64            `json:"max"`
	Items []model.JudgedItem `json:"items"`
}

// ArmPair holds both arms.
type ArmPair struct {
	With    ArmStats `json:"with"`
	Without ArmStats `json:"without"`
}

// ArmStats summarises one arm.
type ArmStats struct {
	Runs          int           `json:"runs"`
	ScoreMean     *float64      `json:"score_mean"`
	MaxMean       *float64      `json:"max_mean"`
	PercentMean   *float64      `json:"percent_mean"`
	PercentMin    *float64      `json:"percent_min"`
	PercentMax    *float64      `json:"percent_max"`
	PendingPoints float64       `json:"pending_points"`
	Failures      int           `json:"failures"`
	Timing        ArmTiming     `json:"timing"`
	Usage         AcceptedUsage `json:"usage"`
}

// ArmTiming is invocation time over one arm.
type ArmTiming struct {
	AgentSeconds *float64 `json:"agent_seconds"`
	JudgeSeconds *float64 `json:"judge_seconds"`
	TimedOutRuns int      `json:"timed_out_runs"`
}

// AcceptedUsage holds the accepted agent, judge, and combined usage.
type AcceptedUsage struct {
	Agent    UsageTotals `json:"agent"`
	Judge    UsageTotals `json:"judge"`
	Combined UsageTotals `json:"combined"`
}

// UsageTotals sums one usage stream. Unknown is never zero: runs_known counts
// only known records, and totals cover those records.
type UsageTotals struct {
	RunsKnown        int      `json:"runs_known"`
	RunsTotal        int      `json:"runs_total"`
	CostUSD          *float64 `json:"cost_usd"`
	TotalTokens      *int64   `json:"total_tokens"`
	InputTokens      *int64   `json:"input_tokens"`
	OutputTokens     *int64   `json:"output_tokens"`
	ReasoningTokens  *int64   `json:"reasoning_tokens"`
	CacheReadTokens  *int64   `json:"cache_read_tokens"`
	CacheWriteTokens *int64   `json:"cache_write_tokens"`
}

// CriterionRow is one rubric criterion's paired result.
type CriterionRow struct {
	Name       string   `json:"name"`
	Max        float64  `json:"max"`
	With       *float64 `json:"with"`
	Without    *float64 `json:"without"`
	Delta      *float64 `json:"delta"`
	PairedRuns int      `json:"paired_runs"`
	Status     string   `json:"status"`
}

// ExpenditureSummary sums every attempt, including superseded and failed ones.
type ExpenditureSummary struct {
	KnownTotal            model.Usage `json:"known_total"`
	Complete              bool        `json:"complete"`
	TotalAttempts         int         `json:"total_attempts"`
	KnownAttempts         int         `json:"known_attempts"`
	UnknownAttempts       int         `json:"unknown_attempts"`
	NotApplicableAttempts int         `json:"not_applicable_attempts"`
	AgentAttempts         int         `json:"agent_attempts"`
	HookAttempts          int         `json:"hook_attempts"`
	JudgeAttempts         int         `json:"judge_attempts"`
}

// PluginRow is the per-plugin summary.
type PluginRow struct {
	Plugin        string   `json:"plugin"`
	Scenarios     int      `json:"scenarios"`
	With          *float64 `json:"with"`
	Without       *float64 `json:"without"`
	Delta         *float64 `json:"delta"`
	PendingPoints float64  `json:"pending_points"`
	Failures      int      `json:"failures"`
}

type suiteDocument struct {
	Config          model.SuiteConfig `json:"config"`
	Counts          map[string]int    `json:"counts"`
	DurationSeconds float64           `json:"duration_seconds"`
	Scenarios       []ScenarioRow     `json:"scenarios"`
	Plugins         []PluginRow       `json:"plugins"`
}

type aggregateStats struct {
	Scenarios       []ScenarioRow
	Plugins         []PluginRow
	Counts          map[string]int
	DurationSeconds float64
	RunnerFailures  int
	PendingRuns     int
	StrictFailures  []string
}

const epsilon = 0.0001

func (r *Runner) aggregate(selected []scenario.Selected, fingerprints map[string]string,
	decisions map[runKey]decision, started time.Time, executed, skipped int) aggregateStats {

	stats := aggregateStats{Counts: map[string]int{}}
	stats.DurationSeconds = time.Since(started).Seconds()
	var agentSeconds, judgeSeconds []float64
	var timedOut, costKnown, costTotal int
	var totalAttempts, agentAttempts, judgeAttempts, hookAttempts int
	var failedAttempts, interruptedAttempts, retriedAttempts int
	var usageKnown, usageUnknown, usageNotApplicable int
	var checksFailedRuns, cancelledRuns, incompleteRuns, mechanicalScenarios int
	judgedRuns, pendingRuns, runCount := 0, 0, 0

	for _, item := range selected {
		row := r.buildScenarioRow(item, fingerprints[item.Pair()])
		stats.Scenarios = append(stats.Scenarios, row)
		if row.Status != "ok" {
			stats.RunnerFailures++
		}
		if row.Functional || armCountOf(row.Arms) > 0 {
			hasMechanical := false
			for _, run := range row.Runs {
				if run.Mechanical.Max > 0 {
					hasMechanical = true
				}
			}
			if hasMechanical {
				mechanicalScenarios++
			}
		}
		for _, run := range row.Runs {
			runCount++
			if run.Judged != nil {
				judgedRuns++
			}
			if run.Pending > 0 {
				pendingRuns++
			}
			if run.DurationSeconds != nil {
				agentSeconds = append(agentSeconds, *run.DurationSeconds)
			}
			if run.JudgeDurationSeconds != nil {
				judgeSeconds = append(judgeSeconds, *run.JudgeDurationSeconds)
			}
			if run.TimedOut {
				timedOut++
			}
			if run.AgentUsage.CostUSD != nil || run.JudgeUsage.CostUSD != nil {
				costKnown++
			}
			costTotal++
			switch run.Status {
			case string(model.StatusChecksFailed):
				checksFailedRuns++
			case string(model.StatusCancelled):
				cancelledRuns++
			case string(model.StatusIncomplete), string(model.StatusMissing):
				incompleteRuns++
			}
		}

		dir := filepath.Join(r.cfg.Out, "runs", item.Scenario.Plugin, item.Scenario.Name)
		list, err := enumerateRunAttempts(dir)
		if err == nil {
			totalAttempts += list.total
			agentAttempts += list.agent
			judgeAttempts += list.judge
			hookAttempts += list.hook
			failedAttempts += list.failed
			interruptedAttempts += list.interrupted
			retriedAttempts += list.retried
			usageKnown += list.known
			usageUnknown += list.unknown
			usageNotApplicable += list.notApplicable
			for _, run := range row.Runs {
				if run.Status != "ok" {
					stats.RunnerFailures += 0
				}
			}
		}
	}
	// RunnerFailures counts scenarios, not runs; recompute explicitly.
	stats.RunnerFailures = 0
	for _, row := range stats.Scenarios {
		if row.Status != "ok" {
			stats.RunnerFailures++
		}
	}
	stats.PendingRuns = pendingRuns
	stats.Plugins = pluginRows(stats.Scenarios)

	kept := 0
	for _, decision := range decisions {
		if decision.retain {
			kept++
		}
	}
	counts := stats.Counts
	counts["planned"] = len(selected)
	counts["executed"] = executed
	counts["skipped"] = skipped
	counts["runner_failures"] = stats.RunnerFailures
	counts["agent_invocations"] = runCount
	counts["judged_runs"] = judgedRuns
	counts["pending_runs"] = pendingRuns
	counts["scenarios_with_mechanical_checks"] = mechanicalScenarios
	counts["cost_known_runs"] = costKnown
	counts["cost_total_runs"] = costTotal
	counts["duration_seconds"] = int(math.Round(stats.DurationSeconds))
	counts["timed_out_runs"] = timedOut
	counts["total_attempts"] = totalAttempts
	counts["agent_attempts"] = agentAttempts
	counts["hook_attempts"] = hookAttempts
	counts["judge_attempts"] = judgeAttempts
	counts["failed_attempts"] = failedAttempts
	counts["interrupted_attempts"] = interruptedAttempts
	counts["retried_attempts"] = retriedAttempts
	counts["kept_failures"] = kept
	counts["checks_failed_runs"] = checksFailedRuns
	counts["cancelled_runs"] = cancelledRuns
	counts["incomplete_runs"] = incompleteRuns
	counts["usage_known_attempts"] = usageKnown
	counts["usage_unknown_attempts"] = usageUnknown
	counts["usage_not_applicable_attempts"] = usageNotApplicable
	if len(agentSeconds) > 0 {
		counts["agent_seconds"] = int(math.Round(sum(agentSeconds)))
	}
	if len(judgeSeconds) > 0 {
		counts["judge_seconds"] = int(math.Round(sum(judgeSeconds)))
	}

	if r.cfg.Strict {
		if stats.RunnerFailures > 0 {
			stats.StrictFailures = append(stats.StrictFailures,
				fmt.Sprintf("%d scenario(s) had runner failures", stats.RunnerFailures))
		}
		if pendingRuns > 0 {
			stats.StrictFailures = append(stats.StrictFailures,
				fmt.Sprintf("%d run(s) still have pending criteria", pendingRuns))
		}
		nonDiscriminating := 0
		for _, row := range stats.Scenarios {
			if row.Delta != nil && *row.Delta <= epsilon {
				nonDiscriminating++
			}
		}
		if nonDiscriminating > 0 {
			stats.StrictFailures = append(stats.StrictFailures,
				fmt.Sprintf("%d scenario(s) scored no better with the unit than without", nonDiscriminating))
		}
	}
	return stats
}

// runFailureLines describes every failed run in the aggregate so the summary
// and the suite-level error explain what happened instead of only counting.
func runFailureLines(stats aggregateStats) []string {
	var lines []string
	for _, row := range stats.Scenarios {
		for _, run := range row.Runs {
			if run.Status == string(model.StatusOK) {
				continue
			}
			line := fmt.Sprintf("%s/%s %s/run-%d: %s",
				row.Plugin, row.Scenario, run.Arm, run.Run, run.Status)
			if run.Error != "" {
				line += ": " + run.Error
			}
			lines = append(lines, line)
		}
	}
	return lines
}

// firstRunFailure returns the first failed run, or "" when nothing failed.
func firstRunFailure(stats aggregateStats) string {
	if lines := runFailureLines(stats); len(lines) > 0 {
		return lines[0]
	}
	return ""
}

func armCountOf(pair ArmPair) int {
	n := 0
	if pair.With.Runs > 0 {
		n++
	}
	if pair.Without.Runs > 0 {
		n++
	}
	return n
}

func (r *Runner) buildScenarioRow(item scenario.Selected, fingerprint string) ScenarioRow {
	rubric := r.loadRubric(item)
	denominator := 0.0
	for _, criterion := range rubric {
		denominator += criterion.MaxScore
	}
	functional := item.Mode() == model.ModeFunctional
	armCount := 2
	if functional || (r.cfg.Arm != "" && r.cfg.Arm != "both") {
		armCount = 1
	}
	row := ScenarioRow{
		Plugin:         item.Scenario.Plugin,
		Scenario:       item.Scenario.Name,
		Units:          nonNilStrings(item.InstallUnits()),
		Mode:           string(item.Mode()),
		Functional:     functional,
		ArmCount:       armCount,
		Denominator:    denominator,
		CriteriaCount:  len(rubric),
		ConfiguredRuns: r.cfg.Runs,
	}
	for _, arm := range armsFor(item.Mode(), r.cfg.Arm) {
		for n := 1; n <= r.cfg.Runs; n++ {
			dir := runKey{item.Scenario.Plugin, item.Scenario.Name, arm, n}.runDir(r.cfg.Out)
			row.Runs = append(row.Runs, loadRunRow(dir, arm, n, rubric, denominator))
		}
	}
	for _, run := range row.Runs {
		row.PendingPoints += run.PendingPoints
		row.PendingCriteria += run.Pending
	}
	row.Arms.With = armStats(row.Runs, "with")
	row.Arms.Without = armStats(row.Runs, "without")
	if !functional && row.Arms.With.PercentMean != nil && row.Arms.Without.PercentMean != nil {
		delta := *row.Arms.With.PercentMean - *row.Arms.Without.PercentMean
		row.Delta = &delta
	}
	row.Criteria = criterionRows(rubric, row.Runs, functional)
	row.Regressions = []CriterionRow{}
	for _, criterion := range row.Criteria {
		if criterion.Delta != nil && *criterion.Delta < -epsilon {
			row.Regressions = append(row.Regressions, criterion)
		}
	}
	row.Status = "ok"
	for _, run := range row.Runs {
		if run.Status != string(model.StatusOK) {
			row.Status = "runner-failed"
			break
		}
	}
	row.Expenditure = expenditure(item, r.cfg.Out)
	row.AcceptedUsage = acceptedUsage(row.Runs)
	if fingerprint == "" {
		row.Status = "runner-failed"
	}
	return row
}

func (r *Runner) loadRubric(item scenario.Selected) []model.Criterion {
	paths := []string{
		filepath.Join(r.cfg.Out, "criteria", item.Scenario.Plugin, item.Scenario.Name+".json"),
		filepath.Join(item.Scenario.Dir, "criteria.json"),
	}
	for _, path := range paths {
		rubric, err := scenario.ParseCriteria(path)
		if err == nil {
			return rubric.Checklist
		}
	}
	return nil
}

func loadRunRow(runDir, arm string, run int, rubric []model.Criterion, denominator float64) RunRow {
	row := RunRow{
		Arm:           arm,
		Run:           run,
		Status:        string(model.StatusMissing),
		Usage:         model.NotApplicable("missing"),
		AgentUsage:    model.NotApplicable("missing"),
		JudgeUsage:    model.NotApplicable("no judge attempt"),
		CombinedUsage: model.NotApplicable("missing"),
		Max:           denominator,
		Mechanical:    model.Mechanical{},
		Evidence:      defaultEvidence(runDir, arm, run),
	}
	info, err := os.Stat(runDir)
	if err != nil || !info.IsDir() {
		return row
	}
	store := attempts.New(runDir)
	list, err := store.Enumerate()
	if err != nil || len(list) == 0 {
		return row
	}
	pointer, raw, currentErr := store.LoadCurrent()
	if currentErr == nil {
		row.CurrentSHA256 = attempts.SHA256Hex(raw)
		row.AgentAttempt = pointer.AgentAttempt
		row.JudgeAttempt = pointer.JudgeAttempt
	}
	execution := latestExecution(list)
	if currentErr == nil {
		if agent, err := store.LoadAttempt(pointer.AgentAttempt); err == nil {
			execution = agent
		}
	}
	if execution == nil {
		return row
	}
	if execution.Finished != nil && execution.Finished.EvidenceSHA256 != "" {
		row.Evidence = evidencePath(runDir, execution)
	}
	if execution.Finished == nil {
		row.Status = string(model.StatusInterrupted)
		row.AgentUsage = execution.EffectiveUsage()
		row.Usage = row.AgentUsage
		row.CombinedUsage = model.Combine(row.AgentUsage, row.JudgeUsage)
		row.Pending = len(rubric)
		for _, criterion := range rubric {
			row.PendingPoints += criterion.MaxScore
		}
		return row
	}
	finished := execution.Finished
	row.Status = string(finished.Status)
	if row.Status == "" {
		row.Status = string(model.StatusOK)
	}
	row.StopReason = finished.StopReason
	row.AgentExit = finished.AgentExit
	row.TimedOut = finished.TimedOut || finished.Status == model.StatusTimedOut
	row.DurationSeconds = attemptDuration(execution)
	row.AgentUsage = finished.Usage.NormalizeState()
	if row.AgentUsage.State == "" {
		row.AgentUsage = execution.EffectiveUsage()
	}
	row.Usage = row.AgentUsage
	row.Error = finished.Error
	row.Mechanical = readMechanical(execution.Dir)

	scored := map[string]float64{}
	for _, item := range row.Mechanical.Items {
		scored[item.Name] = item.Score
	}
	pending := rubric
	if row.CurrentSHA256 != "" {
		row.Complete = row.Status == string(model.StatusOK)
		pending = pendingCriteria(store, rubric, row.Mechanical.Items)
		if pointer.JudgeAttempt != "" {
			judgeAttempt, err := store.LoadAttempt(pointer.JudgeAttempt)
			switch {
			case err == nil && judgeAttempt.Finished != nil:
				row.JudgeUsage = judgeAttempt.Finished.Usage.NormalizeState()
				row.JudgeDurationSeconds = attemptDuration(judgeAttempt)
				if judged, ok := acceptedJudgement(pointer, judgeAttempt, pending); ok {
					row.Judged = judged
					pending = nil
					for _, item := range judged.Items {
						scored[item.Name] = item.Score
					}
				}
			case err == nil:
				row.JudgeUsage = judgeAttempt.EffectiveUsage()
			}
		}
	}
	row.Pending = len(pending)
	for _, criterion := range pending {
		row.PendingPoints += criterion.MaxScore
	}
	row.JudgeUsage = row.JudgeUsage.NormalizeState()
	if row.JudgeUsage.State == "" {
		row.JudgeUsage = model.NotApplicable("no judge attempt")
	}
	row.CombinedUsage = model.Combine(row.AgentUsage, row.JudgeUsage)
	if row.Status == string(model.StatusOK) {
		score := 0.0
		for _, value := range scored {
			score += value
		}
		row.Score = &score
		if denominator > 0 {
			percent := score * 100 / denominator
			row.Percent = &percent
		}
	}
	return row
}

func latestExecution(list []*attempts.Attempt) *attempts.Attempt {
	var latest *attempts.Attempt
	for _, attempt := range list {
		if attempt.Kind != model.KindAgent && attempt.Kind != model.KindHook {
			continue
		}
		if latest == nil || attempt.Sequence > latest.Sequence {
			latest = attempt
		}
	}
	return latest
}

func pendingCriteria(store *attempts.Store, rubric []model.Criterion, mechanical []model.JudgedItem) []model.Criterion {
	request, _, _, err := store.ReadGradingRequest()
	if err == nil {
		return request.JudgementPending
	}
	covered := map[string]bool{}
	for _, item := range mechanical {
		covered[item.Name] = true
	}
	var pending []model.Criterion
	for _, criterion := range rubric {
		if !covered[criterion.Name] {
			pending = append(pending, criterion)
		}
	}
	return pending
}

func acceptedJudgement(pointer model.CurrentPointer,
	judgeAttempt *attempts.Attempt, pending []model.Criterion) (*JudgedBlock, bool) {
	if judgeAttempt.Finished == nil || judgeAttempt.Finished.Status != model.StatusOK {
		return nil, false
	}
	if pointer.GradedAgentAttempt != "" && judgeAttempt.Finished.GradedAgentAttempt != pointer.GradedAgentAttempt {
		return nil, false
	}
	if judgeAttempt.Finished.GradedEvidenceSHA256 != pointer.AgentEvidenceSHA256 {
		return nil, false
	}
	if judgeAttempt.Finished.GradingRequestSHA256 != pointer.AgentGradingRequestSHA256 {
		return nil, false
	}
	if judgeAttempt.Finished.JudgeIdentitySHA256 != pointer.JudgeIdentitySHA256 {
		return nil, false
	}
	if attempts.VerifyJudgement(judgeAttempt) != nil {
		return nil, false
	}
	raw, err := os.ReadFile(filepath.Join(judgeAttempt.Dir, "judgement.json"))
	if err != nil {
		return nil, false
	}
	judgement, err := judge.ValidateJudgement(raw, pending)
	if err != nil {
		return nil, false
	}
	block := &JudgedBlock{Items: judgement}
	for _, item := range judgement {
		block.Score += item.Score
		block.Max += item.Max
	}
	return block, true
}

func defaultEvidence(runDir, arm string, run int) string {
	slashed := filepath.ToSlash(runDir)
	if idx := strings.Index(slashed, "/runs/"); idx >= 0 {
		return slashed[idx+1:]
	}
	return filepath.ToSlash(filepath.Join("runs", fmt.Sprintf("%s-run-%d", arm, run)))
}

func evidencePath(runDir string, attempt *attempts.Attempt) string {
	if attempt == nil || attempt.Finished == nil || attempt.Finished.EvidenceSHA256 == "" {
		return ""
	}
	rel := filepath.ToSlash(filepath.Join("attempts", attempt.ID, "evidence.json"))
	if idx := strings.Index(filepath.ToSlash(runDir), "/runs/"); idx >= 0 {
		return filepath.ToSlash(filepath.Join(filepath.ToSlash(runDir)[idx+1:], rel))
	}
	return rel
}

func attemptDuration(attempt *attempts.Attempt) *float64 {
	if attempt.Started == nil || attempt.Finished == nil {
		return nil
	}
	start, err := time.Parse(time.RFC3339Nano, attempt.Started.StartedAt)
	if err != nil {
		return nil
	}
	end, err := time.Parse(time.RFC3339Nano, attempt.Finished.FinishedAt)
	if err != nil {
		return nil
	}
	seconds := end.Sub(start).Seconds()
	if seconds < 0 {
		return nil
	}
	return &seconds
}

func readMechanical(dir string) model.Mechanical {
	var mechanical model.Mechanical
	data, err := os.ReadFile(filepath.Join(dir, "mechanical.json"))
	if err != nil {
		return model.Mechanical{}
	}
	if err := json.Unmarshal(data, &mechanical); err != nil {
		return model.Mechanical{}
	}
	return mechanical
}

func armStats(runs []RunRow, arm string) ArmStats {
	stats := ArmStats{}
	var scores, percents []float64
	var maxes []float64
	var agentSeconds, judgeSeconds []float64
	var usages []RunRow
	for _, run := range runs {
		if run.Arm != arm {
			continue
		}
		stats.Runs++
		if run.Status != string(model.StatusOK) {
			stats.Failures++
		}
		if run.Score != nil {
			scores = append(scores, *run.Score)
		}
		if run.Percent != nil {
			percents = append(percents, *run.Percent)
		}
		maxes = append(maxes, run.Max)
		stats.PendingPoints += run.PendingPoints
		if run.DurationSeconds != nil {
			agentSeconds = append(agentSeconds, *run.DurationSeconds)
		}
		if run.JudgeDurationSeconds != nil {
			judgeSeconds = append(judgeSeconds, *run.JudgeDurationSeconds)
		}
		if run.TimedOut {
			stats.Timing.TimedOutRuns++
		}
		if run.Status == string(model.StatusOK) {
			usages = append(usages, run)
		}
	}
	stats.ScoreMean = meanPtr(scores)
	stats.MaxMean = meanPtr(maxes)
	stats.PercentMean = meanPtr(percents)
	stats.PercentMin = minPtr(percents)
	stats.PercentMax = maxPtr(percents)
	stats.Timing.AgentSeconds = sumPtr(agentSeconds)
	stats.Timing.JudgeSeconds = sumPtr(judgeSeconds)
	stats.Usage = acceptedUsage(usages)
	return stats
}

func acceptedUsage(runs []RunRow) AcceptedUsage {
	var agent, judge, combined []model.Usage
	for _, run := range runs {
		if run.Status != string(model.StatusOK) {
			continue
		}
		agent = append(agent, run.AgentUsage)
		judge = append(judge, run.JudgeUsage)
		combined = append(combined, run.CombinedUsage)
	}
	return AcceptedUsage{
		Agent:    usageTotals(agent),
		Judge:    usageTotals(judge),
		Combined: usageTotals(combined),
	}
}

func usageTotals(usages []model.Usage) UsageTotals {
	out := UsageTotals{RunsTotal: len(usages)}
	var cost float64
	var total, input, output, reasoning, cacheRead, cacheWrite int64
	var hasTotal, hasInput, hasOutput, hasReasoning, hasCacheRead, hasCacheWrite bool
	for _, usage := range usages {
		usage = usage.NormalizeState()
		if usage.State != model.UsageKnown {
			continue
		}
		out.RunsKnown++
		if usage.CostUSD != nil {
			cost += *usage.CostUSD
			out.CostUSD = floatPtr(cost)
		}
		addInt := func(dst *int64, src *int64) bool {
			if src == nil {
				return false
			}
			*dst += *src
			return true
		}
		hasTotal = addInt(&total, usage.TotalTokens) || hasTotal
		hasInput = addInt(&input, usage.InputTokens) || hasInput
		hasOutput = addInt(&output, usage.OutputTokens) || hasOutput
		hasReasoning = addInt(&reasoning, usage.ReasoningTokens) || hasReasoning
		hasCacheRead = addInt(&cacheRead, usage.CacheReadTokens) || hasCacheRead
		hasCacheWrite = addInt(&cacheWrite, usage.CacheWriteTokens) || hasCacheWrite
	}
	// Assign totals only when the component appeared in at least one known
	// record, so "no data" stays nil while a known zero stays zero.
	if hasTotal {
		out.TotalTokens = int64Ptr(total)
	}
	if hasInput {
		out.InputTokens = int64Ptr(input)
	}
	if hasOutput {
		out.OutputTokens = int64Ptr(output)
	}
	if hasReasoning {
		out.ReasoningTokens = int64Ptr(reasoning)
	}
	if hasCacheRead {
		out.CacheReadTokens = int64Ptr(cacheRead)
	}
	if hasCacheWrite {
		out.CacheWriteTokens = int64Ptr(cacheWrite)
	}
	return out
}

func criterionRows(rubric []model.Criterion, runs []RunRow, functional bool) []CriterionRow {
	rows := make([]CriterionRow, 0, len(rubric))
	for _, criterion := range rubric {
		row := CriterionRow{Name: criterion.Name, Max: criterion.MaxScore}
		var with, without []float64
		for _, run := range runs {
			if run.Status != string(model.StatusOK) {
				continue
			}
			value, ok := scoreFor(run, criterion.Name)
			if !ok {
				continue
			}
			switch run.Arm {
			case "with":
				with = append(with, value)
			case "without":
				without = append(without, value)
			}
		}
		row.With = meanPtr(with)
		row.Without = meanPtr(without)
		if !functional && row.With != nil && row.Without != nil {
			delta := *row.With - *row.Without
			row.Delta = &delta
		}
		row.PairedRuns = len(with)
		switch {
		case row.With == nil:
			row.Status = "pending"
		case !functional && row.Without == nil:
			row.Status = "not-comparable"
		default:
			row.Status = "scored"
		}
		rows = append(rows, row)
	}
	return rows
}

func scoreFor(run RunRow, name string) (float64, bool) {
	for _, item := range run.Mechanical.Items {
		if item.Name == name {
			return item.Score, true
		}
	}
	if run.Judged != nil {
		for _, item := range run.Judged.Items {
			if item.Name == name {
				return item.Score, true
			}
		}
	}
	return 0, false
}

func pluginRows(rows []ScenarioRow) []PluginRow {
	byPlugin := map[string][]ScenarioRow{}
	for _, row := range rows {
		byPlugin[row.Plugin] = append(byPlugin[row.Plugin], row)
	}
	names := make([]string, 0, len(byPlugin))
	for name := range byPlugin {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]PluginRow, 0, len(names))
	for _, name := range names {
		group := byPlugin[name]
		row := PluginRow{Plugin: name, Scenarios: len(group)}
		var with, without []float64
		for _, scenarioRow := range group {
			row.PendingPoints += scenarioRow.PendingPoints
			if scenarioRow.Status != "ok" {
				row.Failures++
			}
			if scenarioRow.Arms.With.PercentMean != nil {
				with = append(with, *scenarioRow.Arms.With.PercentMean)
			}
			if scenarioRow.Arms.Without.PercentMean != nil {
				without = append(without, *scenarioRow.Arms.Without.PercentMean)
			}
		}
		row.With = meanPtr(with)
		row.Without = meanPtr(without)
		if row.With != nil && row.Without != nil {
			delta := *row.With - *row.Without
			row.Delta = &delta
		}
		out = append(out, row)
	}
	return out
}

// attemptList summarises the immutable attempt directories under one scenario.
type attemptList struct {
	total, agent, judge, hook     int
	failed, interrupted, retried  int
	known, unknown, notApplicable int
}

func enumerateRunAttempts(scenarioDir string) (attemptList, error) {
	var out attemptList
	entries, err := os.ReadDir(scenarioDir)
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		armDir := filepath.Join(scenarioDir, entry.Name())
		runs, err := os.ReadDir(armDir)
		if err != nil {
			continue
		}
		for _, runEntry := range runs {
			if !runEntry.IsDir() || !strings.HasPrefix(runEntry.Name(), "run-") {
				continue
			}
			store := attempts.New(filepath.Join(armDir, runEntry.Name()))
			list, err := store.Enumerate()
			if err != nil {
				continue
			}
			executions := 0
			for _, attempt := range list {
				out.total++
				switch attempt.Kind {
				case model.KindAgent:
					out.agent++
				case model.KindHook:
					out.hook++
				case model.KindJudge:
					out.judge++
				}
				if attempt.Kind == model.KindAgent || attempt.Kind == model.KindHook {
					executions++
				}
				if attempt.Finished == nil {
					out.interrupted++
				} else if attempt.Finished.Status != model.StatusOK {
					out.failed++
				}
				switch attempt.EffectiveUsage().NormalizeState().State {
				case model.UsageKnown:
					out.known++
				case model.UsageUnknown:
					out.unknown++
				case model.UsageNotApplicable:
					out.notApplicable++
				}
			}
			if executions > 1 {
				out.retried += executions - 1
			}
		}
	}
	return out, nil
}

func expenditure(item scenario.Selected, outRoot string) ExpenditureSummary {
	summary := ExpenditureSummary{Complete: true}
	scenarioDir := filepath.Join(outRoot, "runs", item.Scenario.Plugin, item.Scenario.Name)
	entries, _ := os.ReadDir(scenarioDir)
	var usages []model.Usage
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		armDir := filepath.Join(scenarioDir, entry.Name())
		runs, _ := os.ReadDir(armDir)
		for _, runEntry := range runs {
			if !runEntry.IsDir() || !strings.HasPrefix(runEntry.Name(), "run-") {
				continue
			}
			store := attempts.New(filepath.Join(armDir, runEntry.Name()))
			list, err := store.Enumerate()
			if err != nil {
				continue
			}
			for _, attempt := range list {
				summary.TotalAttempts++
				switch attempt.Kind {
				case model.KindAgent:
					summary.AgentAttempts++
				case model.KindHook:
					summary.HookAttempts++
				case model.KindJudge:
					summary.JudgeAttempts++
				}
				usage := attempt.EffectiveUsage().NormalizeState()
				usages = append(usages, usage)
				switch usage.State {
				case model.UsageKnown:
					summary.KnownAttempts++
				case model.UsageUnknown:
					summary.UnknownAttempts++
					summary.Complete = false
				case model.UsageNotApplicable:
					summary.NotApplicableAttempts++
				}
			}
		}
	}
	summary.KnownTotal = sumKnownUsage(usages)
	return summary
}

func sumKnownUsage(usages []model.Usage) model.Usage {
	out := model.KnownUsage("expenditure")
	var total, input, output, reasoning, cacheRead, cacheWrite int64
	var cost float64
	hasCost := false
	var hasTotal, hasInput, hasOutput, hasReasoning, hasCacheRead, hasCacheWrite bool
	for _, usage := range usages {
		usage = usage.NormalizeState()
		if usage.State != model.UsageKnown {
			continue
		}
		add := func(dst *int64, src *int64) bool {
			if src == nil {
				return false
			}
			*dst += *src
			return true
		}
		hasTotal = add(&total, usage.TotalTokens) || hasTotal
		hasInput = add(&input, usage.InputTokens) || hasInput
		hasOutput = add(&output, usage.OutputTokens) || hasOutput
		hasReasoning = add(&reasoning, usage.ReasoningTokens) || hasReasoning
		hasCacheRead = add(&cacheRead, usage.CacheReadTokens) || hasCacheRead
		hasCacheWrite = add(&cacheWrite, usage.CacheWriteTokens) || hasCacheWrite
		if usage.CostUSD != nil {
			cost += *usage.CostUSD
			hasCost = true
		}
	}
	if hasTotal {
		out.TotalTokens = int64Ptr(total)
	}
	if hasInput {
		out.InputTokens = int64Ptr(input)
	}
	if hasOutput {
		out.OutputTokens = int64Ptr(output)
	}
	if hasReasoning {
		out.ReasoningTokens = int64Ptr(reasoning)
	}
	if hasCacheRead {
		out.CacheReadTokens = int64Ptr(cacheRead)
	}
	if hasCacheWrite {
		out.CacheWriteTokens = int64Ptr(cacheWrite)
	}
	if hasCost {
		out.CostUSD = floatPtr(cost)
	}
	return out
}

func writeGradingIndex(path string, rows []ScenarioRow) error {
	var b strings.Builder
	b.WriteString("plugin\tscenario\tarm\trun\tpending\tevidence\n")
	for _, row := range rows {
		for _, run := range row.Runs {
			if run.Pending <= 0 {
				continue
			}
			fmt.Fprintf(&b, "%s\t%s\t%s\t%d\t%d\t%s\n",
				row.Plugin, row.Scenario, run.Arm, run.Run, run.Pending, run.Evidence)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return atomicfile.Write(path, []byte(b.String()), 0o600)
}

func renderSummary(manifest model.SuiteManifest, stats aggregateStats) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🧪 Eval suite: %s\n\n", manifest.CreatedAt)
	fmt.Fprintf(&b, "- Planned: %d  ·  Executed: %d  ·  Skipped: %d\n",
		stats.Counts["planned"], stats.Counts["executed"], stats.Counts["skipped"])
	fmt.Fprintf(&b, "- Runner failures: %d  ·  Pending runs: %d  ·  Duration: %.0fs\n",
		stats.Counts["runner_failures"], stats.Counts["pending_runs"], stats.DurationSeconds)
	fmt.Fprintf(&b, "- Agent invocations: %d  ·  Judged runs: %d  ·  Timed-out runs: %d\n\n",
		stats.Counts["agent_invocations"], stats.Counts["judged_runs"], stats.Counts["timed_out_runs"])

	if stats.Counts["pending_runs"] > 0 {
		gradeHint := "evalctl grade --suite <suite>"
		if manifest.Config.AuthCLI {
			gradeHint += " --auth-cli"
		}
		fmt.Fprintf(&b, "**%d run(s) still have pending criteria.** Grade them with `%s`.\n\n",
			stats.Counts["pending_runs"], gradeHint)
	}

	b.WriteString("## By plugin\n\n")
	b.WriteString("| Plugin | Scenarios | With | Without | Δ |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, row := range stats.Plugins {
		fmt.Fprintf(&b, "| %s | %d | %s | %s | %s |\n",
			row.Plugin, row.Scenarios, pct(row.With), pct(row.Without), pts(row.Delta))
	}
	b.WriteString("\n## By scenario\n\n")
	b.WriteString("| Plugin | Scenario | With | Without | Δ | Pending | Status |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, row := range stats.Scenarios {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n",
			row.Plugin, row.Scenario, pct(row.Arms.With.PercentMean),
			pct(row.Arms.Without.PercentMean), pts(row.Delta),
			strconv.FormatFloat(row.PendingPoints, 'f', -1, 64)+" pending", row.Status)
	}
	if lines := runFailureLines(stats); len(lines) > 0 {
		b.WriteString("\n## Failures\n\n")
		const maxLines = 20
		for i, line := range lines {
			if i == maxLines {
				fmt.Fprintf(&b, "- … and %d more\n", len(lines)-maxLines)
				break
			}
			fmt.Fprintf(&b, "- %s\n", line)
		}
	}
	b.WriteString("\n## Counts\n\n")
	keys := make([]string, 0, len(stats.Counts))
	for key := range stats.Counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	b.WriteString("| Count | Value |\n| --- | --- |\n")
	for _, key := range keys {
		fmt.Fprintf(&b, "| %s | %d |\n", key, stats.Counts[key])
	}
	return b.String()
}

func pct(value *float64) string {
	if value == nil {
		return "—"
	}
	return strconv.FormatFloat(math.Round(*value*10)/10, 'f', -1, 64) + "%"
}

func pts(value *float64) string {
	if value == nil {
		return "—"
	}
	rounded := math.Round(*value*10) / 10
	if rounded > 0 {
		return "+" + strconv.FormatFloat(rounded, 'f', -1, 64) + " pts"
	}
	return strconv.FormatFloat(rounded, 'f', -1, 64) + " pts"
}

func meanPtr(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	return floatPtr(sum(values) / float64(len(values)))
}

func minPtr(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	min := values[0]
	for _, value := range values[1:] {
		if value < min {
			min = value
		}
	}
	return floatPtr(min)
}

func maxPtr(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	max := values[0]
	for _, value := range values[1:] {
		if value > max {
			max = value
		}
	}
	return floatPtr(max)
}

func sum(values []float64) float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total
}

func sumPtr(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	return floatPtr(sum(values))
}

func floatPtr(value float64) *float64 { return &value }

func int64Ptr(value int64) *int64 { return &value }

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
