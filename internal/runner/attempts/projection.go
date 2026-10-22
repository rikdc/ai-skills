package attempts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/platform/atomicfile"
	"github.com/rikdc/ai-skills/internal/skilluse"
)

// RunContext supplies the scenario metadata that is not stored in attempt
// records but is required by the result projection.
type RunContext struct {
	Plugin   string
	Scenario string
	Arm      string
	Mode     string
	Run      int
	// ChecksRequired records whether the scenario declared checks.sh.
	ChecksRequired bool
	// EvidencePath overrides the default attempts/<id>/evidence.json path.
	EvidencePath string
	// Skills names the skill units installed for a with-arm run. When set,
	// the projection records whether the accepted agent log used them.
	Skills []string
	// SkillDir is the workspace skills directory; empty means the default.
	SkillDir string
	// SkillUse carries an already-recorded detection, which wins over
	// re-reading the log (the log may since have been pruned).
	SkillUse *model.SkillUse
}

func readMechanical(attemptDir string) (model.Mechanical, error) {
	path := filepath.Join(attemptDir, "mechanical.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return model.Mechanical{}, nil
	}
	if err != nil {
		return model.Mechanical{}, err
	}
	var mechanical model.Mechanical
	if err := json.Unmarshal(data, &mechanical); err != nil {
		return model.Mechanical{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return mechanical, nil
}

func notApplicableUsage(reason string) model.Usage {
	return model.Usage{
		State:  model.UsageNotApplicable,
		Known:  false,
		Source: "none",
		Reason: reason,
	}
}

func unknownUsage(reason string) model.Usage {
	return model.Usage{
		State:  model.UsageUnknown,
		Known:  false,
		Source: "attempt-store",
		Reason: reason,
	}
}

func normalizeUsage(usage model.Usage) model.Usage {
	switch usage.State {
	case model.UsageKnown, model.UsageUnknown, model.UsageNotApplicable:
		return usage
	default:
		return unknownUsage("missing usage state")
	}
}

// combineUsage merges accepted agent and judge usage with the three-state
// rules shared with the report. Unknown is never converted to zero.
func combineUsage(agent, judge model.Usage) model.Usage {
	return model.Combine(normalizeUsage(agent), normalizeUsage(judge))
}

// BuildResult derives the result projection from current.json and the accepted
// immutable attempts. It does not consult any existing result.json.
func (s *Store) BuildResult(ctx RunContext) (model.RunResult, error) {
	pointer, raw, err := s.LoadCurrent()
	if err != nil {
		return model.RunResult{}, err
	}
	agent, err := s.LoadAttempt(pointer.AgentAttempt)
	if err != nil {
		return model.RunResult{}, err
	}
	if agent.Finished == nil {
		return model.RunResult{}, fmt.Errorf("current pointer references interrupted attempt %s", agent.ID)
	}
	if agent.Finished.Status != model.StatusOK {
		return model.RunResult{}, fmt.Errorf("current pointer references attempt %s with status %q", agent.ID, agent.Finished.Status)
	}
	if agent.Finished.EvidenceSHA256 != pointer.AgentEvidenceSHA256 {
		return model.RunResult{}, fmt.Errorf("current pointer evidence digest does not match attempt %s", agent.ID)
	}
	if agent.Finished.GradingRequestSHA256 != pointer.AgentGradingRequestSHA256 {
		return model.RunResult{}, fmt.Errorf("current pointer grading-request digest does not match attempt %s", agent.ID)
	}

	mechanical, err := readMechanical(agent.Dir)
	if err != nil {
		return model.RunResult{}, err
	}
	agentUsage := normalizeUsage(agent.Finished.Usage)
	judgeUsage := notApplicableUsage("no accepted judge attempt")
	var judge *Attempt
	if pointer.JudgeAttempt != "" {
		judge, err = s.LoadAttempt(pointer.JudgeAttempt)
		if err != nil {
			return model.RunResult{}, err
		}
		if err := validateJudgeBinding(judge, pointer); err != nil {
			return model.RunResult{}, err
		}
		judgeUsage = normalizeUsage(judge.Finished.Usage)
	}

	evidencePath := ctx.EvidencePath
	if evidencePath == "" {
		evidencePath = filepath.ToSlash(filepath.Join("attempts", agent.ID, evidenceName))
	}
	startedAt := ""
	backend := ""
	modelName := ""
	if agent.Started != nil {
		startedAt = agent.Started.StartedAt
		backend = agent.Started.Executor
		modelName = agent.Started.Model
	}
	fingerprint := agent.Finished.Fingerprint
	if fingerprint == "" && agent.Started != nil {
		fingerprint = agent.Started.Fingerprint
	}

	result := model.RunResult{
		SchemaVersion:        model.SchemaVersion,
		Plugin:               ctx.Plugin,
		Scenario:             ctx.Scenario,
		Arm:                  ctx.Arm,
		Mode:                 ctx.Mode,
		Run:                  ctx.Run,
		AgentAttempt:         agent.ID,
		CurrentSHA256:        SHA256Hex(raw),
		Complete:             true,
		Status:               agent.Finished.Status,
		StopReason:           agent.Finished.StopReason,
		Backend:              backend,
		Model:                modelName,
		Fingerprint:          fingerprint,
		ChecksRequired:       ctx.ChecksRequired,
		TimedOut:             agent.Finished.Status == model.StatusTimedOut,
		StartedAt:            startedAt,
		FinishedAt:           agent.Finished.FinishedAt,
		Mechanical:           mechanical,
		Usage:                agentUsage,
		AgentUsage:           agentUsage,
		JudgeUsage:           judgeUsage,
		CombinedUsage:        combineUsage(agentUsage, judgeUsage),
		Evidence:             evidencePath,
		EvidenceSHA256:       agent.Finished.EvidenceSHA256,
		GradingRequestSHA256: agent.Finished.GradingRequestSHA256,
		JudgeIdentitySHA256:  pointer.JudgeIdentitySHA256,
		Error:                agent.Finished.Error,
	}
	if agent.Finished.AgentExit != nil {
		code := *agent.Finished.AgentExit
		result.AgentExit = &code
	}
	if agent.Finished.TimedOut || agent.Finished.Status == model.StatusTimedOut {
		result.TimedOut = true
	}
	if agent.Finished.DurationSeconds != nil {
		seconds := *agent.Finished.DurationSeconds
		result.DurationSeconds = &seconds
	}
	if judge != nil {
		result.JudgeAttempt = judge.ID
	}
	if ctx.Arm == "with" {
		switch {
		case ctx.SkillUse != nil:
			recorded := *ctx.SkillUse
			result.SkillUse = &recorded
		case len(ctx.Skills) > 0:
			detected := skilluse.DetectPath(filepath.Join(agent.Dir, agentLogName), ctx.Skills, ctx.SkillDir)
			result.SkillUse = &detected
		}
	}
	return result, nil
}

// ErrSuperseded reports that a failed rerun replaced the accepted attempt, so
// current.json no longer describes the run's outcome.
var ErrSuperseded = errors.New("accepted attempt is superseded by a failed rerun")

// EnsureResultProjection rebuilds result.json when it is absent or differs
// from the projection of current.json. current.json wins unless a failed rerun
// superseded it: then result.json describes that rerun and is left alone.
func (s *Store) EnsureResultProjection(ctx RunContext) (model.RunResult, error) {
	if superseding, err := s.Superseded(); err != nil {
		return model.RunResult{}, err
	} else if superseding != nil {
		return model.RunResult{}, fmt.Errorf("%w: %s", ErrSuperseded, superseding.ID)
	}
	result, err := s.BuildResult(ctx)
	if err != nil {
		return model.RunResult{}, err
	}
	desired, err := marshalIndented(result)
	if err != nil {
		return model.RunResult{}, err
	}
	path := s.ResultPath()
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, desired) {
		return result, nil
	}
	if s.Hooks.BeforeProjectionWrite != nil {
		if err := s.Hooks.BeforeProjectionWrite(resultName); err != nil {
			return model.RunResult{}, err
		}
	}
	if err := atomicfile.Write(path, desired, 0o600); err != nil {
		return model.RunResult{}, err
	}
	if s.Hooks.AfterProjectionWrite != nil {
		if err := s.Hooks.AfterProjectionWrite(resultName); err != nil {
			return model.RunResult{}, err
		}
	}
	return result, nil
}

// verifyRequestProjection checks an existing grading-request.json against the
// accepted binding. A missing projection is fine: it is rebuilt from
// current.json. A present projection with different semantics is stale and
// makes the run non-reusable.
func (s *Store) verifyRequestProjection(pointer model.CurrentPointer) error {
	data, err := os.ReadFile(s.GradingRequestPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read grading request projection: %w", err)
	}
	var request model.GradingRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return fmt.Errorf("parse grading request projection: %w", err)
	}
	canonical, err := CanonicalJSON(request)
	if err != nil {
		return err
	}
	if digest := SHA256Hex(canonical); digest != pointer.AgentGradingRequestSHA256 {
		return fmt.Errorf("grading request projection digest %s does not match current binding %s",
			digest, pointer.AgentGradingRequestSHA256)
	}
	return nil
}

// WriteGradingRequest canonicalizes and writes grading-request.json, returning
// the canonical digest. It is used before finished.json and current.json are
// written, so it does not consult the pointer.
func (s *Store) WriteGradingRequest(request model.GradingRequest) (string, error) {
	if request.SchemaVersion == 0 {
		request.SchemaVersion = model.SchemaVersion
	}
	if request.AgentAttempt == "" {
		return "", fmt.Errorf("grading request: missing agent_attempt")
	}
	if request.EvidenceSHA256 == "" {
		return "", fmt.Errorf("grading request: missing evidence_sha256")
	}
	if _, _, ok := parseAttemptID(request.AgentAttempt); !ok {
		return "", fmt.Errorf("grading request: invalid agent_attempt %q", request.AgentAttempt)
	}
	data, err := CanonicalJSON(request)
	if err != nil {
		return "", err
	}
	if err := atomicfile.Write(s.GradingRequestPath(), data, 0o600); err != nil {
		return "", err
	}
	return SHA256Hex(data), nil
}

// ReadGradingRequest reads the derived grading-request projection.
func (s *Store) ReadGradingRequest() (model.GradingRequest, []byte, string, error) {
	data, err := os.ReadFile(s.GradingRequestPath())
	if err != nil {
		return model.GradingRequest{}, nil, "", err
	}
	var request model.GradingRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return model.GradingRequest{}, nil, "", fmt.Errorf("parse grading request: %w", err)
	}
	return request, data, SHA256Hex(data), nil
}

// EnsureGradingRequestProjection validates that request is the accepted
// execution attempt's request and rewrites canonical bytes when the projection
// is absent or stale. The supplied request must hash to the digest recorded by
// current.json; current.json wins over any existing projection.
func (s *Store) EnsureGradingRequestProjection(request model.GradingRequest) (string, error) {
	pointer, _, err := s.LoadCurrent()
	if err != nil {
		return "", err
	}
	if request.SchemaVersion == 0 {
		request.SchemaVersion = model.SchemaVersion
	}
	if request.AgentAttempt != pointer.AgentAttempt {
		return "", fmt.Errorf("grading request agent_attempt %q does not match current %q",
			request.AgentAttempt, pointer.AgentAttempt)
	}
	if request.EvidenceSHA256 != pointer.AgentEvidenceSHA256 {
		return "", fmt.Errorf("grading request evidence_sha256 does not match current pointer")
	}
	data, err := CanonicalJSON(request)
	if err != nil {
		return "", err
	}
	digest := SHA256Hex(data)
	if digest != pointer.AgentGradingRequestSHA256 {
		return "", fmt.Errorf("grading request digest %s does not match current binding %s",
			digest, pointer.AgentGradingRequestSHA256)
	}
	if existing, err := os.ReadFile(s.GradingRequestPath()); err == nil && bytes.Equal(existing, data) {
		return digest, nil
	}
	name := requestName
	if s.Hooks.BeforeProjectionWrite != nil {
		if err := s.Hooks.BeforeProjectionWrite(name); err != nil {
			return "", err
		}
	}
	if err := atomicfile.Write(s.GradingRequestPath(), data, 0o600); err != nil {
		return "", err
	}
	if s.Hooks.AfterProjectionWrite != nil {
		if err := s.Hooks.AfterProjectionWrite(name); err != nil {
			return "", err
		}
	}
	return digest, nil
}
