//go:build unix

package suite

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"

	"github.com/rikdc/ai-skills/internal/platform/errs"
)

// lockFile takes a non-blocking exclusive flock on <dir>/.evalctl.lock. It
// returns a release function. A second writer receives an exit-2 error.
func lockFile(dir string) (func(), error) {
	path := filepath.Join(dir, ".evalctl.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, errs.Operational("create suite lock: %v", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, errs.Usage("%s is already being written by another evalctl suite", dir)
		}
		return nil, errs.Operational("lock %s: %v", path, err)
	}
	release := func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}
	return release, nil
}
