package skilluse

import (
	"os"
	"strings"
	"testing"

	"github.com/rikdc/ai-skills/internal/model"
)

const (
	opencodeStart = `{"type":"step_start","part":{"type":"step-start"}}` + "\n"
	opencodeText  = `{"type":"text","part":{"type":"text","text":"hello"}}` + "\n"
	opencodeSkill = `{"type":"tool_use","part":{"type":"tool","tool":"skill","state":{"status":"completed","input":{"name":"nix-expert"},"output":"x"}}}` + "\n"
	opencodeRead  = `{"type":"tool_use","part":{"type":"tool","tool":"read","state":{"input":{"filePath":"/ws/.claude/skills/nix-expert/references/Deploy.md"}}}}` + "\n"
	opencodeBash  = `{"type":"tool_use","part":{"type":"tool","tool":"bash","state":{"input":{"command":"git status"}}}}` + "\n"
)

func detect(t *testing.T, log string, skills ...string) model.SkillUse {
	t.Helper()
	return Detect(strings.NewReader(log), skills, "")
}

func TestOpencodeSkillToolCallIsInvoked(t *testing.T) {
	got := detect(t, opencodeStart+opencodeSkill+opencodeText, "nix-expert")
	if got.State != model.SkillInvoked || got.Skills["nix-expert"] != model.SkillInvoked {
		t.Fatalf("got %+v", got)
	}
}

func TestOpencodeReadOfSkillFileIsInvoked(t *testing.T) {
	got := detect(t, opencodeStart+opencodeRead+opencodeText, "nix-expert")
	if got.State != model.SkillInvoked {
		t.Fatalf("got %+v", got)
	}
}

func TestOpencodeLogWithoutSkillUseIsNotInvoked(t *testing.T) {
	got := detect(t, opencodeStart+opencodeBash+opencodeText, "nix-expert")
	if got.State != model.SkillNotInvoked || got.Skills["nix-expert"] != model.SkillNotInvoked {
		t.Fatalf("got %+v", got)
	}
}

func TestOtherSkillNameDoesNotCount(t *testing.T) {
	log := strings.ReplaceAll(opencodeSkill, "nix-expert", "nix-expert-extra") +
		strings.ReplaceAll(opencodeRead, "nix-expert", "nix-experts")
	got := detect(t, opencodeStart+log, "nix-expert")
	if got.State != model.SkillNotInvoked {
		t.Fatalf("a different skill must not match: %+v", got)
	}
}

func TestMentionInAssistantTextIsNotInvocation(t *testing.T) {
	text := `{"type":"text","part":{"type":"text","text":"I could read .claude/skills/nix-expert/SKILL.md"}}` + "\n"
	got := detect(t, opencodeStart+text, "nix-expert")
	if got.State != model.SkillNotInvoked {
		t.Fatalf("prose is not a tool call: %+v", got)
	}
}

func TestClaudeStreamJSONSkillTool(t *testing.T) {
	log := `{"type":"system","subtype":"init"}` + "\n" +
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Skill","input":{"skill":"nix-expert"}}]}}` + "\n"
	got := detect(t, log, "nix-expert")
	if got.State != model.SkillInvoked {
		t.Fatalf("got %+v", got)
	}
	none := `{"type":"system","subtype":"init"}` + "\n" +
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}` + "\n"
	if got := detect(t, none, "nix-expert"); got.State != model.SkillNotInvoked {
		t.Fatalf("got %+v", got)
	}
}

func TestUnrecognisedOrEmptyLogsAreUnknown(t *testing.T) {
	for name, log := range map[string]string{
		"empty":       "",
		"plain text":  "I read the skill and did the thing\n",
		"foreign":     `{"event":"x","data":1}` + "\n",
		"truncated":   `{"type":"tool_use","part":{"tool":"ski`,
		"blank lines": "\n\n",
	} {
		got := detect(t, log, "nix-expert")
		if got.State != model.SkillUnknown || got.Skills["nix-expert"] != model.SkillUnknown {
			t.Errorf("%s: got %+v, want unknown", name, got)
		}
	}
}

func TestInvokedWinsEvenInUnrecognisedLog(t *testing.T) {
	got := detect(t, "noise\n"+opencodeSkill, "nix-expert")
	if got.State != model.SkillInvoked {
		t.Fatalf("got %+v", got)
	}
}

func TestMultipleSkillsAggregate(t *testing.T) {
	got := detect(t, opencodeStart+opencodeSkill, "nix-expert", "other")
	if got.State != model.SkillInvoked ||
		got.Skills["nix-expert"] != model.SkillInvoked || got.Skills["other"] != model.SkillNotInvoked {
		t.Fatalf("got %+v", got)
	}
}

func TestCustomSkillDir(t *testing.T) {
	read := strings.ReplaceAll(opencodeRead, ".claude/skills", ".agents/skills")
	if got := Detect(strings.NewReader(opencodeStart+read), []string{"nix-expert"}, ".agents/skills"); got.State != model.SkillInvoked {
		t.Fatalf("got %+v", got)
	}
	if got := Detect(strings.NewReader(opencodeStart+read), []string{"nix-expert"}, ".claude/skills"); got.State != model.SkillNotInvoked {
		t.Fatalf("got %+v", got)
	}
}

func TestHugeLinesAreHandled(t *testing.T) {
	big := `{"type":"text","part":{"text":"` + strings.Repeat("x", 3<<20) + `"}}` + "\n"
	got := detect(t, opencodeStart+big+opencodeSkill, "nix-expert")
	if got.State != model.SkillInvoked {
		t.Fatalf("got %+v", got)
	}
}

func TestSkillNamesFromUnits(t *testing.T) {
	got := SkillNames([]string{"skills/a", "commands/b", "hooks/c", "skills/d", "skills/a"})
	if strings.Join(got, ",") != "a,d" {
		t.Fatalf("got %v", got)
	}
	if SkillNames(nil) != nil {
		t.Fatal("no units must yield no skills")
	}
}

func TestNoSkillsMeansNoResult(t *testing.T) {
	if got := Detect(strings.NewReader(opencodeSkill), nil, ""); got.State != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestDetectPath(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/agent.log"
	if err := os.WriteFile(path, []byte(opencodeStart+opencodeSkill), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := DetectPath(path, []string{"nix-expert"}, ""); got.State != model.SkillInvoked {
		t.Fatalf("got %+v", got)
	}
	if got := DetectPath(dir+"/missing.log", []string{"nix-expert"}, ""); got.State != model.SkillUnknown {
		t.Fatalf("a missing log must be unknown, got %+v", got)
	}
}
