package cli

import "github.com/spf13/cobra"

func newRunCommand() *cobra.Command {
	return newStubCommand("run", "Run one scenario")
}
