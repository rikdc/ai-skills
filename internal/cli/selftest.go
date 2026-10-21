package cli

import "github.com/spf13/cobra"

func newSelftestCommand() *cobra.Command {
	return newStubCommand("selftest", "Run the deterministic end-to-end test")
}
