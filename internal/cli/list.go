package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/rikdc/ai-skills/internal/config"
	"github.com/rikdc/ai-skills/internal/platform/errs"
	"github.com/rikdc/ai-skills/internal/scenario"
)

// listEntry is the JSON shape of one selected scenario.
type listEntry struct {
	Plugin    string   `json:"plugin"`
	Scenario  string   `json:"scenario"`
	Units     []string `json:"units"`
	Mode      string   `json:"mode"`
	Execution string   `json:"execution"`
}

func newListCommand() *cobra.Command {
	var (
		plugins []string
		units   []string
		pattern string
		changed string
		shard   string
		asJSON  bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List selected scenarios",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := resolveRoot(cmd)
			if err != nil {
				return err
			}
			selected, warnings, err := scenario.Select(root, scenario.Selection{
				Plugins: plugins,
				Units:   units,
				Pattern: pattern,
				Changed: changed,
				Shard:   shard,
			})
			if err != nil {
				return selectionError(err)
			}
			printWarnings(cmd, warnings)

			out := cmd.OutOrStdout()
			if asJSON {
				entries := make([]listEntry, 0, len(selected))
				for _, item := range selected {
					declared := item.Declared
					if declared == nil {
						declared = []string{}
					}
					entries = append(entries, listEntry{
						Plugin:    item.Scenario.Plugin,
						Scenario:  item.Scenario.Name,
						Units:     declared,
						Mode:      string(item.Mode()),
						Execution: string(item.Execution()),
					})
				}
				encoder := json.NewEncoder(out)
				encoder.SetIndent("", "  ")
				return encoder.Encode(entries)
			}
			for _, item := range selected {
				fmt.Fprintln(out, item.Pair())
			}
			return nil
		},
	}
	flags := cmd.Flags()
	flags.StringArrayVar(&plugins, "plugin", nil, "only this plugin (repeatable)")
	flags.StringArrayVar(&units, "unit", nil, "only scenarios declaring this unit (repeatable)")
	flags.StringVar(&pattern, "scenario", "", "only scenarios whose plugin/scenario matches this regex")
	flags.StringVar(&changed, "changed", "", "only scenarios a ref touched")
	flags.StringVar(&shard, "shard", "", "deterministic slice I/N (1-based)")
	flags.BoolVar(&asJSON, "json", false, "emit JSON")
	return cmd
}

func resolveRoot(cmd *cobra.Command) (string, error) {
	flags := cmd.Root().PersistentFlags()
	flagRoot, err := flags.GetString("root")
	if err != nil {
		return "", errs.Usage("resolve repository root: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", errs.Usage("get working directory: %v", err)
	}
	root, err := config.ResolveRoot(flagRoot, flags.Changed("root"), cwd, os.Getenv)
	if err != nil {
		return "", errs.Usage("resolve repository root: %v", err)
	}
	return root, nil
}

func selectionError(err error) error {
	if scenario.IsUsageError(err) {
		return errs.Usage("%s", scenario.UsageMessage(err))
	}
	return err
}

func printWarnings(cmd *cobra.Command, warnings []string) {
	for _, warning := range warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", warning)
	}
}
