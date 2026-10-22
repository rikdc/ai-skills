package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/rikdc/ai-skills/internal/config"
	"github.com/rikdc/ai-skills/internal/platform/atomicfile"
	"github.com/rikdc/ai-skills/internal/platform/errs"
	"github.com/rikdc/ai-skills/internal/report"
	"github.com/rikdc/ai-skills/internal/reportio"
)

func newReportCommand() *cobra.Command {
	var format string
	var outDir string
	var baselineDir string

	cmd := &cobra.Command{
		Use:   "report SUITE_DIR",
		Short: "Render a report from an existing suite",
		Long: "Render an existing suite without running an eval, model, or network call. " +
			"The suite is not modified; reports go to SUITE_DIR/report by default.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReport(cmd, args[0], format, outDir, baselineDir)
		},
	}
	cmd.Flags().StringVar(&format, "format", "cli", "cli|json|md|html|all")
	cmd.Flags().StringVar(&outDir, "out", "", "report directory (default SUITE_DIR/report)")
	cmd.Flags().StringVar(&baselineDir, "baseline", "", "compare with an earlier suite directory")
	return cmd
}

func runReport(cmd *cobra.Command, suiteDir, format, outDir, baselineDir string) error {
	switch format {
	case "cli", "json", "md", "html", "all":
	default:
		return errs.Usage("--format must be cli, json, md, html or all")
	}
	repoRoot := discoverRoot(cmd)
	loaded, err := reportio.Load(suiteDir, reportio.Options{RepoRoot: repoRoot})
	if err != nil {
		return errs.Operational("%v", err)
	}
	result := report.Build(loaded)
	if baselineDir != "" {
		baseline, err := reportio.Load(baselineDir, reportio.Options{RepoRoot: repoRoot})
		if err != nil {
			return errs.Operational("baseline: %v", err)
		}
		result.Comparison = report.Compare(result, report.Build(baseline))
	}

	if outDir == "" {
		outDir = filepath.Join(loaded.SuiteDir, "report")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return errs.Operational("cannot create report directory: %v", err)
	}
	jsonPath := filepath.Join(outDir, "report.json")
	mdPath := filepath.Join(outDir, "report.md")
	htmlPath := filepath.Join(outDir, "report.html")

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return errs.Operational("encode report: %v", err)
	}
	jsonBytes = append(jsonBytes, '\n')
	if err := atomicfile.Write(jsonPath, jsonBytes, 0o644); err != nil {
		return errs.Operational("cannot write report: %v", err)
	}

	color := reportColor(cmd)
	stdout := cmd.OutOrStdout()
	stderr := cmd.ErrOrStderr()
	markdown := func() ([]byte, error) {
		data := []byte(report.RenderMarkdown(result))
		return data, atomicfile.Write(mdPath, data, 0o644)
	}
	htmlDoc := func() ([]byte, error) {
		data := []byte(report.RenderHTML(result))
		return data, atomicfile.Write(htmlPath, data, 0o644)
	}

	switch format {
	case "cli":
		fmt.Fprint(stdout, report.RenderCLI(result, color))
		fmt.Fprintf(stdout, "\nReport directory: %s\n", outDir)
	case "json":
		fmt.Fprint(stdout, string(jsonBytes))
		fmt.Fprintf(stderr, "Report: %s\n", jsonPath)
	case "md":
		data, err := markdown()
		if err != nil {
			return errs.Operational("cannot write report: %v", err)
		}
		fmt.Fprint(stdout, string(data))
		fmt.Fprintf(stderr, "Report: %s\n", mdPath)
	case "html":
		if _, err := htmlDoc(); err != nil {
			return errs.Operational("cannot write report: %v", err)
		}
		fmt.Fprintf(stdout, "Report: %s\n", htmlPath)
	case "all":
		if _, err := markdown(); err != nil {
			return errs.Operational("cannot write report: %v", err)
		}
		if _, err := htmlDoc(); err != nil {
			return errs.Operational("cannot write report: %v", err)
		}
		fmt.Fprint(stdout, report.RenderCLI(result, color))
		fmt.Fprintf(stdout, "\nReports:\n  %s\n  %s\n  %s\n", jsonPath, mdPath, htmlPath)
	}
	return nil
}

func discoverRoot(cmd *cobra.Command) string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	flagRoot, _ := cmd.Flags().GetString("root")
	root, err := config.ResolveRoot(flagRoot, cmd.Flags().Changed("root"), cwd, os.Getenv)
	if err != nil {
		return ""
	}
	return root
}

func reportColor(cmd *cobra.Command) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if flag := cmd.Flag("no-color"); flag != nil {
		if value, err := strconv.ParseBool(flag.Value.String()); err == nil && value {
			return false
		}
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
