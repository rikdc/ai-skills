package usage

import (
	"encoding/json"
	"fmt"
	"math"
	"os"

	"github.com/rikdc/ai-skills/internal/model"
)

// ModelPrice is USD per million tokens.
type ModelPrice struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
}

// Pricing maps exact model names to prices. Missing models produce no cost.
type Pricing struct {
	Models map[string]ModelPrice `json:"models"`
}

// LoadPricing reads a pricing JSON file.
func LoadPricing(path string) (Pricing, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Pricing{}, fmt.Errorf("read pricing %s: %w", path, err)
	}
	var p Pricing
	if err := json.Unmarshal(data, &p); err != nil {
		return Pricing{}, fmt.Errorf("parse pricing %s: %w", path, err)
	}
	for name, price := range p.Models {
		if !validPrice(price) {
			return Pricing{}, fmt.Errorf("parse pricing %s: invalid non-negative price for %q", path, name)
		}
	}
	return p, nil
}

func validPrice(p ModelPrice) bool {
	for _, v := range []float64{p.Input, p.Output, p.CacheRead, p.CacheWrite} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return false
		}
	}
	return true
}

// CostFor computes the cost for an exact model match. Claude and opencode logs
// report input tokens excluding cache reads, so all four components are billed
// as disjoint; other sources follow OpenAI semantics, where input includes the
// cache reads billed separately. It reports false when the model is absent,
// the usage is not known, or input or output tokens are missing (missing
// components are never treated as zero).
func (p Pricing) CostFor(modelName string, u model.Usage) (float64, bool) {
	price, ok := p.Models[modelName]
	if !ok {
		return 0, false
	}
	u = u.NormalizeState()
	if u.State != model.UsageKnown || u.InputTokens == nil || u.OutputTokens == nil {
		return 0, false
	}
	input := float64(*u.InputTokens)
	output := float64(*u.OutputTokens)
	cacheRead := 0.0
	if u.CacheReadTokens != nil {
		cacheRead = float64(*u.CacheReadTokens)
	}
	cacheWrite := 0.0
	if u.CacheWriteTokens != nil {
		cacheWrite = float64(*u.CacheWriteTokens)
	}
	billableInput := input
	if !inputExcludesCache(u.Source) {
		billableInput = math.Max(input-cacheRead, 0)
	}
	cost := billableInput/1e6*price.Input +
		cacheRead/1e6*price.CacheRead +
		cacheWrite/1e6*price.CacheWrite +
		output/1e6*price.Output
	return cost, true
}

// inputExcludesCache reports whether source counts cache reads outside input.
func inputExcludesCache(source string) bool {
	return source == SourceClaude || source == SourceOpencode
}

// Apply sets CostUSD when an exact pricing entry exists. Existing costs are
// preserved. Usage state is never upgraded by pricing alone.
func (p Pricing) Apply(modelName string, u model.Usage) model.Usage {
	if u.CostUSD != nil {
		return u
	}
	cost, ok := p.CostFor(modelName, u)
	if !ok {
		return u
	}
	u.CostUSD = &cost
	u.Currency = "USD"
	return u
}
