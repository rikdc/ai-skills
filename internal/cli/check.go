package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/rikdc/ai-skills/internal/check"
	"github.com/rikdc/ai-skills/internal/platform/errs"
)

func newCheckCommand() *cobra.Command {
	var (
		changes string
		asJSON  bool
	)
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run the deterministic authoring gate",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := resolveRoot(cmd)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if changes != "" {
				result, err := check.DetectChanges(root, changes)
				if err != nil {
					return errs.Usage("%s", err)
				}
				printMatches(out, "Changed skills", result.Skills)
				printMatches(out, "Changed scenarios", result.Scenarios)
				return nil
			}

			result, err := check.Run(root)
			if err != nil {
				return err
			}
			if asJSON {
				encoder := json.NewEncoder(out)
				for _, finding := range result.Findings {
					if err := encoder.Encode(finding); err != nil {
						return err
					}
				}
			} else {
				for _, finding := range result.Findings {
					fmt.Fprintf(out, "✗ %s\n", finding.Message)
				}
			}
			if !result.OK() {
				return errs.Operational("check: %d finding(s)", len(result.Findings))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&changes, "changes", "", "list the skills and scenarios a ref touched")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit one JSON object per finding")
	return cmd
}

func printMatches(out io.Writer, label string, matches []string) {
	if len(matches) == 0 {
		fmt.Fprintf(out, "%s: (none)\n", label)
		return
	}
	fmt.Fprintf(out, "%s:\n", label)
	for _, match := range matches {
		fmt.Fprintf(out, "  %s\n", match)
	}
}
