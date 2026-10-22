package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rikdc/ai-skills/internal/model"
)

// Commit is the build commit, overridable with -ldflags.
var Commit = "unknown"

func newVersionCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version, schema, and grader versions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			if asJSON {
				payload := map[string]any{
					"version":        model.Version,
					"commit":         Commit,
					"schema_version": model.SchemaVersion,
					"grader_version": model.GraderVersion,
				}
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}
			fmt.Fprintf(out, "evalctl %s (%s) schema=%d grader=%s\n",
				model.Version, Commit, model.SchemaVersion, model.GraderVersion)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}
