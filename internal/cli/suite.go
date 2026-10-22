package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rikdc/ai-skills/internal/executor"
	"github.com/rikdc/ai-skills/internal/platform/errs"
	"github.com/rikdc/ai-skills/internal/provider/command"
	"github.com/rikdc/ai-skills/internal/scenario"
	"github.com/rikdc/ai-skills/internal/suite"
	"github.com/rikdc/ai-skills/internal/usage"
)

func newSuiteCommand() *cobra.Command {
	var (
		plugins          []string
		units            []string
		pattern          string
		changed          string
		shard            string
		arm              string
		runs             int
		jobs             int
		timeout          string
		setupTimeout     string
		checksTimeout    string
		judgeTimeout     string
		skillDir         string
		outDir           string
		resume           bool
		resumePolicy     string
		keepFailures     bool
		rerun            bool
		regrade          bool
		keep             bool
		strict           bool
		planOnly         bool
		authCLI          bool
		keychain         bool
		authPaths        []string
		passEnv          []string
		usageCmd         string
		judgeUsageCmd    string
		backend          string
		agentCmd         string
		modelName        string
		baseURL          string
		pricingFile      string
		maxSnapshotMB    int
		attemptArtifacts string
		judgeCmd         string
		judgeBackend     string
		judgeModel       string
		judgeBaseURL     string
	)

	cmd := &cobra.Command{
		Use:   "suite",
		Short: "Plan, run, resume, and grade a whole eval suite",
		Long: "Select scenarios, run every arm/run through the runner, optionally grade them, " +
			"and write the suite artifacts (manifest, plan, results, suite.json, summary).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := resolveRoot(cmd)
			if err != nil {
				return err
			}
			selection := scenario.Selection{
				Plugins: plugins,
				Units:   units,
				Pattern: pattern,
				Changed: changed,
				Shard:   shard,
			}
			agentDuration, err := parseTimeout("--timeout", timeout)
			if err != nil {
				return err
			}
			setupDuration, err := parseTimeout("--setup-timeout", setupTimeout)
			if err != nil {
				return err
			}
			checksDuration, err := parseTimeout("--checks-timeout", checksTimeout)
			if err != nil {
				return err
			}
			judgeDuration, err := parseTimeout("--judge-timeout", judgeTimeout)
			if err != nil {
				return err
			}

			if planOnly {
				plan, warnings, err := suite.BuildPlan(root, suite.PlanOptions{
					Selection:       selection,
					Runs:            runs,
					Arm:             arm,
					Jobs:            jobs,
					TimeoutSeconds:  int(agentDuration / time.Second),
					JudgeConfigured: strings.TrimSpace(judgeCmd) != "",
				})
				if err != nil {
					return selectionError(err)
				}
				printWarnings(cmd, warnings)
				printSuitePlan(cmd, plan)
				return nil
			}

			switch backend {
			case "", "command":
			case "openai":
				return errs.Usage("--backend openai is milestone B and not implemented")
			default:
				return errs.Usage("--backend must be command or openai")
			}
			if strings.TrimSpace(agentCmd) == "" {
				return errs.Usage("--agent-cmd is required for the command backend, e.g. --agent-cmd 'claude -p'")
			}
			switch judgeBackend {
			case "", "command":
			case "openai":
				return errs.Usage("--judge-backend openai is milestone B and not implemented")
			default:
				return errs.Usage("--judge-backend must be command or openai")
			}
			if judgeModel != "" || judgeBaseURL != "" {
				return errs.Usage("native judge flags (--judge-model/--judge-base-url) are milestone B and not implemented")
			}
			if judgeBackend == "command" && strings.TrimSpace(judgeCmd) == "" {
				return errs.Usage("--judge-backend command requires --judge-cmd")
			}
			if maxSnapshotMB < 1 {
				return errs.Usage("--max-snapshot-mb must be a positive integer")
			}

			flags := cmd.Flags()
			policyFlags := 0
			for _, name := range []string{"resume-policy", "keep-failures", "rerun"} {
				if flags.Changed(name) {
					policyFlags++
				}
			}
			if policyFlags > 1 {
				return errs.Usage("--resume-policy, --keep-failures and --rerun are mutually exclusive")
			}
			effectivePolicy := resumePolicy
			if keepFailures {
				effectivePolicy = suite.PolicyKeepFailures
			}
			if rerun {
				effectivePolicy = suite.PolicyRerunAll
			}
			if r := flags.Changed("resume-policy"); r && strings.TrimSpace(resumePolicy) == "" {
				return errs.Usage("--resume-policy must be retry-failed, keep-failures or rerun-all")
			}
			if resumePolicy != "" && !resume {
				return errs.Usage("--resume-policy requires --resume")
			}
			if keepFailures && !resume {
				return errs.Usage("--keep-failures requires --resume")
			}
			if rerun && !resume {
				return errs.Usage("--rerun requires --resume")
			}
			if regrade && !resume {
				return errs.Usage("--regrade needs --resume and --out")
			}
			if regrade && strings.TrimSpace(outDir) == "" {
				return errs.Usage("--regrade needs --out")
			}
			if regrade && strings.TrimSpace(judgeCmd) == "" {
				return errs.Usage("--regrade needs --judge-cmd")
			}

			var pricing usage.Pricing
			pricingSHA := ""
			if pricingFile != "" {
				pricing, err = usage.LoadPricing(pricingFile)
				if err != nil {
					return errs.Usage("%v", err)
				}
				if data, err := os.ReadFile(pricingFile); err == nil {
					sum := sha256.Sum256(data)
					pricingSHA = hex.EncodeToString(sum[:])
				}
			}

			agentExec, err := command.New(command.Config{
				Command:  agentCmd,
				Timeout:  agentDuration,
				Auth:     command.AuthOptions{CLI: authCLI, Keychain: keychain, Paths: authPaths},
				PassEnv:  passEnv,
				UsageCmd: usageCmd,
				Pricing:  pricing,
				Model:    modelName,
			})
			if err != nil {
				return err
			}
			var judgeExec executor.AgentExecutor
			if strings.TrimSpace(judgeCmd) != "" {
				exec, err := command.New(command.Config{
					Command:  judgeCmd,
					Timeout:  judgeDuration,
					UsageCmd: judgeUsageCmd,
					Pricing:  pricing,
					Model:    modelName,
				})
				if err != nil {
					return err
				}
				judgeExec = exec
			}

			resolvedOut := outDir
			if resolvedOut != "" {
				resolvedOut, err = filepath.Abs(resolvedOut)
				if err != nil {
					return errs.Usage("resolve --out: %v", err)
				}
			}

			runner, err := suite.New(suite.Config{
				Root:             root,
				Selection:        selection,
				Arm:              arm,
				Runs:             runs,
				Jobs:             jobs,
				Out:              resolvedOut,
				Keep:             keep,
				Strict:           strict,
				Resume:           resume,
				ResumePolicy:     effectivePolicy,
				Regrade:          regrade,
				SkillDir:         skillDir,
				Timeout:          agentDuration,
				SetupTimeout:     setupDuration,
				ChecksTimeout:    checksDuration,
				JudgeTimeout:     judgeDuration,
				MaxSnapshotBytes: int64(maxSnapshotMB) << 20,
				AttemptArtifacts: attemptArtifacts,
				PassEnv:          passEnv,
				UsageCmd:         usageCmd,
				JudgeUsageCmd:    judgeUsageCmd,
				PricingSHA256:    pricingSHA,
				AuthCLI:          authCLI,
				Keychain:         keychain,
				AuthPaths:        authPaths,
				AgentExec:        agentExec,
				JudgeExec:        judgeExec,
				JudgeCmd:         judgeCmd,
			})
			if err != nil {
				return selectionError(err)
			}
			for _, warning := range runner.Warnings() {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", warning)
			}
			result, runErr := runner.Run(cmd.Context())
			if result != nil {
				fmt.Fprint(cmd.OutOrStdout(), suite.SummaryText(result.SummaryPath))
			}
			return runErr
		},
	}

	flags := cmd.Flags()
	flags.StringArrayVar(&plugins, "plugin", nil, "only this plugin (repeatable)")
	flags.StringArrayVar(&units, "unit", nil, "only scenarios declaring this unit (repeatable)")
	flags.StringVar(&pattern, "scenario", "", "only scenarios whose plugin/scenario matches this regex")
	flags.StringVar(&changed, "changed", "", "only scenarios a ref touched")
	flags.StringVar(&shard, "shard", "", "deterministic slice I/N (1-based)")
	flags.StringVar(&arm, "arm", "both", "arm: both|with|without")
	flags.IntVar(&runs, "runs", 1, "runs per arm")
	flags.IntVar(&jobs, "jobs", 1, "concurrent scenarios")
	flags.StringVar(&timeout, "timeout", "15m", "per-run agent deadline (duration or seconds)")
	flags.StringVar(&setupTimeout, "setup-timeout", "5m", "setup deadline (duration or seconds)")
	flags.StringVar(&checksTimeout, "checks-timeout", "2m", "checks deadline (duration or seconds)")
	flags.StringVar(&judgeTimeout, "judge-timeout", "10m", "per-run judge deadline (duration or seconds)")
	flags.StringVar(&skillDir, "skill-dir", ".claude/skills", "workspace skills directory")
	flags.StringVar(&outDir, "out", "", "suite directory (default results/eval-suite/<stamp>-<sha>)")
	flags.BoolVar(&resume, "resume", false, "resume an existing suite directory")
	flags.StringVar(&resumePolicy, "resume-policy", "", "resume policy: retry-failed|keep-failures|rerun-all")
	flags.BoolVar(&keepFailures, "keep-failures", false, "retain failed runs without rerunning (requires --resume)")
	flags.BoolVar(&rerun, "rerun", false, "rerun every selected run in a new attempt (requires --resume)")
	flags.BoolVar(&regrade, "regrade", false, "replace judgements with new judge attempts")
	flags.BoolVar(&keep, "keep", false, "keep workspaces")
	flags.BoolVar(&strict, "strict", false, "fail on pending criteria or non-discriminating scenarios")
	flags.BoolVar(&planOnly, "plan", false, "print the plan and exit")
	flags.BoolVar(&authCLI, "auth-cli", false, "copy selected CLI credentials into the workspace home")
	flags.BoolVar(&keychain, "keychain", false, "link the macOS Keychains directory (requires --auth-cli)")
	flags.StringArrayVar(&authPaths, "auth-path", nil, "extra credential path under the real home (requires --auth-cli)")
	flags.StringArrayVar(&passEnv, "pass-env", nil, "environment variable to pass to setup, the agent, and checks")
	flags.StringVar(&usageCmd, "usage-cmd", "", "agent usage normalization hook")
	flags.StringVar(&judgeUsageCmd, "judge-usage-cmd", "", "judge usage normalization hook")
	flags.StringVar(&backend, "backend", "command", "agent runtime: command|openai")
	flags.StringVar(&agentCmd, "agent-cmd", "", "command adapter command")
	flags.StringVar(&modelName, "model", "", "model name for identity and pricing")
	flags.StringVar(&baseURL, "base-url", "", "native endpoint (milestone B; command backend ignores it)")
	flags.StringVar(&pricingFile, "pricing", "", "pricing JSON file")
	flags.IntVar(&maxSnapshotMB, "max-snapshot-mb", 256, "evidence archive limit in MiB")
	flags.StringVar(&attemptArtifacts, "attempt-artifacts", "failed", "attempt evidence retention: failed|all|none")
	flags.StringVar(&judgeCmd, "judge-cmd", "", "command that reads the judge prompt on stdin and writes judgement.json")
	flags.StringVar(&judgeBackend, "judge-backend", "", "judge backend: command|openai")
	flags.StringVar(&judgeModel, "judge-model", "", "native judge model (milestone B)")
	flags.StringVar(&judgeBaseURL, "judge-base-url", "", "native judge base URL (milestone B)")
	return cmd
}

// printSuitePlan renders the same plan text as `evalctl plan`.
func printSuitePlan(cmd *cobra.Command, plan *scenario.Plan) {
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
}
