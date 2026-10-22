// Package atomicfile provides durable whole-file replacement: temp file in the
// same directory, chmod, write, fsync, close, rename, then directory fsync.
package atomicfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrNotDurable reports that path already holds the new content but the
// directory fsync after the rename failed, so the replacement may not survive
// a crash. Callers must not treat it as "not written".
var ErrNotDurable = errors.New("content replaced but directory sync failed")

// syncDirFunc is replaced in tests to simulate a directory fsync failure.
var syncDirFunc = syncDir

// Write atomically replaces path with data using perm. An error wrapping
// ErrNotDurable means the content was replaced; any other error means path is
// unchanged.
func Write(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return fmt.Errorf("atomic write %s: create temp: %w", path, err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("atomic write %s: chmod temp: %w", path, err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("atomic write %s: write temp: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("atomic write %s: sync temp: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("atomic write %s: close temp: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("atomic write %s: rename: %w", path, err)
	}
	cleanup = false

	if err := syncDirFunc(dir); err != nil {
		return fmt.Errorf("atomic write %s: %w: %w", path, ErrNotDurable, err)
	}
	return nil
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return err
	}
	return d.Close()
}
