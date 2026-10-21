package cli

import "github.com/spf13/cobra"

func newGradeCommand() *cobra.Command {
	return newStubCommand("grade", "Grade pending criteria")
}
