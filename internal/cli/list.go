package cli

import "github.com/spf13/cobra"

func newListCommand() *cobra.Command {
	return newStubCommand("list", "List selected scenarios")
}
