package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	if err := Write(path, []byte("first\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "first\n" {
		t.Fatalf("content = %q", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o, want 640", info.Mode().Perm())
	}
}

func TestWriteReplacesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	if err := Write(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("content = %q, want new", data)
	}
}

func TestWriteMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "data.json")
	if err := Write(path, []byte("x"), 0o644); err == nil {
		t.Fatal("write into missing directory succeeded")
	}
}

func TestWriteLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	if err := Write(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "data.json" {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("unexpected files: %s", strings.Join(names, ", "))
	}
}

func TestWriteDirSyncFailureReportsReplacedContent(t *testing.T) {
	syncErr := errors.New("sync failed")
	syncDirFunc = func(string) error { return syncErr }
	t.Cleanup(func() { syncDirFunc = syncDir })

	path := filepath.Join(t.TempDir(), "data.json")
	err := Write(path, []byte("new"), 0o644)
	if !errors.Is(err, ErrNotDurable) || !errors.Is(err, syncErr) {
		t.Fatalf("err = %v, want ErrNotDurable wrapping the sync error", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "new" {
		t.Fatalf("content = %q, want new", data)
	}
}

func TestWriteEarlyFailureIsNotErrNotDurable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "data.json")
	if err := Write(path, []byte("x"), 0o644); errors.Is(err, ErrNotDurable) {
		t.Fatalf("err = %v, want a plain failure", err)
	}
}
