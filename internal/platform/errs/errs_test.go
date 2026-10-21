package errs

import (
	"errors"
	"fmt"
	"syscall"
	"testing"
)

func TestCode(t *testing.T) {
	if got := Code(nil); got != CodeOK {
		t.Fatalf("nil code = %d, want 0", got)
	}
	if got := Code(errors.New("plain")); got != CodeOperational {
		t.Fatalf("plain code = %d, want 1", got)
	}
	if got := Code(Usage("bad flag")); got != CodeUsage {
		t.Fatalf("usage code = %d, want 2", got)
	}
	if got := Code(Operational("failed")); got != CodeOperational {
		t.Fatalf("operational code = %d, want 1", got)
	}
	wrapped := fmt.Errorf("outer: %w", Usage("inner"))
	if got := Code(wrapped); got != CodeUsage {
		t.Fatalf("wrapped usage code = %d, want 2", got)
	}
	if got := Code(WithCode(7, errors.New("custom"))); got != 7 {
		t.Fatalf("custom code = %d, want 7", got)
	}
}

func TestSignalCode(t *testing.T) {
	if got := SignalCode(syscall.SIGINT); got != 130 {
		t.Fatalf("SIGINT code = %d, want 130", got)
	}
	if got := SignalCode(syscall.SIGTERM); got != 143 {
		t.Fatalf("SIGTERM code = %d, want 143", got)
	}
	if got := SignalCode(syscall.SIGHUP); got != 0 {
		t.Fatalf("SIGHUP code = %d, want 0", got)
	}
}

func TestWithCodeNil(t *testing.T) {
	if err := WithCode(2, nil); err != nil {
		t.Fatalf("WithCode(nil) = %v", err)
	}
}
