package cli

import "github.com/spf13/cobra"

func newCheckCommand() *cobra.Command {
	return newStubCommand("check", "Run the deterministic authoring gate")
}
