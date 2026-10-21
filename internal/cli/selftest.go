package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/rikdc/ai-skills/internal/platform/errs"
	"github.com/rikdc/ai-skills/internal/selftest"
)

func newSelftestCommand() *cobra.Command {
	var (
		quick bool
		keep  bool
		jobs  int
	)
	cmd := &cobra.Command{
		Use:   "selftest",
		Short: "Run the deterministic end-to-end test",
		Long: "Run run, suite, grade, report, resume, and regrade against a synthetic " +
			"repository with the command backend and a command judge. Unless --quick is " +
			"used, every repository fixture setup script is also exercised in a temporary " +
			"workspace. No model, network, or credentials are used.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if jobs < 1 {
				return errs.Usage("--jobs must be a positive integer")
			}
			root := ""
			if !quick {
				resolved, err := resolveRoot(cmd)
				if err != nil {
					return err
				}
				root = resolved
			}
			result, err := selftest.Run(cmd.Context(), selftest.Options{
				Root:  root,
				Quick: quick,
				Keep:  keep,
				Jobs:  jobs,
				Out:   cmd.OutOrStdout(),
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "selftest passed: %d checks, %d fixtures in %s\n",
				result.Checks, result.Fixtures, result.Duration.Round(time.Millisecond))
			if keep {
				fmt.Fprintf(cmd.OutOrStdout(), "selftest directory kept: %s\n", result.BaseDir)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&quick, "quick", false, "skip repository fixture coverage")
	cmd.Flags().BoolVar(&keep, "keep", false, "keep the temporary selftest directory")
	cmd.Flags().IntVar(&jobs, "jobs", 1, "concurrent fixture scenarios")
	return cmd
}
