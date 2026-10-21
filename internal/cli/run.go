package cli

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rikdc/ai-skills/internal/platform/errs"
	"github.com/rikdc/ai-skills/internal/provider/command"
	"github.com/rikdc/ai-skills/internal/runner"
	"github.com/rikdc/ai-skills/internal/scenario"
	"github.com/rikdc/ai-skills/internal/usage"
)

func newRunCommand() *cobra.Command {
	var (
		plugin        string
		scenarioName  string
		arm           string
		runs          int
		units         []string
		skillDir      string
		outDir        string
		timeout       string
		setupTimeout  string
		checksTimeout string
		keep          bool

		authCLI   bool
		keychain  bool
		authPaths []string
		passEnv   []string
		usageCmd  string

		backend          string
		agentCmd         string
		modelName        string
		pricingFile      string
		maxSnapshotMB    int
		attemptArtifacts string

		native nativeFlags
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one scenario",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := resolveRoot(cmd)
			if err != nil {
				return err
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
			if err := validateBackend(resolveBackend(cmd, backend)); err != nil {
				return err
			}
			if err := native.nativeErrors(cmd); err != nil {
				return err
			}
			if plugin != "" && scenarioName != "" {
				if sc, err := scenario.Load(root, plugin, scenarioName); err == nil && sc.DirectHook() {
					return errs.Usage("%s: direct-hook execution is milestone C and not implemented", sc.Pair())
				}
			}
			if strings.TrimSpace(agentCmd) == "" {
				return errs.Usage("--agent-cmd is required for the command backend, e.g. --agent-cmd 'claude -p'")
			}
			if maxSnapshotMB < 1 {
				return errs.Usage("--max-snapshot-mb must be a positive integer")
			}
			pricingFile = resolvePricing(cmd, pricingFile)
			modelName = resolveModel(cmd, modelName)
			var pricing usage.Pricing
			if pricingFile != "" {
				pricing, err = usage.LoadPricing(pricingFile)
				if err != nil {
					return errs.Usage("%v", err)
				}
			}
			exec, err := command.New(command.Config{
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

			resolvedOut := outDir
			if resolvedOut == "" {
				resolvedOut = filepath.Join(root, "results", "eval-run")
			} else if !filepath.IsAbs(resolvedOut) {
				resolvedOut, err = filepath.Abs(resolvedOut)
				if err != nil {
					return errs.Usage("resolve --out: %v", err)
				}
			}

			run, err := runner.New(runner.Config{
				Root:             root,
				Plugin:           plugin,
				Scenario:         scenarioName,
				Arm:              arm,
				Runs:             runs,
				Units:            units,
				SkillDir:         skillDir,
				Out:              resolvedOut,
				Timeout:          agentDuration,
				SetupTimeout:     setupDuration,
				ChecksTimeout:    checksDuration,
				Keep:             keep,
				MaxSnapshotBytes: int64(maxSnapshotMB) << 20,
				AttemptArtifacts: attemptArtifacts,
				PassEnv:          passEnv,
				Executor:         exec,
			})
			if err != nil {
				return err
			}
			summary, runErr := run.Run(cmd.Context())
			if summary != nil {
				fmt.Fprint(cmd.OutOrStdout(), runner.SummaryText(summary))
			}
			return runErr
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&plugin, "plugin", "", "plugin under test")
	flags.StringVar(&scenarioName, "scenario", "", "scenario directory under plugins/<plugin>/evals/")
	flags.StringVar(&arm, "arm", "both", "arm: both|with|without")
	flags.IntVar(&runs, "runs", 1, "runs per arm")
	flags.StringArrayVar(&units, "unit", nil, "install only this unit (repeatable)")
	flags.StringVar(&skillDir, "skill-dir", ".claude/skills", "workspace skills directory")
	flags.StringVar(&outDir, "out", "", "results directory (default <root>/results/eval-run)")
	flags.StringVar(&timeout, "timeout", "15m", "per-run agent deadline (duration or seconds)")
	flags.StringVar(&setupTimeout, "setup-timeout", "5m", "setup deadline (duration or seconds)")
	flags.StringVar(&checksTimeout, "checks-timeout", "2m", "checks deadline (duration or seconds)")
	flags.BoolVar(&keep, "keep", false, "keep workspaces")
	flags.BoolVar(&authCLI, "auth-cli", false, "copy selected CLI credentials into the workspace home")
	flags.BoolVar(&keychain, "keychain", false, "link the macOS Keychains directory (requires --auth-cli)")
	flags.StringArrayVar(&authPaths, "auth-path", nil, "extra credential path under the real home (requires --auth-cli)")
	flags.StringArrayVar(&passEnv, "pass-env", nil, "environment variable to pass to setup, the agent, and checks")
	flags.StringVar(&usageCmd, "usage-cmd", "", "usage normalization hook")
	flags.StringVar(&backend, "backend", "command", "agent runtime: command|openai")
	flags.StringVar(&agentCmd, "agent-cmd", "", "command adapter command")
	flags.StringVar(&modelName, "model", "", "model name for identity and pricing (env EVALCTL_MODEL)")
	flags.StringVar(&pricingFile, "pricing", "", "pricing JSON file (env EVALCTL_PRICING)")
	flags.IntVar(&maxSnapshotMB, "max-snapshot-mb", 256, "evidence archive limit in MiB")
	flags.StringVar(&attemptArtifacts, "attempt-artifacts", "failed", "attempt evidence retention: failed|all|none")
	addNativeFlags(flags, &native)
	return cmd
}

// parseTimeout accepts a Go duration ("15m") or an integer number of seconds
// for compatibility with the legacy shell flags.
func parseTimeout(flag, value string) (time.Duration, error) {
	if duration, err := time.ParseDuration(value); err == nil {
		if duration <= 0 {
			return 0, errs.Usage("%s must be positive", flag)
		}
		return duration, nil
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return 0, errs.Usage("%s must be a positive duration or number of seconds", flag)
	}
	return time.Duration(seconds) * time.Second, nil
}
