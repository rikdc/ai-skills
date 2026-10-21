package scenario

import "github.com/rikdc/ai-skills/internal/model"

// PlanOptions carries the execution shape used for invocation counts.
type PlanOptions struct {
	Selection       Selection
	Runs            int
	Arm             string
	Jobs            int
	TimeoutSeconds  int
	JudgeConfigured bool
}

// Plan is the deterministic suite plan derived from selection.
type Plan struct {
	Selected    []Selected
	Unsupported []Selected
	Counts      PlanCounts
	Config      PlanConfig
}

// PlanCounts are the invocation totals in the plan.
type PlanCounts struct {
	Scenarios           int `json:"scenarios"`
	ComparisonScenarios int `json:"comparison_scenarios"`
	FunctionalScenarios int `json:"functional_scenarios"`
	AgentInvocations    int `json:"agent_invocations"`
	JudgeInvocations    int `json:"judge_invocations"`
	Unsupported         int `json:"unsupported"`
}

// PlanConfig is the execution configuration a plan reports.
type PlanConfig struct {
	Arm            string `json:"arm"`
	ArmCount       int    `json:"arm_count"`
	Runs           int    `json:"runs"`
	Jobs           int    `json:"jobs"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// BuildPlan selects scenarios and computes invocation counts. Direct-hook
// scenarios are listed as unsupported (milestone C) and excluded from counts.
func BuildPlan(root string, opts PlanOptions) (*Plan, []string, error) {
	if opts.Runs < 1 {
		return nil, nil, usagef("--runs must be a positive integer")
	}
	if opts.Jobs < 1 {
		return nil, nil, usagef("--jobs must be a positive integer")
	}
	if opts.TimeoutSeconds < 1 {
		return nil, nil, usagef("--timeout must be a positive integer")
	}
	armCount, err := ArmCount(opts.Arm)
	if err != nil {
		return nil, nil, err
	}

	selected, warnings, err := Select(root, opts.Selection)
	if err != nil {
		return nil, nil, err
	}

	plan := &Plan{
		Config: PlanConfig{
			Arm:            opts.Arm,
			ArmCount:       armCount,
			Runs:           opts.Runs,
			Jobs:           opts.Jobs,
			TimeoutSeconds: opts.TimeoutSeconds,
		},
	}
	for _, item := range selected {
		if item.DirectHook() {
			plan.Unsupported = append(plan.Unsupported, item)
			continue
		}
		plan.Selected = append(plan.Selected, item)
		plan.Counts.Scenarios++
		if item.Mode() == model.ModeFunctional {
			plan.Counts.FunctionalScenarios++
			plan.Counts.AgentInvocations += opts.Runs
		} else {
			plan.Counts.ComparisonScenarios++
			plan.Counts.AgentInvocations += armCount * opts.Runs
		}
	}
	plan.Counts.Unsupported = len(plan.Unsupported)
	if opts.JudgeConfigured {
		plan.Counts.JudgeInvocations = plan.Counts.AgentInvocations
	}
	return plan, warnings, nil
}

// ArmCount validates --arm and returns how many arms it runs.
func ArmCount(arm string) (int, error) {
	switch arm {
	case "", "both":
		return 2, nil
	case "with", "without":
		return 1, nil
	default:
		return 0, usagef("--arm must be both, with or without")
	}
}
