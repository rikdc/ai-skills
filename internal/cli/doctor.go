package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rikdc/ai-skills/internal/platform/errs"
)

func newDoctorCommand(opts *GlobalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Report external tool and environment readiness",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()

			bashOK := reportTool(ctx, out, "bash", true, "--version")
			reportTool(ctx, out, "git", false, "--version")
			reportTool(ctx, out, "jq", false, "--version")

			if opts.RootErr != nil {
				fmt.Fprintf(out, "root: not found (%v)\n", opts.RootErr)
			} else {
				fmt.Fprintf(out, "root: %s\n", opts.Root)
			}

			fmt.Fprintf(out, "api key env: EVALCTL_API_KEY=%s OPENAI_API_KEY=%s\n",
				presence("EVALCTL_API_KEY"), presence("OPENAI_API_KEY"))

			if !bashOK {
				return errs.Operational("required tool missing: bash")
			}
			return nil
		},
	}
}

func reportTool(ctx context.Context, out io.Writer, name string, required bool, args ...string) bool {
	path, err := exec.LookPath(name)
	if err != nil {
		suffix := ""
		if required {
			suffix = " (required)"
		}
		fmt.Fprintf(out, "%s: not found%s\n", name, suffix)
		return false
	}
	version := toolVersion(ctx, path, args...)
	if version == "" {
		fmt.Fprintf(out, "%s: %s\n", name, path)
	} else {
		fmt.Fprintf(out, "%s: %s (%s)\n", name, path, version)
	}
	return true
}

func toolVersion(ctx context.Context, path string, args ...string) string {
	output, err := exec.CommandContext(ctx, path, args...).Output()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(output))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	return strings.TrimSpace(line)
}

func presence(name string) string {
	if os.Getenv(name) == "" {
		return "not set"
	}
	return "set"
}
