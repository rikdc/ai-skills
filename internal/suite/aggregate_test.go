package suite

import (
	"strings"
	"testing"

	"github.com/rikdc/ai-skills/internal/model"
)

func TestRenderSummaryGradeHintMatchesCredentialMode(t *testing.T) {
	stats := aggregateStats{Counts: map[string]int{"pending_runs": 3}}

	plain := renderSummary(model.SuiteManifest{}, stats)
	if !strings.Contains(plain, "`evalctl grade --suite <suite>`") {
		t.Fatalf("summary lacks the grade hint:\n%s", plain)
	}
	if strings.Contains(plain, "--auth-cli") {
		t.Fatalf("summary suggests --auth-cli without credentials:\n%s", plain)
	}

	credentialed := renderSummary(model.SuiteManifest{Config: model.SuiteConfig{AuthCLI: true}}, stats)
	if !strings.Contains(credentialed, "`evalctl grade --suite <suite> --auth-cli`") {
		t.Fatalf("summary omits --auth-cli for a credentialed suite:\n%s", credentialed)
	}
}
