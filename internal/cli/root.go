// Package cli builds the evalctl command tree.
package cli

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"

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
	NoColor   bool
	Debug     bool
	Logger    *slog.Logger
}

// RequireRoot returns the discovered repository root or a usage error.
func (o *GlobalOptions) RequireRoot() (string, error) {
	if o.RootErr != nil {
		return "", errs.Usage("resolve repository root: %v", o.RootErr)
	}
	return o.Root, nil
}

// NewRootCommand builds the full command tree with all A1 subcommands.
func NewRootCommand() *cobra.Command {
	opts := &GlobalOptions{}
	var (
		flagRoot      string
		flagVerbose   bool
		flagQuiet     bool
		flagLogFormat string
		flagNoColor   bool
		flagDebug     bool
	)

	root := &cobra.Command{
		Use:           "evalctl",
		Short:         "Run and report AI skill eval scenarios",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			flags := cmd.Flags()
			cwd, err := os.Getwd()
			if err != nil {
				return errs.Usage("get working directory: %v", err)
			}
			opts.Root, opts.RootErr = config.ResolveRoot(flagRoot, flags.Changed("root"), cwd, os.Getenv)
			opts.Verbose = flagVerbose
			opts.Quiet = flagQuiet
			opts.LogFormat = config.ResolveString(flagLogFormat, flags.Changed("log-format"), "text", os.Getenv, "EVALCTL_LOG_FORMAT")
			if opts.LogFormat != "text" && opts.LogFormat != "json" {
				return errs.Usage("unsupported --log-format %q (want text or json)", opts.LogFormat)
			}
			opts.NoColor = config.ResolveBool(flagNoColor, flags.Changed("no-color"), false, os.Getenv, "NO_COLOR")
			opts.Debug = config.ResolveBool(flagDebug, flags.Changed("debug"), false, os.Getenv, "EVALCTL_DEBUG")
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
		newRunCommand(),
		newSuiteCommand(),
		newGradeCommand(),
		newReportCommand(),
		newListCommand(),
		newPlanCommand(),
		newCheckCommand(),
		newSelftestCommand(),
		newDoctorCommand(opts),
		newVersionCommand(),
	)
	return root
}

// Execute runs the command tree with args and maps otherwise unclassified
// command-line errors to exit code 2.
func Execute(ctx context.Context, args []string) error {
	root := NewRootCommand()
	root.SetArgs(args)
	root.SetContext(ctx)
	err := root.Execute()
	if err == nil {
		return nil
	}
	var exitErr *errs.ExitError
	if errors.As(err, &exitErr) {
		return err
	}
	if isUsageError(err) {
		return errs.WithCode(errs.CodeUsage, err)
	}
	return err
}

func isUsageError(err error) bool {
	msg := err.Error()
	return strings.HasPrefix(msg, "unknown command") ||
		strings.HasPrefix(msg, "unknown flag") ||
		strings.HasPrefix(msg, "flag needs an argument")
}
