package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rikdc/ai-skills/internal/judge"
	"github.com/rikdc/ai-skills/internal/platform/errs"
	"github.com/rikdc/ai-skills/internal/provider/command"
	suitepkg "github.com/rikdc/ai-skills/internal/suite"
	"github.com/rikdc/ai-skills/internal/usage"
)

func newGradeCommand() *cobra.Command {
	var (
		runDir   string
		suiteDir string

		judgeCmd     string
		judgeBackend string
		judgeModel   string
		judgeBaseURL string
		judgeTimeout string
		judgeUsage   string
		usageCmd     string

		authCLI   bool
		keychain  bool
		authPaths []string
		passEnv   []string

		regrade     bool
		pricingFile string

		native nativeFlags
	)
	cmd := &cobra.Command{
		Use:   "grade",
		Short: "Grade pending criteria",
		Long: "Grade the criteria a run left pending with a command judge. A judgement is " +
			"reused only when its canonical identity and evidence binding still match; " +
			"use --regrade to create a new immutable judge attempt.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if runDir != "" && suiteDir != "" {
				return errs.Usage("pass either --run-dir or --suite, not both")
			}
			if runDir == "" && suiteDir == "" {
				return errs.Usage("pass --run-dir or --suite")
			}
			switch judgeBackend {
			case "", "command":
			case "openai":
				return errs.Usage("--judge-backend openai is milestone B and not implemented")
			default:
				return errs.Usage("--judge-backend must be command or openai")
			}
			if err := native.nativeErrors(cmd); err != nil {
				return err
			}
			pricingFile = resolvePricing(cmd, pricingFile)
			if judgeModel != "" || judgeBaseURL != "" {
				return errs.Usage("native judge flags (--judge-model/--judge-base-url) are milestone B and not implemented")
			}
			if strings.TrimSpace(judgeCmd) == "" {
				return errs.Usage("--judge-cmd is required for the command judge")
			}
			timeout, err := parseTimeout("--judge-timeout", judgeTimeout)
			if err != nil {
				return err
			}
			usageCommand := judgeUsage
			if usageCommand == "" {
				usageCommand = usageCmd
			}
			var pricing usage.Pricing
			if pricingFile != "" {
				pricing, err = usage.LoadPricing(pricingFile)
				if err != nil {
					return errs.Usage("%v", err)
				}
			}
			creds := executorCredentials{
				auth:    command.AuthOptions{CLI: authCLI, Keychain: keychain, Paths: authPaths},
				passEnv: passEnv,
			}
			exec, err := creds.newExecutor(command.Config{
				Command:  judgeCmd,
				Timeout:  timeout,
				UsageCmd: usageCommand,
				Pricing:  pricing,
			})
			if err != nil {
				return err
			}
			cfg := judge.Config{
				Command:  judgeCmd,
				Timeout:  timeout,
				Regrade:  regrade,
				Executor: exec,
			}
			if runDir != "" {
				resolved, err := absoluteDir(runDir)
				if err != nil {
					return err
				}
				cfg.RunDir = resolved
				agentCmd, agentModel := suiteAgent(resolved)
				warnSameModelJudge(cmd, agentCmd, agentModel, judgeCmd, "")
				result, err := judge.Grade(cmd.Context(), cfg)
				refreshSuite(cmd, suiteRootOf(resolved))
				if err != nil {
					return err
				}
				printGradeResult(cmd, result)
				return nil
			}

			resolved, err := absoluteDir(suiteDir)
			if err != nil {
				return err
			}
			cfg.SuiteDir = resolved
			agentCmd, agentModel := suiteAgent(resolved)
			warnSameModelJudge(cmd, agentCmd, agentModel, judgeCmd, "")
			suite, err := judge.GradeSuite(cmd.Context(), cfg)
			refreshSuite(cmd, resolved)
			for _, run := range suite.Runs {
				if run.Err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "  failed: %s: %v\n", run.Result.RunDir, run.Err)
					continue
				}
				printGradeResult(cmd, run.Result)
			}
			fmt.Fprintln(cmd.OutOrStdout(), gradeSummaryLine(suite))
			if err != nil {
				return err
			}
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&runDir, "run-dir", "", "one run directory containing grading-request.json")
	flags.StringVar(&suiteDir, "suite", "", "a suite directory produced by evalctl suite")
	flags.StringVar(&judgeCmd, "judge-cmd", "", "command that reads the prompt on stdin and writes judgement.json")
	flags.StringVar(&judgeBackend, "judge-backend", "command", "judge backend: command|openai")
	flags.StringVar(&judgeModel, "judge-model", "", "native judge model (milestone B)")
	flags.StringVar(&judgeBaseURL, "judge-base-url", "", "native judge base URL (milestone B)")
	flags.StringVar(&judgeTimeout, "judge-timeout", "10m", "per-run judge deadline (duration or seconds)")
	flags.StringVar(&judgeUsage, "judge-usage-cmd", "", "judge usage normalization hook")
	flags.StringVar(&usageCmd, "usage-cmd", "", "alias for --judge-usage-cmd")
	flags.BoolVar(&authCLI, "auth-cli", false, "copy selected CLI credentials into the judge home")
	flags.BoolVar(&keychain, "keychain", false, "link the macOS Keychains directory (requires --auth-cli)")
	flags.StringArrayVar(&authPaths, "auth-path", nil, "extra credential path under the real home (requires --auth-cli)")
	flags.StringArrayVar(&passEnv, "pass-env", nil, "environment variable to pass to the judge (overrides the credential filter)")
	flags.BoolVar(&regrade, "regrade", false, "create a new judge attempt for existing judgements")
	flags.StringVar(&pricingFile, "pricing", "", "pricing JSON file (env EVALCTL_PRICING)")
	addNativeFlags(flags, &native)
	return cmd
}

// refreshSuite rewrites the suite's summary and counts after grading added
// judge attempts. It is best effort: grading already succeeded or failed on
// its own terms, so a refresh problem is reported but never changes the exit
// status. A directory that is not a suite has nothing to refresh.
func refreshSuite(cmd *cobra.Command, suiteDir string) {
	if suiteDir == "" {
		return
	}
	if err := suitepkg.Refresh(discoverRoot(cmd), suiteDir); err != nil && !suitepkg.IsNotSuite(err) {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not refresh summary.md and suite.json: %v\n", err)
	}
}

func absoluteDir(dir string) (string, error) {
	resolved, err := filepath.Abs(dir)
	if err != nil {
		return "", errs.Usage("resolve path %s: %v", dir, err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errs.Usage("no such directory: %s", dir)
	}
	return resolved, nil
}

func printGradeResult(cmd *cobra.Command, result judge.Result) {
	format := func(value float64) string {
		return strconv.FormatFloat(value, 'f', -1, 64)
	}
	switch result.Outcome {
	case judge.OutcomeNothingPending:
		fmt.Fprintf(cmd.ErrOrStderr(), "  nothing pending: %s\n", result.RunDir)
	case judge.OutcomeAlreadyComplete:
		fmt.Fprintf(cmd.ErrOrStderr(), "  already graded (%s/%s): %s\n",
			format(result.Score), format(result.Max), result.RunDir)
	case judge.OutcomeGraded:
		fmt.Fprintf(cmd.ErrOrStderr(), "  graded (%s/%s): %s\n",
			format(result.Score), format(result.Max), result.RunDir)
	case judge.OutcomeSuperseded:
		fmt.Fprintf(cmd.ErrOrStderr(), "  superseded by a failed rerun, not graded: %s\n", result.RunDir)
	}
}

// gradeSummaryLine renders the grade --suite totals. The superseded count
// appears only when non-zero, so the line matches the shell harness otherwise.
func gradeSummaryLine(suite judge.SuiteResult) string {
	superseded := ""
	if suite.Superseded > 0 {
		superseded = fmt.Sprintf(", superseded: %d", suite.Superseded)
	}
	return fmt.Sprintf("Grading: %d, graded: %d, already complete: %d%s, failed: %d",
		suite.Total, suite.Graded, suite.AlreadyComplete, superseded, suite.Failed)
}
