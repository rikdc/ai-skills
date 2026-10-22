// Package errs defines exit-coded errors and maps errors and signals to
// process exit codes.
package errs

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// Exit codes from the CLI contract.
const (
	CodeOK          = 0
	CodeOperational = 1
	CodeUsage       = 2
	CodeSIGINT      = 130
	CodeSIGTERM     = 143
)

// ExitError carries an explicit process exit code.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit code %d", e.Code)
	}
	return e.Err.Error()
}

// Unwrap exposes the underlying error.
func (e *ExitError) Unwrap() error { return e.Err }

// WithCode wraps err with an exit code. A nil err stays nil.
func WithCode(code int, err error) error {
	if err == nil {
		return nil
	}
	return &ExitError{Code: code, Err: err}
}

// Usage marks a usage, configuration, or validation failure (exit 2).
func Usage(format string, args ...any) error {
	return &ExitError{Code: CodeUsage, Err: fmt.Errorf(format, args...)}
}

// Operational marks an operational failure (exit 1).
func Operational(format string, args ...any) error {
	return &ExitError{Code: CodeOperational, Err: fmt.Errorf(format, args...)}
}

// Code maps err to an exit code. Unclassified errors are operational.
func Code(err error) int {
	if err == nil {
		return CodeOK
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return CodeOperational
}

// SignalCode maps a termination signal to its conventional exit code.
func SignalCode(sig os.Signal) int {
	switch sig {
	case syscall.SIGINT:
		return CodeSIGINT
	case syscall.SIGTERM:
		return CodeSIGTERM
	default:
		return CodeOK
	}
}
