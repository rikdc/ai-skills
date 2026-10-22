package cli

import "github.com/spf13/cobra"

func newPlanCommand() *cobra.Command {
	return newStubCommand("plan", "Print a plan")
}
