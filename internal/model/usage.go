package model

import "encoding/json"

// UsageState is the three-state accounting contract. Unknown is never zero.
type UsageState string

const (
	UsageNotApplicable UsageState = "not_applicable"
	UsageUnknown       UsageState = "unknown"
	UsageKnown         UsageState = "known"
)

// Usage records token and cost usage for one invocation. Missing numeric
// components are omitted, never written as zero. State is authoritative; Known
// is a compatibility flag derived from it, so set state with WithState.
type Usage struct {
	State            UsageState `json:"state"`
	Known            bool       `json:"known"`
	Source           string     `json:"source"`
	Reason           string     `json:"reason,omitempty"`
	InputTokens      *int64     `json:"input_tokens,omitempty"`
	OutputTokens     *int64     `json:"output_tokens,omitempty"`
	ReasoningTokens  *int64     `json:"reasoning_tokens,omitempty"`
	CacheReadTokens  *int64     `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens *int64     `json:"cache_write_tokens,omitempty"`
	TotalTokens      *int64     `json:"total_tokens,omitempty"`
	CostUSD          *float64   `json:"cost_usd,omitempty"`
	Currency         string     `json:"currency,omitempty"`
}

// Unknown returns an applicable-but-unmeasured usage record. It is never zero.
func Unknown(source, reason string) Usage {
	return Usage{State: UsageUnknown, Known: false, Source: source, Reason: reason}
}

// NotApplicable returns the usage for an invocation that never happened.
func NotApplicable(source string) Usage {
	return Usage{State: UsageNotApplicable, Known: false, Source: source}
}

// KnownUsage returns a known-state usage record with no numeric components yet.
func KnownUsage(source string) Usage {
	return Usage{State: UsageKnown, Known: true, Source: source}
}

// IsApplicable reports whether the invocation happened.
func (u Usage) IsApplicable() bool { return u.State != UsageNotApplicable }

// IsUnknown reports whether the invocation happened without usable usage.
func (u Usage) IsUnknown() bool { return u.State == UsageUnknown }

// WithState returns u with State set to state and Known derived from it.
func (u Usage) WithState(state UsageState) Usage {
	u.State = state
	u.Known = state == UsageKnown
	return u
}

// MarshalJSON writes Known derived from State, so a serialised record never
// disagrees with itself. A legacy record with no State keeps its Known flag.
func (u Usage) MarshalJSON() ([]byte, error) {
	type wire Usage
	if u.State != "" {
		u.Known = u.State == UsageKnown
	}
	return json.Marshal(wire(u))
}

// NormalizeState fills State from the compatibility Known flag when the state
// is empty. It never invents numerics.
func (u Usage) NormalizeState() Usage {
	if u.State == "" {
		if u.Known {
			u.State = UsageKnown
		} else {
			u.State = UsageUnknown
		}
	}
	u.Known = u.State == UsageKnown
	return u
}

// Combine merges usage from two invocations according to the v3 contract:
//   - both not_applicable -> not_applicable
//   - one not_applicable  -> the other, unchanged
//   - both known          -> known, numeric components summed
//   - any applicable unknown -> unknown, retaining partial numeric components
//
// A known result carries only components both sides report, and a summed cost
// needs both costs: a one-sided total would look exact while omitting the
// other invocation. An unknown result retains one-sided token components as a
// partial lower bound. Unknown is never treated as zero.
func Combine(a, b Usage) Usage {
	a = a.NormalizeState()
	b = b.NormalizeState()

	if a.State == UsageNotApplicable {
		return b
	}
	if b.State == UsageNotApplicable {
		return a
	}

	known := a.State == UsageKnown && b.State == UsageKnown
	sum := sumInt64
	if known {
		sum = sumBothInt64
	}
	out := Usage{
		State:            UsageKnown,
		Source:           combineSource(a.Source, b.Source),
		InputTokens:      sum(a.InputTokens, b.InputTokens),
		OutputTokens:     sum(a.OutputTokens, b.OutputTokens),
		ReasoningTokens:  sum(a.ReasoningTokens, b.ReasoningTokens),
		CacheReadTokens:  sum(a.CacheReadTokens, b.CacheReadTokens),
		CacheWriteTokens: sum(a.CacheWriteTokens, b.CacheWriteTokens),
		TotalTokens:      sum(a.TotalTokens, b.TotalTokens),
	}
	if !known {
		out.State = UsageUnknown
		out.Reason = combineReason(a.Reason, b.Reason)
	}
	out.Known = out.State == UsageKnown
	out.CostUSD = sumFloat64(a.CostUSD, b.CostUSD)
	if out.CostUSD != nil {
		out.Currency = combineCurrency(a.Currency, b.Currency)
	}
	return out
}

func combineSource(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "", a == b:
		return a
	default:
		return a + "+" + b
	}
}

func combineReason(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "", a == b:
		return a
	default:
		return a + "; " + b
	}
}

func combineCurrency(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "", a == b:
		return a
	default:
		return ""
	}
}

func sumInt64(a, b *int64) *int64 {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	v := *a + *b
	return &v
}

func sumBothInt64(a, b *int64) *int64 {
	if a == nil || b == nil {
		return nil
	}
	v := *a + *b
	return &v
}

func sumFloat64(a, b *float64) *float64 {
	if a == nil || b == nil {
		return nil
	}
	v := *a + *b
	return &v
}
