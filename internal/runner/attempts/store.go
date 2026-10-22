// Package attempts implements the v3 immutable attempt store and the
// current-pointer acceptance contract for one run directory.
//
// The authoritative source of truth is current.json plus immutable
// attempts/<kind>-<sequence>/ directories. result.json and
// grading-request.json are derived projections.
package attempts

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/platform/atomicfile"
)

// Store errors. Callers may use errors.Is to distinguish states.
var (
	ErrNoStarted        = errors.New("attempt has no started.json")
	ErrAlreadyFinished  = errors.New("attempt is already finished")
	ErrNotFound         = errors.New("attempt not found")
	ErrInvalidAttemptID = errors.New("invalid attempt id")
	ErrInvalidAttempt   = errors.New("invalid attempt record")
	ErrNoCurrent        = errors.New("run has no current.json")
	ErrInvalidCurrent   = errors.New("invalid current.json")
	ErrNoAccepted       = errors.New("run has no accepted attempt")
)

const (
	startedName  = "started.json"
	finishedName = "finished.json"
	currentName  = "current.json"
	resultName   = "result.json"
	requestName  = "grading-request.json"
	indexName    = "index.jsonl"
)

// Hooks are deterministic failure-injection points used by crash-recovery
// tests. A non-nil error is returned to the caller at the named step and the
// store leaves the on-disk state exactly as that crash point would.
type Hooks struct {
	// AfterAllocate runs after started.json has been written and fsynced but
	// before the caller starts setup or paid work.
	AfterAllocate func(attemptID string) error
	// BeforeFinishWrite runs before finished.json is written.
	BeforeFinishWrite func(attemptID string) error
	// AfterFinishWrite runs after finished.json has been atomically written.
	AfterFinishWrite func(attemptID string) error
	// BeforePointerRename runs after the current.json temp file has been
	// fsynced and closed but before it is renamed into place.
	BeforePointerRename func() error
	// AfterPointerRename runs after current.json has been renamed into place
	// but before the run directory is fsynced.
	AfterPointerRename func() error
	// BeforeProjectionWrite runs before result.json or grading-request.json
	// is replaced.
	BeforeProjectionWrite func(name string) error
	// AfterProjectionWrite runs after a projection has been replaced.
	AfterProjectionWrite func(name string) error
}

// Store operates on one run directory.
type Store struct {
	RunDir string
	Hooks  Hooks
	// Now supplies timestamps; tests may override it.
	Now func() time.Time
}

// New returns a store rooted at runDir.
func New(runDir string) *Store {
	return &Store{RunDir: runDir}
}

// AttemptsDir is the directory holding immutable attempt directories.
func (s *Store) AttemptsDir() string { return filepath.Join(s.RunDir, "attempts") }

// CurrentPath is the mutable acceptance pointer.
func (s *Store) CurrentPath() string { return filepath.Join(s.RunDir, currentName) }

// ResultPath is the derived result projection.
func (s *Store) ResultPath() string { return filepath.Join(s.RunDir, resultName) }

// GradingRequestPath is the derived grading-request projection.
func (s *Store) GradingRequestPath() string { return filepath.Join(s.RunDir, requestName) }

// IndexPath is the optional derived attempt index.
func (s *Store) IndexPath() string { return filepath.Join(s.AttemptsDir(), indexName) }

// Attempt is one attempt directory with its immutable records.
type Attempt struct {
	ID       string
	Dir      string
	Kind     model.AttemptKind
	Sequence int
	Started  *model.StartedAttempt
	Finished *model.FinishedAttempt
	// Status is the finished status, or interrupted when a valid started
	// record has no finished record.
	Status model.RunStatus
}

// Interrupted reports whether the attempt has a started record but no
// finished outcome.
func (a *Attempt) Interrupted() bool { return a.Finished == nil }

// EffectiveUsage returns the attempt usage. An interrupted attempt has unknown
// usage, never zero.
func (a *Attempt) EffectiveUsage() model.Usage {
	if a.Finished != nil {
		return a.Finished.Usage
	}
	return model.Usage{
		State:  model.UsageUnknown,
		Known:  false,
		Source: "attempt-store",
		Reason: "interrupted: no finished record",
	}
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Store) timestamp() string {
	return s.now().Format(time.RFC3339Nano)
}

func validKind(kind model.AttemptKind) bool {
	switch kind {
	case model.KindAgent, model.KindHook, model.KindJudge:
		return true
	default:
		return false
	}
}

func parseAttemptID(name string) (model.AttemptKind, int, bool) {
	for _, kind := range []model.AttemptKind{model.KindAgent, model.KindHook, model.KindJudge} {
		prefix := string(kind) + "-"
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		digits := name[len(prefix):]
		if digits == "" {
			continue
		}
		for _, r := range digits {
			if r < '0' || r > '9' {
				return "", 0, false
			}
		}
		sequence, err := strconv.Atoi(digits)
		if err != nil || sequence <= 0 {
			return "", 0, false
		}
		return kind, sequence, true
	}
	return "", 0, false
}

func formatAttemptID(kind model.AttemptKind, sequence int) string {
	return fmt.Sprintf("%s-%06d", kind, sequence)
}

func marshalIndented(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func writeIndented(path string, v any) error {
	data, err := marshalIndented(v)
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	return atomicfile.Write(path, data, 0o600)
}

func syncDir(dir string) error {
	handle, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := handle.Sync(); err != nil {
		_ = handle.Close()
		return err
	}
	return handle.Close()
}

func (s *Store) nextSequence() (int, error) {
	entries, err := os.ReadDir(s.AttemptsDir())
	if errors.Is(err, fs.ErrNotExist) {
		return 1, nil
	}
	if err != nil {
		return 0, fmt.Errorf("list attempts: %w", err)
	}
	next := 1
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		_, sequence, ok := parseAttemptID(entry.Name())
		if ok && sequence >= next {
			next = sequence + 1
		}
	}
	return next, nil
}

// Allocate creates the next immutable attempt directory, fsyncs its parent,
// and writes started.json before returning. The caller must not start setup or
// paid work if Allocate fails.
func (s *Store) Allocate(started model.StartedAttempt) (*Attempt, error) {
	if !validKind(started.Kind) {
		return nil, fmt.Errorf("%w: kind %q", ErrInvalidAttempt, started.Kind)
	}
	if err := os.MkdirAll(s.AttemptsDir(), 0o700); err != nil {
		return nil, fmt.Errorf("create attempts directory: %w", err)
	}
	sequence, err := s.nextSequence()
	if err != nil {
		return nil, err
	}

	for {
		id := formatAttemptID(started.Kind, sequence)
		dir := filepath.Join(s.AttemptsDir(), id)
		err := os.Mkdir(dir, 0o700)
		if err == nil {
			if err := syncDir(s.AttemptsDir()); err != nil {
				return nil, fmt.Errorf("fsync attempts directory: %w", err)
			}
			record := started
			record.SchemaVersion = model.SchemaVersion
			record.AttemptID = id
			if record.StartedAt == "" {
				record.StartedAt = s.timestamp()
			}
			if err := writeIndented(filepath.Join(dir, startedName), record); err != nil {
				// Allocation failed before any paid work: the directory has no
				// valid started record and is not an attempt.
				_ = os.Remove(dir)
				return nil, err
			}
			attempt := &Attempt{
				ID:       id,
				Dir:      dir,
				Kind:     record.Kind,
				Sequence: sequence,
				Started:  &record,
				Status:   model.StatusInterrupted,
			}
			if s.Hooks.AfterAllocate != nil {
				if err := s.Hooks.AfterAllocate(id); err != nil {
					return nil, err
				}
			}
			return attempt, nil
		}
		if errors.Is(err, fs.ErrExist) {
			sequence++
			continue
		}
		return nil, fmt.Errorf("create attempt %s: %w", id, err)
	}
}

func readStarted(dir, id string, kind model.AttemptKind) (*model.StartedAttempt, error) {
	path := filepath.Join(dir, startedName)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNoStarted, id)
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var record model.StartedAttempt
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidAttempt, path, err)
	}
	if record.SchemaVersion != model.SchemaVersion {
		return nil, fmt.Errorf("%w: %s: schema_version %d", ErrInvalidAttempt, path, record.SchemaVersion)
	}
	if record.AttemptID != id {
		return nil, fmt.Errorf("%w: %s: attempt_id %q", ErrInvalidAttempt, path, record.AttemptID)
	}
	if record.Kind != kind || !validKind(record.Kind) {
		return nil, fmt.Errorf("%w: %s: kind %q", ErrInvalidAttempt, path, record.Kind)
	}
	if record.StartedAt == "" {
		return nil, fmt.Errorf("%w: %s: missing started_at", ErrInvalidAttempt, path)
	}
	return &record, nil
}

func readFinished(dir, id string, kind model.AttemptKind) (*model.FinishedAttempt, error) {
	path := filepath.Join(dir, finishedName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var record model.FinishedAttempt
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidAttempt, path, err)
	}
	if record.SchemaVersion != model.SchemaVersion {
		return nil, fmt.Errorf("%w: %s: schema_version %d", ErrInvalidAttempt, path, record.SchemaVersion)
	}
	if record.AttemptID != id {
		return nil, fmt.Errorf("%w: %s: attempt_id %q", ErrInvalidAttempt, path, record.AttemptID)
	}
	if record.Kind != kind {
		return nil, fmt.Errorf("%w: %s: kind %q", ErrInvalidAttempt, path, record.Kind)
	}
	if record.Status == model.StatusInterrupted || !record.Status.Valid() {
		return nil, fmt.Errorf("%w: %s: status %q", ErrInvalidAttempt, path, record.Status)
	}
	if record.FinishedAt == "" {
		return nil, fmt.Errorf("%w: %s: missing finished_at", ErrInvalidAttempt, path)
	}
	return &record, nil
}

// LoadAttempt reads one attempt by ID.
func (s *Store) LoadAttempt(id string) (*Attempt, error) {
	kind, sequence, ok := parseAttemptID(id)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrInvalidAttemptID, id)
	}
	dir := filepath.Join(s.AttemptsDir(), id)
	started, err := readStarted(dir, id, kind)
	if errors.Is(err, ErrNoStarted) || errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	attempt := &Attempt{
		ID:       id,
		Dir:      dir,
		Kind:     kind,
		Sequence: sequence,
		Started:  started,
		Status:   model.StatusInterrupted,
	}
	finished, err := readFinished(dir, id, kind)
	if err == nil {
		attempt.Finished = finished
		attempt.Status = finished.Status
		return attempt, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return attempt, nil
}

// Finish writes the immutable finished.json outcome exactly once. The attempt
// must already have a valid started record.
func (s *Store) Finish(id string, finished model.FinishedAttempt) (*Attempt, error) {
	attempt, err := s.LoadAttempt(id)
	if err != nil {
		return nil, err
	}
	if attempt.Finished != nil {
		return nil, fmt.Errorf("%w: %s", ErrAlreadyFinished, id)
	}
	if finished.Kind != "" && finished.Kind != attempt.Kind {
		return nil, fmt.Errorf("%w: kind %q does not match %q", ErrInvalidAttempt, finished.Kind, attempt.Kind)
	}
	if finished.Status == model.StatusInterrupted {
		return nil, fmt.Errorf("%w: interrupted is derived and cannot be recorded", ErrInvalidAttempt)
	}
	if finished.Status == "" || !finished.Status.Valid() {
		return nil, fmt.Errorf("%w: status %q", ErrInvalidAttempt, finished.Status)
	}
	record := finished
	record.SchemaVersion = model.SchemaVersion
	record.AttemptID = id
	record.Kind = attempt.Kind
	if record.FinishedAt == "" {
		record.FinishedAt = s.timestamp()
	}
	if record.Fingerprint == "" && attempt.Started != nil {
		record.Fingerprint = attempt.Started.Fingerprint
	}
	if attempt.Started != nil && record.Fingerprint != "" && attempt.Started.Fingerprint != "" &&
		record.Fingerprint != attempt.Started.Fingerprint {
		return nil, fmt.Errorf("%w: fingerprint %q does not match started fingerprint %q",
			ErrInvalidAttempt, record.Fingerprint, attempt.Started.Fingerprint)
	}
	if s.Hooks.BeforeFinishWrite != nil {
		if err := s.Hooks.BeforeFinishWrite(id); err != nil {
			return nil, err
		}
	}
	if err := writeIndented(filepath.Join(attempt.Dir, finishedName), record); err != nil {
		return nil, err
	}
	if s.Hooks.AfterFinishWrite != nil {
		if err := s.Hooks.AfterFinishWrite(id); err != nil {
			return nil, err
		}
	}
	attempt.Finished = &record
	attempt.Status = record.Status
	return attempt, nil
}

func isIgnorableAttemptError(err error) bool {
	return errors.Is(err, ErrNoStarted) || errors.Is(err, ErrInvalidAttempt)
}

// Enumerate lists attempts by immutable directory name exactly once, sorted by
// numeric sequence. Directories without a valid started.json are failed
// allocations and are ignored; they are not attempts.
func (s *Store) Enumerate() ([]*Attempt, error) {
	return s.enumerate(func(model.AttemptKind, int) bool { return true })
}

// newerExecutions lists the agent and hook attempts with a sequence above
// after, read exactly like Enumerate. Older attempts are never read, so damage
// to them cannot affect a decision about the accepted attempt.
func (s *Store) newerExecutions(after int) ([]*Attempt, error) {
	return s.enumerate(func(kind model.AttemptKind, sequence int) bool {
		return sequence > after && (kind == model.KindAgent || kind == model.KindHook)
	})
}

func (s *Store) enumerate(include func(model.AttemptKind, int) bool) ([]*Attempt, error) {
	entries, err := os.ReadDir(s.AttemptsDir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list attempts: %w", err)
	}
	var attempts []*Attempt
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		kind, sequence, ok := parseAttemptID(entry.Name())
		if !ok || !include(kind, sequence) {
			continue
		}
		id := entry.Name()
		dir := filepath.Join(s.AttemptsDir(), id)
		started, err := readStarted(dir, id, kind)
		if isIgnorableAttemptError(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		attempt := &Attempt{
			ID:       id,
			Dir:      dir,
			Kind:     kind,
			Sequence: sequence,
			Started:  started,
			Status:   model.StatusInterrupted,
		}
		finished, err := readFinished(dir, id, kind)
		if err == nil {
			attempt.Finished = finished
			attempt.Status = finished.Status
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	sort.Slice(attempts, func(i, j int) bool {
		if attempts[i].Sequence != attempts[j].Sequence {
			return attempts[i].Sequence < attempts[j].Sequence
		}
		return attempts[i].ID < attempts[j].ID
	})
	return attempts, nil
}

// IndexEntry is one line of the optional derived index.
type IndexEntry struct {
	AttemptID  string            `json:"attempt_id"`
	Kind       model.AttemptKind `json:"kind"`
	Sequence   int               `json:"sequence"`
	Status     model.RunStatus   `json:"status"`
	StartedAt  string            `json:"started_at"`
	FinishedAt string            `json:"finished_at,omitempty"`
}

// RebuildIndex rebuilds attempts/index.jsonl from the attempt directories. The
// index is a cache, never authoritative, and is deduplicated by attempt ID.
func (s *Store) RebuildIndex() ([]IndexEntry, error) {
	attempts, err := s.Enumerate()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.AttemptsDir(), 0o700); err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(attempts))
	entries := make([]IndexEntry, 0, len(attempts))
	for _, attempt := range attempts {
		if seen[attempt.ID] {
			continue
		}
		seen[attempt.ID] = true
		entry := IndexEntry{
			AttemptID: attempt.ID,
			Kind:      attempt.Kind,
			Sequence:  attempt.Sequence,
			Status:    attempt.Status,
		}
		if attempt.Started != nil {
			entry.StartedAt = attempt.Started.StartedAt
		}
		if attempt.Finished != nil {
			entry.FinishedAt = attempt.Finished.FinishedAt
		}
		entries = append(entries, entry)
	}
	var buf strings.Builder
	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			return nil, err
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	if err := atomicfile.Write(s.IndexPath(), []byte(buf.String()), 0o600); err != nil {
		return nil, err
	}
	return entries, nil
}

// ReadIndex reads the derived index, ignoring malformed lines and
// deduplicating by attempt ID. It returns nil when the index is absent.
func (s *Store) ReadIndex() ([]IndexEntry, error) {
	data, err := os.ReadFile(s.IndexPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var entries []IndexEntry
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry IndexEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.AttemptID == "" || seen[entry.AttemptID] {
			continue
		}
		seen[entry.AttemptID] = true
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Sequence < entries[j].Sequence })
	return entries, nil
}
