package selftest

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/platform/errs"
	"github.com/rikdc/ai-skills/internal/provider/command"
	"github.com/rikdc/ai-skills/internal/runner"
	"github.com/rikdc/ai-skills/internal/scenario"
)

// runFixtures executes every repository scenario's setup through the runner
// with the command backend `true` and the with arm, asserting that the
// documented evidence artifacts were produced.
func runFixtures(ctx context.Context, root, out string, jobs int, logf func(string, ...any)) (int, error) {
	entries, err := scenario.Discover(root)
	if err != nil {
		return 0, fmt.Errorf("selftest fixtures: %w", err)
	}
	if len(entries) == 0 {
		return 0, fmt.Errorf("selftest fixtures: %s has no plugin scenarios", root)
	}
	if jobs < 1 {
		jobs = 1
	}

	exec, err := command.New(command.Config{Command: "true", Timeout: 2 * time.Minute})
	if err != nil {
		return 0, fmt.Errorf("selftest fixtures: %w", err)
	}

	type result struct {
		pair string
		err  error
	}
	jobsCh := make(chan scenario.Entry)
	results := make(chan result)
	var wg sync.WaitGroup
	for range minInt(jobs, len(entries)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for entry := range jobsCh {
				err := runFixture(ctx, root, out, entry, exec)
				results <- result{pair: entry.Pair(), err: err}
			}
		}()
	}
	go func() {
	feed:
		for _, entry := range entries {
			select {
			case <-ctx.Done():
				break feed
			case jobsCh <- entry:
			}
		}
		close(jobsCh)
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	count := 0
	for res := range results {
		if res.err != nil {
			return count, res.err
		}
		count++
		logf("  ok  fixture %s", res.pair)
	}
	if ctx.Err() != nil {
		return count, ctx.Err()
	}
	return count, nil
}

func runFixture(ctx context.Context, root, out string, entry scenario.Entry, exec *command.Executor) error {
	meta, err := scenario.LoadMeta(root, entry.Plugin, entry.Name)
	if err != nil {
		return fmt.Errorf("selftest fixture %s: %w", entry.Pair(), err)
	}
	if meta.Meta.Spec.Execution != nil && meta.Meta.Spec.Execution.Backend == model.ExecutionHook {
		_, err := runner.New(runner.Config{
			Root:     root,
			Plugin:   entry.Plugin,
			Scenario: entry.Name,
			Arm:      "with",
			Runs:     1,
			Out:      filepath.Join(out, entry.Plugin, entry.Name),
			Executor: exec,
		})
		if err == nil || errs.Code(err) != errs.CodeUsage {
			return fmt.Errorf("selftest fixture %s: direct-hook scenario did not return exit 2 (err=%v)",
				entry.Pair(), err)
		}
		return nil
	}

	r, err := runner.New(runner.Config{
		Root:             root,
		Plugin:           entry.Plugin,
		Scenario:         entry.Name,
		Arm:              "with",
		Runs:             1,
		SkillDir:         ".claude/skills",
		Out:              filepath.Join(out, entry.Plugin, entry.Name),
		Timeout:          2 * time.Minute,
		SetupTimeout:     2 * time.Minute,
		ChecksTimeout:    time.Minute,
		MaxSnapshotBytes: 64 << 20,
		AttemptArtifacts: "all",
		Executor:         exec,
	})
	if err != nil {
		return fmt.Errorf("selftest fixture %s: %w", entry.Pair(), err)
	}
	summary, err := r.Run(ctx)
	if err != nil {
		return fmt.Errorf("selftest fixture %s: %w", entry.Pair(), err)
	}
	if len(summary.Runs) != 1 || summary.Runs[0].Status != model.StatusOK {
		return fmt.Errorf("selftest fixture %s: status %q", entry.Pair(), summary.Runs[0].Status)
	}
	runDir := summary.Runs[0].Dir
	if err := requireFiles(filepath.Join(runDir, "attempts", "agent-000001"),
		"setup.log", "before.txt", "after.txt", "snapshot.tar"); err != nil {
		return fmt.Errorf("selftest fixture %s: %w", entry.Pair(), err)
	}
	if err := requireFiles(runDir, "grading-request.json", "result.json", "current.json"); err != nil {
		return fmt.Errorf("selftest fixture %s: %w", entry.Pair(), err)
	}
	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
