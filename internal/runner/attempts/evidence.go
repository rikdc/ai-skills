package attempts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/platform/atomicfile"
)

const (
	evidenceName = "evidence.json"
	prunedName   = "evidence_pruned.json"
)

// EvidenceFile is one graded artifact in the canonical evidence manifest.
type EvidenceFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// EvidenceManifest is the canonical evidence.json document.
type EvidenceManifest struct {
	SchemaVersion int            `json:"schema_version"`
	Files         []EvidenceFile `json:"files"`
}

// PrunedEvidence records deliberate pruning of heavy evidence. Usage and
// attempt metadata are never pruned.
type PrunedEvidence struct {
	PrunedAt       string   `json:"pruned_at"`
	Paths          []string `json:"paths"`
	Reason         string   `json:"reason"`
	EvidenceSHA256 string   `json:"evidence_sha256"`
}

var protectedEvidence = map[string]bool{
	startedName:        true,
	finishedName:       true,
	"usage.json":       true,
	"judge-usage.json": true,
	evidenceName:       true,
	prunedName:         true,
}

func cleanEvidencePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty evidence path")
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute evidence path %q", path)
	}
	cleaned := filepath.Clean(path)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("escaping evidence path %q", path)
	}
	return cleaned, nil
}

func evidenceDigest(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

// WriteEvidenceManifest builds the canonical evidence manifest for paths
// (relative to the attempt directory), writes evidence.json atomically, and
// returns the digest of the canonical manifest bytes.
func (s *Store) WriteEvidenceManifest(attemptID string, paths []string) (string, error) {
	attempt, err := s.LoadAttempt(attemptID)
	if err != nil {
		return "", err
	}
	if attempt.Finished != nil {
		return "", fmt.Errorf("%w: %s", ErrAlreadyFinished, attemptID)
	}
	unique := make(map[string]bool, len(paths))
	cleaned := make([]string, 0, len(paths))
	for _, path := range paths {
		rel, err := cleanEvidencePath(path)
		if err != nil {
			return "", err
		}
		if unique[rel] {
			continue
		}
		unique[rel] = true
		cleaned = append(cleaned, rel)
	}
	sort.Strings(cleaned)
	manifest := EvidenceManifest{SchemaVersion: model.SchemaVersion}
	for _, rel := range cleaned {
		digest, size, err := evidenceDigest(filepath.Join(attempt.Dir, rel))
		if err != nil {
			return "", fmt.Errorf("evidence %s: %w", rel, err)
		}
		manifest.Files = append(manifest.Files, EvidenceFile{Path: rel, SHA256: digest, Size: size})
	}
	data, err := CanonicalJSON(manifest)
	if err != nil {
		return "", err
	}
	if err := atomicfile.Write(filepath.Join(attempt.Dir, evidenceName), data, 0o600); err != nil {
		return "", err
	}
	return SHA256Hex(data), nil
}

// ReadEvidenceManifest reads the canonical evidence manifest for an attempt.
func ReadEvidenceManifest(attemptDir string) (EvidenceManifest, []byte, error) {
	path := filepath.Join(attemptDir, evidenceName)
	data, err := os.ReadFile(path)
	if err != nil {
		return EvidenceManifest{}, nil, err
	}
	var manifest EvidenceManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return EvidenceManifest{}, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return manifest, data, nil
}

func readPruned(attemptDir string) (map[string]bool, error) {
	path := filepath.Join(attemptDir, prunedName)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	var pruned PrunedEvidence
	if err := json.Unmarshal(data, &pruned); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	set := make(map[string]bool, len(pruned.Paths))
	for _, p := range pruned.Paths {
		set[p] = true
	}
	return set, nil
}

// VerifyEvidence checks that evidence.json is canonical and matches the
// attempt's immutable digest, and that every listed file still matches its
// recorded size and digest unless evidence_pruned.json lists it.
func VerifyEvidence(attempt *Attempt) error {
	if attempt.Finished == nil {
		return fmt.Errorf("attempt %s is not finished", attempt.ID)
	}
	expected := attempt.Finished.EvidenceSHA256
	if expected == "" {
		return fmt.Errorf("attempt %s has no evidence digest", attempt.ID)
	}
	manifest, raw, err := ReadEvidenceManifest(attempt.Dir)
	if err != nil {
		return err
	}
	if got := SHA256Hex(raw); got != expected {
		return fmt.Errorf("evidence manifest digest %s does not match %s", got, expected)
	}
	canonical, err := CanonicalJSON(manifest)
	if err != nil {
		return err
	}
	if !bytes.Equal(canonical, raw) {
		return fmt.Errorf("evidence manifest is not canonical")
	}
	if manifest.SchemaVersion != model.SchemaVersion {
		return fmt.Errorf("evidence manifest schema_version %d", manifest.SchemaVersion)
	}
	pruned, err := readPruned(attempt.Dir)
	if err != nil {
		return err
	}
	for _, file := range manifest.Files {
		rel, err := cleanEvidencePath(file.Path)
		if err != nil {
			return err
		}
		full := filepath.Join(attempt.Dir, rel)
		if _, err := os.Stat(full); err != nil {
			if errors.Is(err, fs.ErrNotExist) && pruned[file.Path] {
				continue
			}
			return fmt.Errorf("evidence %s: %w", file.Path, err)
		}
		digest, size, err := evidenceDigest(full)
		if err != nil {
			return fmt.Errorf("evidence %s: %w", file.Path, err)
		}
		if size != file.Size || digest != file.SHA256 {
			return fmt.Errorf("evidence %s does not match its manifest entry", file.Path)
		}
	}
	return nil
}

// PruneEvidence records a pruning marker and removes the named heavy evidence
// files. Started/finished records, evidence.json, and usage files are never
// deleted. The marker keeps the original evidence digest.
func (s *Store) PruneEvidence(attemptID string, paths []string, reason string) (PrunedEvidence, error) {
	attempt, err := s.LoadAttempt(attemptID)
	if err != nil {
		return PrunedEvidence{}, err
	}
	if attempt.Finished == nil {
		return PrunedEvidence{}, fmt.Errorf("cannot prune unfinished attempt %s", attemptID)
	}
	cleaned := make([]string, 0, len(paths))
	for _, path := range paths {
		rel, err := cleanEvidencePath(path)
		if err != nil {
			return PrunedEvidence{}, err
		}
		if protectedEvidence[filepath.Base(rel)] {
			return PrunedEvidence{}, fmt.Errorf("refusing to prune protected attempt file %q", rel)
		}
		cleaned = append(cleaned, rel)
	}
	sort.Strings(cleaned)

	marker := PrunedEvidence{
		PrunedAt:       s.timestamp(),
		Paths:          cleaned,
		Reason:         reason,
		EvidenceSHA256: attempt.Finished.EvidenceSHA256,
	}
	if existing, err := readPrunedRecord(attempt.Dir); err == nil {
		if existing.EvidenceSHA256 != "" {
			marker.EvidenceSHA256 = existing.EvidenceSHA256
		}
		merged := make(map[string]bool, len(existing.Paths)+len(cleaned))
		for _, p := range existing.Paths {
			merged[p] = true
		}
		for _, p := range cleaned {
			merged[p] = true
		}
		marker.Paths = marker.Paths[:0]
		for p := range merged {
			marker.Paths = append(marker.Paths, p)
		}
		sort.Strings(marker.Paths)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return PrunedEvidence{}, err
	}
	data, err := CanonicalJSON(marker)
	if err != nil {
		return PrunedEvidence{}, err
	}
	// Write the marker before deleting so a crash never hides missing files.
	if err := atomicfile.Write(filepath.Join(attempt.Dir, prunedName), data, 0o600); err != nil {
		return PrunedEvidence{}, err
	}
	for _, rel := range cleaned {
		if err := os.Remove(filepath.Join(attempt.Dir, rel)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return marker, fmt.Errorf("prune %s: %w", rel, err)
		}
	}
	if err := syncDir(attempt.Dir); err != nil {
		return marker, fmt.Errorf("fsync pruned attempt directory: %w", err)
	}
	return marker, nil
}

func readPrunedRecord(attemptDir string) (PrunedEvidence, error) {
	data, err := os.ReadFile(filepath.Join(attemptDir, prunedName))
	if err != nil {
		return PrunedEvidence{}, err
	}
	var pruned PrunedEvidence
	if err := json.Unmarshal(data, &pruned); err != nil {
		return PrunedEvidence{}, fmt.Errorf("parse pruning marker: %w", err)
	}
	return pruned, nil
}

// VerifyJudgement validates the judge attempt's judgement.json digest and
// structure. Strict score validation belongs to the judge package; this only
// establishes the evidence binding.
func VerifyJudgement(judge *Attempt) error {
	if judge.Finished == nil {
		return fmt.Errorf("judge attempt %s is not finished", judge.ID)
	}
	expected := judge.Finished.JudgementSHA256
	if expected == "" {
		return fmt.Errorf("judge attempt %s has no judgement digest", judge.ID)
	}
	data, err := os.ReadFile(filepath.Join(judge.Dir, "judgement.json"))
	if err != nil {
		return fmt.Errorf("judge attempt %s judgement: %w", judge.ID, err)
	}
	if got := SHA256Hex(data); got != expected {
		return fmt.Errorf("judge attempt %s judgement digest does not match", judge.ID)
	}
	var items []model.JudgedItem
	if err := json.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("judge attempt %s judgement: %w", judge.ID, err)
	}
	return nil
}
