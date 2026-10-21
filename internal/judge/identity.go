// Package judge implements the v3 command judge: prompt construction, strict
// judgement validation, canonical judge identity, evidence binding, grading,
// and regrade.
package judge

import (
	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/runner/attempts"
)

// CommandIdentity returns the canonical identity of a command judge. Native
// fields are empty and the generation block is zero-valued so the hashed
// shape is stable across backends.
func CommandIdentity(command string) model.JudgeIdentity {
	return model.JudgeIdentity{
		Kind:    "command",
		Command: command,
		Generation: model.GenerationIdentity{
			Temperature: "0.0",
		},
		PromptVersion: PromptVersion(),
		GraderVersion: model.GraderVersion,
		SchemaVersion: model.SchemaVersion,
	}
}

// IdentitySHA256 returns the lowercase hex SHA-256 of the canonical JSON
// encoding of identity. The hash is not part of the hashed object.
func IdentitySHA256(identity model.JudgeIdentity) (string, error) {
	data, err := attempts.CanonicalJSON(identity)
	if err != nil {
		return "", err
	}
	return attempts.SHA256Hex(data), nil
}
