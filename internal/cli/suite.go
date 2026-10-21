package cli

import "github.com/spf13/cobra"

func newSuiteCommand() *cobra.Command {
	return newStubCommand("suite", "Plan and run a suite")
}
