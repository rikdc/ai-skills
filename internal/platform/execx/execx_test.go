package execx

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestExitCode(t *testing.T) {
	res, err := Run(context.Background(), Spec{Path: "/bin/sh", Args: []string{"-c", "exit 42"}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Started || res.ExitCode != 42 || res.Signaled {
		t.Fatalf("result: %+v", res)
	}
}

func TestStartFailure(t *testing.T) {
	res, err := Run(context.Background(), Spec{Path: filepath.Join(t.TempDir(), "missing-binary")})
	if err == nil {
		t.Fatal("expected a start error")
	}
	if res.Started {
		t.Fatal("started a missing binary")
	}
	if res.ExitCode != -1 {
		t.Fatalf("exit code: %d", res.ExitCode)
	}
}

func TestCancelBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := Run(ctx, Spec{Path: "/bin/sh", Args: []string{"-c", "sleep 30"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error: %v", err)
	}
	if res.Started {
		t.Fatal("started after cancellation")
	}
}

func TestCancelDuringStartup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	res, err := Run(ctx, Spec{
		Path: "/bin/sh", Args: []string{"-c", "sleep 30"},
		Grace: 200 * time.Millisecond, WaitDelay: 300 * time.Millisecond,
	})
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error: %v", err)
	}
	if !res.Started || !res.TimedOut || res.Cancelled {
		t.Fatalf("result: %+v", res)
	}
	if res.ExitCode != 124 {
		t.Fatalf("exit code: %d", res.ExitCode)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("cancellation took %v", elapsed)
	}
}

func TestLeaderExitBackgroundChild(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	script := `sleep 30 & echo $! > "$PIDFILE"; exit 0`
	res, err := Run(context.Background(), Spec{
		Path: "/bin/sh",
		Args: []string{"-c", script},
		Env:  append(os.Environ(), "PIDFILE="+pidFile),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("result: %+v", res)
	}
	pid := readPID(t, pidFile)
	if !waitGone(pid, 3*time.Second) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Fatalf("background child %d survived the group cleanup", pid)
	}
}

func TestInheritedPipes(t *testing.T) {
	var out bytes.Buffer
	start := time.Now()
	res, err := Run(context.Background(), Spec{
		Path:       "/bin/sh",
		Args:       []string{"-c", "sleep 30 & exit 0"},
		Stdout:     &out,
		GroupGrace: 200 * time.Millisecond,
		WaitDelay:  200 * time.Millisecond,
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("inherited pipes blocked for %v", elapsed)
	}
	if res.ExitCode != 0 {
		t.Fatalf("result: %+v", res)
	}
}

func TestTermResistant(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	res, err := Run(ctx, Spec{
		Path:       "/bin/sh",
		Args:       []string{"-c", "trap '' TERM; while :; do sleep 0.05; done"},
		Grace:      150 * time.Millisecond,
		GroupGrace: 100 * time.Millisecond,
		WaitDelay:  300 * time.Millisecond,
	})
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error: %v", err)
	}
	if !res.TimedOut {
		t.Fatalf("result: %+v", res)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("KILL escalation took %v", elapsed)
	}
}

func TestEscalationAfterReap(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	script := `(trap '' TERM; while :; do sleep 0.05; done) & echo $! > "$PIDFILE"; exit 0`
	res, err := Run(context.Background(), Spec{
		Path:       "/bin/sh",
		Args:       []string{"-c", script},
		Env:        append(os.Environ(), "PIDFILE="+pidFile),
		GroupGrace: 150 * time.Millisecond,
		WaitDelay:  300 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("result: %+v", res)
	}
	pid := readPID(t, pidFile)
	if !waitGone(pid, 3*time.Second) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Fatalf("TERM-resistant background child %d survived KILL escalation", pid)
	}
}

// TestEscapedSessionDocumented asserts the documented limitation: a child that
// calls setsid() leaves the group and is not signalled. The test cleans the
// escaped process up explicitly.
func TestEscapedSessionDocumented(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "escaped.pid")
	readyFile := filepath.Join(dir, "escaped.ready")
	helper := os.Args[0]
	script := `"$HELPER" -test.run=TestEscapedSessionHelper >/dev/null 2>&1 & echo $! > "$PIDFILE"
for i in $(seq 1 200); do [ -f "$READY" ] && break; sleep 0.01; done
exit 0`
	res, err := Run(context.Background(), Spec{
		Path: "/bin/sh",
		Args: []string{"-c", script},
		Env: append(os.Environ(),
			"HELPER="+helper,
			"PIDFILE="+pidFile,
			"READY="+readyFile,
			"EXECX_ESCAPE_READY="+readyFile,
			"HOME="+dir,
		),
		GroupGrace: 150 * time.Millisecond,
		WaitDelay:  500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("result: %+v", res)
	}
	pid := readPID(t, pidFile)
	if pid <= 0 {
		t.Fatal("helper did not report a pid")
	}
	if syscall.Kill(pid, 0) != nil {
		t.Fatalf("escaped session %d was signalled; the documented escape no longer holds", pid)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatalf("clean up escaped process: %v", err)
	}
	_ = waitGone(pid, 3*time.Second)
}

// TestEscapedSessionHelper is the re-executed helper for the escape test. It
// skips during normal test runs.
func TestEscapedSessionHelper(t *testing.T) {
	ready := os.Getenv("EXECX_ESCAPE_READY")
	if ready == "" {
		t.Skip("helper process for TestEscapedSessionDocumented")
	}
	if _, err := syscall.Setsid(); err != nil {
		t.Fatalf("setsid: %v", err)
	}
	if err := os.WriteFile(ready, []byte("ready"), 0o600); err != nil {
		t.Fatalf("write ready: %v", err)
	}
	time.Sleep(30 * time.Second)
}

func TestOutputCap(t *testing.T) {
	script := `head -c 1000 /dev/zero | tr '\0' a`
	res, err := Run(context.Background(), Spec{
		Path:           "/bin/sh",
		Args:           []string{"-c", script},
		MaxOutputBytes: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated {
		t.Fatal("expected truncation")
	}
	if len(res.Stdout) > 100+len(truncationMarker) {
		t.Fatalf("captured %d bytes", len(res.Stdout))
	}
	if !strings.Contains(string(res.Stdout), "output truncated") {
		t.Fatalf("missing marker: %q", res.Stdout)
	}
	if !strings.HasPrefix(string(res.Stdout), strings.Repeat("a", 100)) {
		t.Fatal("prefix was not preserved")
	}
}

func TestStderrCapturedSeparately(t *testing.T) {
	res, err := Run(context.Background(), Spec{
		Path: "/bin/sh", Args: []string{"-c", "echo out; echo err >&2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res.Stdout), "out") || strings.Contains(string(res.Stdout), "err") {
		t.Fatalf("stdout: %q", res.Stdout)
	}
	if !strings.Contains(string(res.Stderr), "err") {
		t.Fatalf("stderr: %q", res.Stderr)
	}
}

func readPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, convErr := strconv.Atoi(strings.TrimSpace(string(data)))
			if convErr == nil && pid > 0 {
				return pid
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("pid file %s was not written", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitGone(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func TestSignalFailureIgnored(t *testing.T) {
	// A group that never existed must not panic or block.
	signalGroup(-1, syscall.SIGTERM)
	terminateGroup(0, 10*time.Millisecond)
}
