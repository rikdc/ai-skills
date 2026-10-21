package attempts

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rikdc/ai-skills/internal/model"
)

const defaultAcceptedBy = "evalctl/" + model.Version

// AcceptOptions controls automatic acceptance of an execution attempt.
type AcceptOptions struct {
	// JudgeAttempt optionally accepts a finished judge attempt in the same
	// pointer transition.
	JudgeAttempt string
	// JudgeIdentitySHA256 optionally requires the judge identity to match.
	JudgeIdentitySHA256 string
	// AcceptedBy records who accepted the pointer. Empty means
	// evalctl/<version>.
	AcceptedBy string
}

// LoadCurrent reads current.json and returns its raw bytes. The raw bytes are
// the input to the result projection's current_sha256.
func (s *Store) LoadCurrent() (model.CurrentPointer, []byte, error) {
	path := s.CurrentPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return model.CurrentPointer{}, nil, ErrNoCurrent
	}
	if err != nil {
		return model.CurrentPointer{}, nil, fmt.Errorf("read current pointer: %w", err)
	}
	var pointer model.CurrentPointer
	if err := json.Unmarshal(data, &pointer); err != nil {
		return model.CurrentPointer{}, nil, fmt.Errorf("%w: %v", ErrInvalidCurrent, err)
	}
	if err := validateCurrent(pointer); err != nil {
		return model.CurrentPointer{}, nil, err
	}
	return pointer, data, nil
}

// CurrentSHA256 returns the digest of the raw current.json bytes.
func (s *Store) CurrentSHA256() (string, error) {
	_, data, err := s.LoadCurrent()
	if err != nil {
		return "", err
	}
	return SHA256Hex(data), nil
}

func validateCurrent(pointer model.CurrentPointer) error {
	if pointer.SchemaVersion != model.SchemaVersion {
		return fmt.Errorf("%w: schema_version %d", ErrInvalidCurrent, pointer.SchemaVersion)
	}
	kind, _, ok := parseAttemptID(pointer.AgentAttempt)
	if !ok || (kind != model.KindAgent && kind != model.KindHook) {
		return fmt.Errorf("%w: agent_attempt %q", ErrInvalidCurrent, pointer.AgentAttempt)
	}
	if pointer.AgentEvidenceSHA256 == "" {
		return fmt.Errorf("%w: missing agent_evidence_sha256", ErrInvalidCurrent)
	}
	if pointer.AgentGradingRequestSHA256 == "" {
		return fmt.Errorf("%w: missing agent_grading_request_sha256", ErrInvalidCurrent)
	}
	if pointer.AcceptedAt == "" || pointer.AcceptedBy == "" {
		return fmt.Errorf("%w: missing accepted_at/accepted_by", ErrInvalidCurrent)
	}
	if pointer.JudgeAttempt != "" {
		judgeKind, _, ok := parseAttemptID(pointer.JudgeAttempt)
		if !ok || judgeKind != model.KindJudge {
			return fmt.Errorf("%w: judge_attempt %q", ErrInvalidCurrent, pointer.JudgeAttempt)
		}
		if pointer.JudgeIdentitySHA256 == "" {
			return fmt.Errorf("%w: missing judge_identity_sha256", ErrInvalidCurrent)
		}
		if pointer.GradedAgentAttempt != pointer.AgentAttempt {
			return fmt.Errorf("%w: graded_agent_attempt does not match agent_attempt", ErrInvalidCurrent)
		}
		if pointer.GradedEvidenceSHA256 != pointer.AgentEvidenceSHA256 {
			return fmt.Errorf("%w: graded_evidence_sha256 does not match agent_evidence_sha256", ErrInvalidCurrent)
		}
		if pointer.GradingRequestSHA256 != pointer.AgentGradingRequestSHA256 {
			return fmt.Errorf("%w: grading_request_sha256 does not match agent_grading_request_sha256", ErrInvalidCurrent)
		}
	} else if pointer.GradedAgentAttempt != "" || pointer.GradedEvidenceSHA256 != "" ||
		pointer.GradingRequestSHA256 != "" || pointer.JudgeIdentitySHA256 != "" {
		return fmt.Errorf("%w: judge binding fields without judge_attempt", ErrInvalidCurrent)
	}
	return nil
}

// WriteCurrent atomically replaces current.json. This is the only mutable
// acceptance state in a run.
func (s *Store) WriteCurrent(pointer model.CurrentPointer) error {
	if pointer.SchemaVersion == 0 {
		pointer.SchemaVersion = model.SchemaVersion
	}
	if pointer.AcceptedAt == "" {
		pointer.AcceptedAt = s.timestamp()
	}
	if pointer.AcceptedBy == "" {
		pointer.AcceptedBy = defaultAcceptedBy
	}
	if err := validateCurrent(pointer); err != nil {
		return err
	}
	return s.replaceCurrent(pointer)
}

func (s *Store) replaceCurrent(pointer model.CurrentPointer) error {
	data, err := marshalIndented(pointer)
	if err != nil {
		return fmt.Errorf("encode current pointer: %w", err)
	}
	temp, err := os.CreateTemp(s.RunDir, ".current.json.tmp-*")
	if err != nil {
		return fmt.Errorf("create current temp: %w", err)
	}
	tempName := temp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tempName)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("chmod current temp: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write current temp: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync current temp: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close current temp: %w", err)
	}
	if s.Hooks.BeforePointerRename != nil {
		if err := s.Hooks.BeforePointerRename(); err != nil {
			return err
		}
	}
	if err := os.Rename(tempName, s.CurrentPath()); err != nil {
		return fmt.Errorf("rename current pointer: %w", err)
	}
	renamed = true
	// The pointer is visible after the rename even if the caller treats a
	// post-rename hook as a crash.
	if s.Hooks.AfterPointerRename != nil {
		if err := s.Hooks.AfterPointerRename(); err != nil {
			return err
		}
	}
	if err := syncDir(s.RunDir); err != nil {
		return fmt.Errorf("fsync run directory: %w", err)
	}
	return nil
}

// Accept validates an execution attempt and atomically makes it current. Only
// finished ok agent/hook attempts are accepted. A judge, when supplied, must be
// finished ok and bound to the same execution attempt, evidence digest, and
// grading-request digest.
func (s *Store) Accept(agentID string, opts AcceptOptions) (model.CurrentPointer, error) {
	agent, err := s.LoadAttempt(agentID)
	if err != nil {
		return model.CurrentPointer{}, err
	}
	if err := validateAcceptableExecution(agent); err != nil {
		return model.CurrentPointer{}, err
	}
	var judge *Attempt
	if opts.JudgeAttempt != "" {
		judge, err = s.LoadAttempt(opts.JudgeAttempt)
		if err != nil {
			return model.CurrentPointer{}, err
		}
		if err := validateAcceptableJudge(judge, agent, opts.JudgeIdentitySHA256); err != nil {
			return model.CurrentPointer{}, err
		}
	}
	pointer := buildCurrent(agent, judge, opts.AcceptedBy, s.timestamp())
	if err := s.replaceCurrent(pointer); err != nil {
		return model.CurrentPointer{}, err
	}
	return pointer, nil
}

func validateAcceptableExecution(attempt *Attempt) error {
	if attempt.Finished == nil {
		return fmt.Errorf("attempt %s is interrupted and non-reusable", attempt.ID)
	}
	if attempt.Kind != model.KindAgent && attempt.Kind != model.KindHook {
		return fmt.Errorf("attempt %s is not an execution attempt", attempt.ID)
	}
	if attempt.Finished.Status != model.StatusOK {
		return fmt.Errorf("attempt %s has status %q, not ok", attempt.ID, attempt.Finished.Status)
	}
	if attempt.Finished.ChecksStatus == model.ChecksFailed {
		return fmt.Errorf("attempt %s failed mechanical checks", attempt.ID)
	}
	if attempt.Finished.EvidenceSHA256 == "" {
		return fmt.Errorf("attempt %s has no evidence digest", attempt.ID)
	}
	if attempt.Finished.GradingRequestSHA256 == "" {
		return fmt.Errorf("attempt %s has no grading-request digest", attempt.ID)
	}
	if err := VerifyEvidence(attempt); err != nil {
		return fmt.Errorf("attempt %s evidence: %w", attempt.ID, err)
	}
	return nil
}

func validateAcceptableJudge(judge, agent *Attempt, requiredIdentity string) error {
	if judge.Kind != model.KindJudge {
		return fmt.Errorf("attempt %s is not a judge attempt", judge.ID)
	}
	if judge.Finished == nil {
		return fmt.Errorf("judge attempt %s is interrupted", judge.ID)
	}
	if judge.Finished.Status != model.StatusOK {
		return fmt.Errorf("judge attempt %s has status %q, not ok", judge.ID, judge.Finished.Status)
	}
	if judge.Finished.GradedAgentAttempt != agent.ID {
		return fmt.Errorf("judge attempt %s is bound to agent attempt %q, not %q",
			judge.ID, judge.Finished.GradedAgentAttempt, agent.ID)
	}
	if judge.Finished.GradedEvidenceSHA256 != agent.Finished.EvidenceSHA256 {
		return fmt.Errorf("judge attempt %s evidence binding does not match", judge.ID)
	}
	if judge.Finished.GradingRequestSHA256 != agent.Finished.GradingRequestSHA256 {
		return fmt.Errorf("judge attempt %s grading-request binding does not match", judge.ID)
	}
	if judge.Finished.JudgeIdentitySHA256 == "" {
		return fmt.Errorf("judge attempt %s has no judge identity digest", judge.ID)
	}
	if requiredIdentity != "" && judge.Finished.JudgeIdentitySHA256 != requiredIdentity {
		return fmt.Errorf("judge attempt %s identity does not match configured judge", judge.ID)
	}
	if judge.Finished.JudgementSHA256 == "" {
		return fmt.Errorf("judge attempt %s has no judgement digest", judge.ID)
	}
	if err := VerifyJudgement(judge); err != nil {
		return fmt.Errorf("judge attempt %s judgement: %w", judge.ID, err)
	}
	return nil
}

func buildCurrent(agent, judge *Attempt, acceptedBy, acceptedAt string) model.CurrentPointer {
	if acceptedBy == "" {
		acceptedBy = defaultAcceptedBy
	}
	pointer := model.CurrentPointer{
		SchemaVersion:             model.SchemaVersion,
		AgentAttempt:              agent.ID,
		AgentEvidenceSHA256:       agent.Finished.EvidenceSHA256,
		AgentGradingRequestSHA256: agent.Finished.GradingRequestSHA256,
		AcceptedAt:                acceptedAt,
		AcceptedBy:                acceptedBy,
	}
	if judge != nil {
		pointer.JudgeAttempt = judge.ID
		pointer.JudgeIdentitySHA256 = judge.Finished.JudgeIdentitySHA256
		pointer.GradedAgentAttempt = judge.Finished.GradedAgentAttempt
		pointer.GradedEvidenceSHA256 = judge.Finished.GradedEvidenceSHA256
		pointer.GradingRequestSHA256 = judge.Finished.GradingRequestSHA256
	}
	return pointer
}

// CheckReusable implements the completion predicate from spec §7.2. A nil
// error means the accepted attempt and optional judge are complete and match
// the supplied fingerprint. Missing result.json/grading-request.json
// projections never make a run non-reusable; they are rebuilt separately.
func (s *Store) CheckReusable(fingerprint string) error {
	pointer, _, err := s.LoadCurrent()
	if err != nil {
		return err
	}
	agent, err := s.LoadAttempt(pointer.AgentAttempt)
	if err != nil {
		return err
	}
	if agent.Finished == nil {
		return fmt.Errorf("accepted attempt %s is interrupted", agent.ID)
	}
	if agent.Kind != model.KindAgent && agent.Kind != model.KindHook {
		return fmt.Errorf("accepted attempt %s is not an execution attempt", agent.ID)
	}
	if agent.Finished.Status != model.StatusOK {
		return fmt.Errorf("accepted attempt %s has status %q", agent.ID, agent.Finished.Status)
	}
	gotFingerprint := agent.Finished.Fingerprint
	if gotFingerprint == "" && agent.Started != nil {
		gotFingerprint = agent.Started.Fingerprint
	}
	if gotFingerprint != fingerprint {
		return fmt.Errorf("accepted attempt fingerprint %q does not match %q", gotFingerprint, fingerprint)
	}
	if agent.Finished.EvidenceSHA256 != pointer.AgentEvidenceSHA256 {
		return fmt.Errorf("accepted attempt evidence digest does not match current pointer")
	}
	if agent.Finished.GradingRequestSHA256 != pointer.AgentGradingRequestSHA256 {
		return fmt.Errorf("accepted attempt grading-request digest does not match current pointer")
	}
	superseding, err := s.supersedingAfter(agent)
	if err != nil {
		return err
	}
	if superseding != nil {
		return fmt.Errorf("attempt %s after accepted attempt %s did not finish ok", superseding.ID, agent.ID)
	}
	if err := VerifyEvidence(agent); err != nil {
		return fmt.Errorf("accepted attempt evidence: %w", err)
	}
	if !checksSatisfied(agent) {
		return fmt.Errorf("accepted attempt mechanical checks are not satisfied")
	}
	if err := s.verifyRequestProjection(pointer); err != nil {
		return err
	}
	if pointer.JudgeAttempt != "" {
		judge, err := s.LoadAttempt(pointer.JudgeAttempt)
		if err != nil {
			return err
		}
		if err := validateJudgeBinding(judge, pointer); err != nil {
			return err
		}
	}
	return nil
}

// LatestExecution returns the newest agent or hook attempt in list, or nil.
func LatestExecution(list []*Attempt) *Attempt {
	var latest *Attempt
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

// Supersedes reports whether latest replaces accepted as the run's outcome:
// it is a newer execution attempt that finished with a non-ok status
// (including cancelled and timed out). That rerun is the run's current
// measurement; the accepted pointer stays on disk as provenance but is no
// longer the run's result or reusable. An unfinished attempt is in flight or
// crashed and never supersedes.
func Supersedes(latest, accepted *Attempt) bool {
	if latest == nil || accepted == nil || latest.Sequence <= accepted.Sequence {
		return false
	}
	return latest.Finished != nil && latest.Finished.Status != model.StatusOK
}

// Superseding returns the attempt that supersedes the accepted one, or nil
// when there is no accepted attempt or it is still the run's outcome. It reads
// only the newest execution attempt, so unrelated corrupt attempts are ignored.
func (s *Store) Superseding() (*Attempt, error) {
	pointer, _, err := s.LoadCurrent()
	if err != nil {
		return nil, nil
	}
	accepted, err := s.LoadAttempt(pointer.AgentAttempt)
	if err != nil {
		return nil, nil
	}
	return s.supersedingAfter(accepted)
}

func (s *Store) supersedingAfter(accepted *Attempt) (*Attempt, error) {
	entries, err := os.ReadDir(s.AttemptsDir())
	if err != nil {
		return nil, fmt.Errorf("list attempts: %w", err)
	}
	newest, newestSeq := "", accepted.Sequence
	for _, entry := range entries {
		kind, sequence, ok := parseAttemptID(entry.Name())
		if !ok || !entry.IsDir() || (kind != model.KindAgent && kind != model.KindHook) {
			continue
		}
		if sequence > newestSeq {
			newest, newestSeq = entry.Name(), sequence
		}
	}
	if newest == "" {
		return nil, nil
	}
	latest, err := s.LoadAttempt(newest)
	if err != nil {
		return nil, fmt.Errorf("newer attempt %s: %w", newest, err)
	}
	if !Supersedes(latest, accepted) {
		return nil, nil
	}
	return latest, nil
}

// Reusable reports whether the completion predicate holds for fingerprint.
func (s *Store) Reusable(fingerprint string) bool {
	return s.CheckReusable(fingerprint) == nil
}

func validateJudgeBinding(judge *Attempt, pointer model.CurrentPointer) error {
	if judge.Kind != model.KindJudge {
		return fmt.Errorf("accepted judge attempt %s is not a judge attempt", judge.ID)
	}
	if judge.Finished == nil || judge.Finished.Status != model.StatusOK {
		return fmt.Errorf("accepted judge attempt %s is not finished ok", judge.ID)
	}
	if judge.Finished.GradedAgentAttempt != pointer.AgentAttempt {
		return fmt.Errorf("accepted judge attempt is bound to a different agent attempt")
	}
	if judge.Finished.GradedEvidenceSHA256 != pointer.AgentEvidenceSHA256 {
		return fmt.Errorf("accepted judge attempt evidence binding does not match")
	}
	if judge.Finished.GradingRequestSHA256 != pointer.AgentGradingRequestSHA256 {
		return fmt.Errorf("accepted judge attempt grading-request binding does not match")
	}
	if judge.Finished.JudgeIdentitySHA256 != pointer.JudgeIdentitySHA256 {
		return fmt.Errorf("accepted judge attempt identity does not match current pointer")
	}
	if err := VerifyJudgement(judge); err != nil {
		return err
	}
	return nil
}

// checksSatisfied reports whether the mechanical gate allows reuse. When
// checks ran, mechanical.json must exist and parse; failure never does.
func checksSatisfied(attempt *Attempt) bool {
	if attempt == nil || attempt.Finished == nil {
		return false
	}
	switch attempt.Finished.ChecksStatus {
	case model.ChecksFailed:
		return false
	case model.ChecksOK:
		data, err := os.ReadFile(filepath.Join(attempt.Dir, "mechanical.json"))
		if err != nil {
			return false
		}
		var mechanical model.Mechanical
		return json.Unmarshal(data, &mechanical) == nil
	default:
		// Empty and not-applicable both mean no mechanical gate applies.
		return true
	}
}

// AdoptOptions controls recovery of a finished-but-unaccepted attempt.
type AdoptOptions struct {
	// Fingerprint is the current configured scenario fingerprint. Required.
	Fingerprint string
	// ConfigSHA256 optionally requires the started attempt config digest.
	ConfigSHA256 string
	// JudgeIdentitySHA256 optionally requires a matching judge identity.
	JudgeIdentitySHA256 string
	// AcceptedBy records the accepting actor. Empty means evalctl/<version>.
	AcceptedBy string
}

// Adopt atomically accepts the highest matching finished ok execution attempt,
// plus a matching finished judge attempt when one exists. It never merges or
// reruns attempts. The boolean reports whether the pointer changed.
func (s *Store) Adopt(opts AdoptOptions) (*model.CurrentPointer, bool, error) {
	if opts.Fingerprint == "" {
		return nil, false, fmt.Errorf("adopt: fingerprint is required")
	}
	all, err := s.Enumerate()
	if err != nil {
		return nil, false, err
	}
	// An older ok attempt behind a newer failed one is not the run's outcome.
	if latest := LatestExecution(all); latest != nil &&
		latest.Finished != nil && latest.Finished.Status != model.StatusOK {
		return nil, false, nil
	}
	var execution *Attempt
	for _, attempt := range all {
		if !adoptionCandidate(attempt, opts) {
			continue
		}
		if execution == nil || attempt.Sequence > execution.Sequence {
			execution = attempt
		}
	}
	if execution == nil {
		return nil, false, nil
	}
	var judge *Attempt
	for _, attempt := range all {
		if attempt.Kind != model.KindJudge || attempt.Finished == nil ||
			attempt.Finished.Status != model.StatusOK {
			continue
		}
		if err := validateJudgeForAdoption(attempt, execution, opts.JudgeIdentitySHA256); err != nil {
			continue
		}
		if judge == nil || attempt.Sequence > judge.Sequence {
			judge = attempt
		}
	}
	pointer := buildCurrent(execution, judge, opts.AcceptedBy, s.timestamp())
	if existing, _, err := s.LoadCurrent(); err == nil {
		if sameAcceptance(existing, pointer) {
			return &existing, false, nil
		}
	}
	if err := s.replaceCurrent(pointer); err != nil {
		return nil, false, err
	}
	return &pointer, true, nil
}

func adoptionCandidate(attempt *Attempt, opts AdoptOptions) bool {
	if attempt.Kind != model.KindAgent && attempt.Kind != model.KindHook {
		return false
	}
	if attempt.Finished == nil || attempt.Finished.Status != model.StatusOK {
		return false
	}
	if attempt.Finished.Fingerprint != opts.Fingerprint {
		return false
	}
	if opts.ConfigSHA256 != "" {
		if attempt.Started == nil || attempt.Started.ConfigSHA256 != opts.ConfigSHA256 {
			return false
		}
	}
	if attempt.Finished.EvidenceSHA256 == "" || attempt.Finished.GradingRequestSHA256 == "" {
		return false
	}
	if !checksSatisfied(attempt) {
		return false
	}
	if err := VerifyEvidence(attempt); err != nil {
		return false
	}
	return true
}

func validateJudgeForAdoption(judge, execution *Attempt, requiredIdentity string) error {
	if judge.Finished.GradedAgentAttempt != execution.ID {
		return fmt.Errorf("bound to a different execution attempt")
	}
	if judge.Finished.GradedEvidenceSHA256 != execution.Finished.EvidenceSHA256 {
		return fmt.Errorf("evidence binding does not match")
	}
	if judge.Finished.GradingRequestSHA256 != execution.Finished.GradingRequestSHA256 {
		return fmt.Errorf("grading-request binding does not match")
	}
	if judge.Finished.JudgeIdentitySHA256 == "" {
		return fmt.Errorf("missing judge identity")
	}
	if requiredIdentity != "" && judge.Finished.JudgeIdentitySHA256 != requiredIdentity {
		return fmt.Errorf("judge identity does not match")
	}
	if err := VerifyJudgement(judge); err != nil {
		return err
	}
	return nil
}

func sameAcceptance(a, b model.CurrentPointer) bool {
	return a.AgentAttempt == b.AgentAttempt &&
		a.AgentEvidenceSHA256 == b.AgentEvidenceSHA256 &&
		a.AgentGradingRequestSHA256 == b.AgentGradingRequestSHA256 &&
		a.JudgeAttempt == b.JudgeAttempt &&
		a.JudgeIdentitySHA256 == b.JudgeIdentitySHA256 &&
		a.GradedAgentAttempt == b.GradedAgentAttempt &&
		a.GradedEvidenceSHA256 == b.GradedEvidenceSHA256 &&
		a.GradingRequestSHA256 == b.GradingRequestSHA256
}
