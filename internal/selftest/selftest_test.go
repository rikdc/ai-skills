package selftest

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/rikdc/ai-skills/internal/platform/errs"
	"github.com/rikdc/ai-skills/internal/scenario"
)

func TestQuickEndToEnd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	result, err := Run(context.Background(), Options{Quick: true, Out: io.Discard})
	if err != nil {
		t.Fatalf("selftest: %v", err)
	}
	if result.Checks < 7 {
		t.Fatalf("checks = %d, want at least 7", result.Checks)
	}
	if result.Fixtures != 0 {
		t.Fatalf("fixtures = %d, want 0 with --quick", result.Fixtures)
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("selftest wrote to the real HOME: %v", entries)
	}
}

func TestKeepRetainsWorkDirectory(t *testing.T) {
	result, err := Run(context.Background(), Options{Quick: true, Keep: true, Out: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(result.BaseDir)
	if _, err := os.Stat(filepath.Join(result.BaseDir, "end-to-end", "suite", "manifest.json")); err != nil {
		t.Fatalf("kept work directory is missing the suite: %v", err)
	}
}

func TestFixtureCoverageRequiresRoot(t *testing.T) {
	_, err := Run(context.Background(), Options{Out: io.Discard})
	if errs.Code(err) != 2 {
		t.Fatalf("err = %v (code %d), want exit 2", err, errs.Code(err))
	}
}

func TestFixtureCoverageRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("repository fixture coverage runs every plugin setup.sh")
	}
	root := repositoryRoot(t)
	entries, err := scenario.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "fixtures")
	count, err := runFixtures(context.Background(), root, out, 4, func(string, ...any) {})
	if err != nil {
		t.Fatalf("fixture coverage: %v", err)
	}
	if count != len(entries) {
		t.Fatalf("fixture count = %d, want %d", count, len(entries))
	}
}

func TestFixtureCoverageDirectHookIsMilestoneC(t *testing.T) {
	root := t.TempDir()
	if err := writeSyntheticRepo(root); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "plugins", "selftest", "evals", "direct")
	writeTestFile(t, filepath.Join(dir, "scenario.json"),
		`{"description":"direct hook","include":["./inputs"],"unit":"hooks/h","mode":"functional","execution":{"backend":"hook"}}`)
	writeTestFile(t, filepath.Join(dir, "task.md"), "# Task\n")
	writeTestFile(t, filepath.Join(dir, "criteria.json"), `{"context":"c","type":"weighted_checklist","checklist":[{"name":"a","max_score":10}]}`)
	writeTestFile(t, filepath.Join(dir, "inputs", "setup.sh"), "#!/usr/bin/env bash\nset -euo pipefail\ntrue\n")
	writeTestFile(t, filepath.Join(root, "plugins", "selftest", "hooks", "h.sh"), "#!/usr/bin/env bash\n")

	out := filepath.Join(t.TempDir(), "fixtures")
	count, err := runFixtures(context.Background(), root, out, 2, func(string, ...any) {})
	if err != nil {
		t.Fatalf("fixture coverage: %v", err)
	}
	if count != 2 {
		t.Fatalf("fixture count = %d, want 2", count)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "tessl.json")); err != nil {
		t.Skipf("not running inside the repository: %v", err)
	}
	return root
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	mode := os.FileMode(0o644)
	if filepath.Ext(path) == ".sh" {
		mode = 0o755
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}
