// Package cli builds the evalctl command tree.
package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/rikdc/ai-skills/internal/config"
	"github.com/rikdc/ai-skills/internal/platform/errs"
	"github.com/rikdc/ai-skills/internal/platform/logging"
)

// GlobalOptions holds the resolved persistent flags.
type GlobalOptions struct {
	Root      string
	RootErr   error
	Verbose   bool
	Quiet     bool
	LogFormat string
	Debug     bool
	Logger    *slog.Logger
}

// loggerOf returns the global logger, or nil when no options were supplied.
func loggerOf(opts *GlobalOptions) *slog.Logger {
	if opts == nil {
		return nil
	}
	return opts.Logger
}

// NewRootCommand builds the full command tree with all A1 subcommands.
func NewRootCommand() *cobra.Command {
	opts := &GlobalOptions{}
	var (
		flagRoot      string
		flagVerbose   bool
		flagQuiet     bool
		flagLogFormat string
		flagNoColor   bool // read directly by reportColor
		flagDebug     bool
	)

	root := &cobra.Command{
		Use:           "evalctl",
		Short:         "Run and report AI skill eval scenarios",
		SilenceErrors: true,
		SilenceUsage:  true,
		// Runnable so cobra validates root args through rootArgs instead of
		// its untyped unknown-command error.
		Args:                       rootArgs,
		SuggestionsMinimumDistance: 2, // cobra's default, which SuggestionsFor does not apply
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			flags := cmd.Flags()
			// An unreadable cwd only matters when discovery needs it; ResolveRoot
			// then reports it through RootErr like any other missing root.
			cwd, _ := os.Getwd()
			opts.Root, opts.RootErr = config.ResolveRoot(flagRoot, flags.Changed("root"), cwd, os.Getenv)
			opts.Verbose = flagVerbose
			opts.Quiet = flagQuiet
			opts.LogFormat = config.ResolveString(flagLogFormat, flags.Changed("log-format"), "text", os.Getenv, "EVALCTL_LOG_FORMAT")
			opts.Debug = config.ResolveBool(flagDebug, flags.Changed("debug"), false, os.Getenv, "EVALCTL_DEBUG")
			var err error
			opts.Logger, err = logging.New(os.Stderr, logging.Options{
				Format:    opts.LogFormat,
				Level:     logging.Level(opts.Verbose, opts.Quiet, opts.Debug),
				AddSource: opts.Debug,
			})
			if err != nil {
				return errs.Usage("%v", err)
			}
			return nil
		},
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return errs.Usage("%v", err)
	})

	pf := root.PersistentFlags()
	pf.StringVar(&flagRoot, "root", "", "repository root (env EVALCTL_ROOT)")
	pf.BoolVarP(&flagVerbose, "verbose", "v", false, "verbose logging")
	pf.BoolVarP(&flagQuiet, "quiet", "q", false, "suppress non-error logging")
	pf.StringVar(&flagLogFormat, "log-format", "text", "log format: text|json")
	pf.BoolVar(&flagNoColor, "no-color", false, "disable color output")
	pf.BoolVar(&flagDebug, "debug", false, "debug logging and diagnostics")

	root.AddCommand(
		newRunCommand(opts),
		newSuiteCommand(opts),
		newGradeCommand(),
		newReportCommand(),
		newListCommand(),
		newPlanCommand(),
		newCheckCommand(),
		newSelftestCommand(),
		newDoctorCommand(opts),
		newVersionCommand(),
	)
	// cobra adds these lazily; add them now so their validators are wrapped.
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	wrapArgsAsUsage(root)
	return root
}

// Execute runs the command tree with args. Flag and positional-argument
// errors carry exit code 2.
func Execute(ctx context.Context, args []string) error {
	root := NewRootCommand()
	root.SetArgs(args)
	root.SetContext(ctx)
	return root.Execute()
}

// rootArgs rejects positional args on the root command, which only reach it
// when they name no subcommand.
func rootArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	msg := fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath())
	if suggestions := cmd.SuggestionsFor(args[0]); len(suggestions) > 0 {
		msg += "\n\nDid you mean this?"
		for _, s := range suggestions {
			msg += "\n\t" + s
		}
	}
	return errs.Usage("%s", msg)
}

// wrapArgsAsUsage makes every positional-argument validator in the tree
// return a usage error.
func wrapArgsAsUsage(cmd *cobra.Command) {
	if validate := cmd.Args; validate != nil {
		cmd.Args = func(c *cobra.Command, args []string) error {
			err := validate(c, args)
			var exitErr *errs.ExitError
			if err == nil || errors.As(err, &exitErr) {
				return err
			}
			return errs.WithCode(errs.CodeUsage, err)
		}
	}
	for _, sub := range cmd.Commands() {
		wrapArgsAsUsage(sub)
	}
}
