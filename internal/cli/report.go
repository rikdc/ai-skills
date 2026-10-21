package cli

import "github.com/spf13/cobra"

func newReportCommand() *cobra.Command {
	return newStubCommand("report", "Render a report")
}
