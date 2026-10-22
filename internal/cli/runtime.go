package cli

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/rikdc/ai-skills/internal/config"
	"github.com/rikdc/ai-skills/internal/platform/errs"
)

// nativeFlags holds the milestone-B runtime knobs. Milestone A parses them so
// scripts fail with a clear exit-2 error instead of an unknown-flag error, but
// never silently falls back to the command backend when one is used.
type nativeFlags struct {
	BaseURL             string
	APIKeyEnv           string
	NoAPIKey            bool
	Temperature         float64
	Seed                int64
	MaxOutputTokens     int
	MaxSteps            int
	MaxTotalTokens      int64
	ContextWindow       int
	KeepRecentGroups    int
	RequestTimeout      string
	MaxPromptBytes      int
	MaxToolOutputBytes  int
	MaxArchiveEntries   int
	MaxArchivePathBytes int
}

func addNativeFlags(flags *pflag.FlagSet, o *nativeFlags) {
	flags.StringVar(&o.BaseURL, "base-url", "", "native endpoint (milestone B)")
	flags.StringVar(&o.APIKeyEnv, "api-key-env", "", "native API-key variable name (milestone B)")
	flags.BoolVar(&o.NoAPIKey, "no-api-key", false, "omit Authorization for local servers (milestone B)")
	flags.Float64Var(&o.Temperature, "temperature", 0, "native generation temperature (milestone B)")
	flags.Int64Var(&o.Seed, "seed", 0, "native generation seed (milestone B)")
	flags.IntVar(&o.MaxOutputTokens, "max-output-tokens", 4096, "native per-response cap (milestone B)")
	flags.IntVar(&o.MaxSteps, "max-steps", 40, "native assistant turns (milestone B)")
	flags.Int64Var(&o.MaxTotalTokens, "max-total-tokens", 500000, "native run token budget (milestone B)")
	flags.IntVar(&o.ContextWindow, "context-window", 128000, "native context budget (milestone B)")
	flags.IntVar(&o.KeepRecentGroups, "keep-recent-groups", 3, "native compaction retention (milestone B)")
	flags.StringVar(&o.RequestTimeout, "request-timeout", "120s", "native per-HTTP-attempt deadline (milestone B)")
	flags.IntVar(&o.MaxPromptBytes, "max-prompt-bytes", 262144, "native with-arm prompt ceiling (milestone B)")
	flags.IntVar(&o.MaxToolOutputBytes, "max-tool-output-bytes", 65536, "native tool result cap (milestone B)")
	flags.IntVar(&o.MaxArchiveEntries, "max-archive-entries", 100000, "archive extraction entry cap (milestone B)")
	flags.IntVar(&o.MaxArchivePathBytes, "max-archive-path-bytes", 4096, "archive extraction path cap (milestone B)")
}

// nativeErrors maps each native-only flag to its milestone message. Only
// explicitly changed flags are rejected; ambient environment variables do not
// break command-backend runs.
func (o *nativeFlags) nativeErrors(cmd *cobra.Command) error {
	flags := cmd.Flags()
	for _, name := range []string{
		"base-url", "api-key-env", "no-api-key", "temperature", "seed",
		"max-output-tokens", "max-steps", "max-total-tokens", "context-window",
		"keep-recent-groups", "request-timeout", "max-prompt-bytes",
		"max-tool-output-bytes",
	} {
		if flags.Changed(name) {
			return errs.Usage("--%s requires the native backend, which is milestone B and not implemented; use --backend command", name)
		}
	}
	for _, name := range []string{"max-archive-entries", "max-archive-path-bytes"} {
		if flags.Changed(name) {
			return errs.Usage("--%s is not implemented in milestone A (archive extraction is milestone B)", name)
		}
	}
	return nil
}

// resolveBackend applies --backend > EVALCTL_BACKEND > command.
func resolveBackend(cmd *cobra.Command, flagValue string) string {
	return config.ResolveString(flagValue, cmd.Flags().Changed("backend"), "command", os.Getenv, "EVALCTL_BACKEND")
}

// resolveModel applies --model > EVALCTL_MODEL > empty. The command backend
// uses it only for usage pricing and identity; it never selects a runtime.
func resolveModel(cmd *cobra.Command, flagValue string) string {
	return config.ResolveString(flagValue, cmd.Flags().Changed("model"), "", os.Getenv, "EVALCTL_MODEL")
}

// resolvePricing applies --pricing > EVALCTL_PRICING > empty.
func resolvePricing(cmd *cobra.Command, flagValue string) string {
	return config.ResolveString(flagValue, cmd.Flags().Changed("pricing"), "", os.Getenv, "EVALCTL_PRICING")
}

func validateBackend(backend string) error {
	switch backend {
	case "", "command":
		return nil
	case "openai":
		return errs.Usage("--backend openai is milestone B and not implemented; use --backend command")
	default:
		return errs.Usage("--backend must be command or openai")
	}
}
