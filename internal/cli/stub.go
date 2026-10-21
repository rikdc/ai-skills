package cli

import (
	"github.com/spf13/cobra"

	"github.com/rikdc/ai-skills/internal/platform/errs"
)

// newStubCommand creates a command owned by a later increment. The command
// parses nothing and fails with exit code 2 until its owner replaces it.
func newStubCommand(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:                use,
		Short:              short,
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return errs.Usage("%s: not implemented in this increment", use)
		},
	}
}
