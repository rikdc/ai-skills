package model

import (
	"regexp"
	"strings"
)

var modelFlagPattern = regexp.MustCompile(`(?:--model[ =]|-m )(\S+)`)

// ExtractModel returns the model named by a --model/-m flag in an agent or
// judge command line, or "" when the command names none.
func ExtractModel(command string) string {
	if match := modelFlagPattern.FindStringSubmatch(command); len(match) == 2 {
		return match[1]
	}
	return ""
}

// SameJudgeModel reports whether the judge is the same model as the agent,
// which tends to make scores lenient because a model grades its own style
// kindly. Explicit model names win, then a --model/-m flag in each command.
// When neither side names a model, two identical commands still count as the
// same. The returned name is the shared model, or "" for identical commands
// with no model flag. It never guesses: unknown identities are not "same".
func SameJudgeModel(agentCmd, agentModel, judgeCmd, judgeModel string) (name string, same bool) {
	agent := firstNonBlank(agentModel, ExtractModel(agentCmd))
	judge := firstNonBlank(judgeModel, ExtractModel(judgeCmd))
	if agent != "" && judge != "" {
		return agent, agent == judge
	}
	if agent == "" && judge == "" && strings.TrimSpace(agentCmd) != "" &&
		strings.TrimSpace(agentCmd) == strings.TrimSpace(judgeCmd) {
		return "", true
	}
	return "", false
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// SameModelWarning is the shared wording for the same-model judge caveat.
func SameModelWarning(name string) string {
	subject := "the judge command is identical to the agent command"
	if name != "" {
		subject = "the judge and the agent are both " + name
	}
	return "Same-model judge: " + subject + "; scores may be lenient. Use a different judge model for an independent verdict."
}
