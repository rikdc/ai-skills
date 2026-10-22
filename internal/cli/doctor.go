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

	"github.com/rikdc/ai-skills/internal/config"
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
			backend := config.ResolveString("", false, "command", os.Getenv, "EVALCTL_BACKEND")
			modelName := os.Getenv("EVALCTL_MODEL")
			if modelName == "" {
				modelName = "(unset)"
			}
			baseURL := os.Getenv("EVALCTL_OPENAI_BASE_URL")
			if baseURL == "" {
				baseURL = os.Getenv("OPENAI_BASE_URL")
			}
			if baseURL == "" {
				baseURL = "https://api.openai.com/v1"
			}
			apiKeyEnv := os.Getenv("EVALCTL_API_KEY_ENV")
			if apiKeyEnv == "" {
				apiKeyEnv = "(default: EVALCTL_API_KEY then OPENAI_API_KEY)"
			}
			fmt.Fprintf(out, "backend: %s\n", backend)
			fmt.Fprintf(out, "model: %s\n", modelName)
			fmt.Fprintf(out, "base url: %s\n", baseURL)
			fmt.Fprintf(out, "api key env var: %s\n", apiKeyEnv)
			fmt.Fprintf(out, "archive limits: snapshot %d MiB\n", 256)

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
