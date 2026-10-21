package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rikdc/ai-skills/internal/platform/errs"
	"github.com/rikdc/ai-skills/internal/scenario"
)

type planScenario struct {
	Plugin      string   `json:"plugin"`
	Scenario    string   `json:"scenario"`
	Units       []string `json:"units"`
	Mode        string   `json:"mode"`
	Execution   string   `json:"execution"`
	Unsupported bool     `json:"unsupported,omitempty"`
}

type planDocument struct {
	Scenarios   []planScenario      `json:"scenarios"`
	Unsupported []planScenario      `json:"unsupported"`
	Counts      scenario.PlanCounts `json:"counts"`
	Config      scenario.PlanConfig `json:"config"`
}

func newPlanCommand() *cobra.Command {
	var (
		plugins  []string
		units    []string
		pattern  string
		changed  string
		shard    string
		runs     int
		arm      string
		jobs     int
		timeout  int
		judgeCmd string
		format   string
	)
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Print a plan",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			switch format {
			case "text", "json":
			default:
				return errs.Usage("--format must be text or json")
			}
			root, err := resolveRoot(cmd)
			if err != nil {
				return err
			}
			plan, warnings, err := scenario.BuildPlan(root, scenario.PlanOptions{
				Selection: scenario.Selection{
					Plugins: plugins,
					Units:   units,
					Pattern: pattern,
					Changed: changed,
					Shard:   shard,
				},
				Runs:            runs,
				Arm:             arm,
				Jobs:            jobs,
				TimeoutSeconds:  timeout,
				JudgeConfigured: strings.TrimSpace(judgeCmd) != "",
			})
			if err != nil {
				return selectionError(err)
			}
			printWarnings(cmd, warnings)

			if format == "json" {
				document := planDocument{
					Scenarios:   make([]planScenario, 0, len(plan.Selected)),
					Unsupported: make([]planScenario, 0, len(plan.Unsupported)),
				}
				for _, item := range plan.Selected {
					document.Scenarios = append(document.Scenarios, planScenarioOf(item, false))
				}
				for _, item := range plan.Unsupported {
					document.Unsupported = append(document.Unsupported, planScenarioOf(item, true))
				}
				document.Counts = plan.Counts
				document.Config = plan.Config
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(document)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "Eval suite plan")
			fmt.Fprintln(out)
			for _, item := range plan.Selected {
				fmt.Fprintf(out, "  %s\n", item.Pair())
				fmt.Fprintf(out, "      installs: %s\n", installLine(item))
			}
			for _, item := range plan.Unsupported {
				fmt.Fprintf(out, "  %s\n", item.Pair())
				fmt.Fprintln(out, "      unsupported: direct-hook execution is milestone C")
			}
			fmt.Fprintln(out)
			fmt.Fprintf(out, "Scenarios: %d\n", plan.Counts.Scenarios)
			fmt.Fprintf(out, "Arms: %s (%d)  ·  runs per arm: %d\n", plan.Config.Arm, plan.Config.ArmCount, plan.Config.Runs)
			if plan.Counts.FunctionalScenarios > 0 {
				fmt.Fprintf(out, "Functional scenarios: %d (with arm only, no delta)\n", plan.Counts.FunctionalScenarios)
			}
			fmt.Fprintf(out, "Agent invocations: %d\n", plan.Counts.AgentInvocations)
			if plan.Counts.JudgeInvocations > 0 {
				fmt.Fprintf(out, "Judge invocations: up to %d\n", plan.Counts.JudgeInvocations)
			}
			if plan.Counts.Unsupported > 0 {
				fmt.Fprintf(out, "Unsupported (milestone C): %d\n", plan.Counts.Unsupported)
			}
			fmt.Fprintf(out, "Jobs: %d  ·  per-run timeout: %ds\n", plan.Config.Jobs, plan.Config.TimeoutSeconds)
			if plan.Config.Jobs == 1 && plan.Counts.Scenarios > 1 {
				fmt.Fprintf(out, "Tip: --jobs N runs N scenarios concurrently (this plan has %d);\n", plan.Counts.Scenarios)
				fmt.Fprintln(out, "     each scenario still runs its arms and runs sequentially.")
			}
			return nil
		},
	}
	flags := cmd.Flags()
	flags.StringArrayVar(&plugins, "plugin", nil, "only this plugin (repeatable)")
	flags.StringArrayVar(&units, "unit", nil, "only scenarios declaring this unit (repeatable)")
	flags.StringVar(&pattern, "scenario", "", "only scenarios whose plugin/scenario matches this regex")
	flags.StringVar(&changed, "changed", "", "only scenarios a ref touched")
	flags.StringVar(&shard, "shard", "", "deterministic slice I/N (1-based)")
	flags.IntVar(&runs, "runs", 1, "runs per arm")
	flags.StringVar(&arm, "arm", "both", "arm: both|with|without")
	flags.IntVar(&jobs, "jobs", 1, "concurrent scenarios")
	flags.IntVar(&timeout, "timeout", 900, "per-run timeout in seconds")
	flags.StringVar(&judgeCmd, "judge-cmd", "", "judge command (counts judge invocations)")
	flags.StringVar(&format, "format", "text", "output format: text|json")
	return cmd
}

func planScenarioOf(item scenario.Selected, unsupported bool) planScenario {
	return planScenario{
		Plugin:      item.Scenario.Plugin,
		Scenario:    item.Scenario.Name,
		Units:       item.Declared,
		Mode:        string(item.Mode()),
		Execution:   string(item.Execution()),
		Unsupported: unsupported,
	}
}

func installLine(item scenario.Selected) string {
	if len(item.Units) > 0 {
		return strings.Join(item.Units, " ")
	}
	declared := strings.Join(item.Declared, " ")
	if declared == "" {
		declared = "undeclared"
	}
	return "all units (" + declared + ")"
}
