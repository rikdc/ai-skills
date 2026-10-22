// Package execx runs a child process in its own process group with bounded
// I/O and a TERM-then-KILL escalation policy.
//
// The contract is best-effort signalling of a cooperating process group, not
// containment. Arbitrary host code can escape the group (for example with
// setsid); that limitation is documented and tested rather than hidden.
package execx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Defaults from the v3 lifecycle contract.
const (
	DefaultGrace        = 5 * time.Second
	DefaultWaitDelay    = 2 * time.Second
	DefaultGroupGrace   = 500 * time.Millisecond
	DefaultMaxOutput    = 8 << 20 // 8 MiB per stream
	truncationMarker    = "\n[evalctl: output truncated]\n"
	signalPollInterval  = 10 * time.Millisecond
	killedAfterSignaled = 128
)

// Spec describes one child invocation.
type Spec struct {
	Path string
	Args []string
	Dir  string
	Env  []string

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// Grace bounds the wait after SIGTERM during cancellation before SIGKILL.
	Grace time.Duration
	// GroupGrace bounds the wait after SIGTERM when a leader exits and leaves
	// background group members behind.
	GroupGrace time.Duration
	// WaitDelay bounds pipe draining and direct-child reaping.
	WaitDelay time.Duration
	// MaxOutputBytes caps each captured stream (<=0 means DefaultMaxOutput).
	MaxOutputBytes int
}

// Result describes the observed process outcome. A non-zero exit code is not
// an error; start failures and context termination are returned as errors.
type Result struct {
	Started    bool
	ExitCode   int
	Signaled   bool
	Signal     syscall.Signal
	TimedOut   bool
	Cancelled  bool
	Duration   time.Duration
	Stdout     []byte
	Stderr     []byte
	Truncated  bool
	Residual   bool // a process may still be alive after the bounded wait
	StartError string
}

// Run starts the command and waits for it, signalling its process group on
// cancellation and after leader exit.
func Run(ctx context.Context, spec Spec) (Result, error) {
	started := time.Now()
	result := Result{ExitCode: -1}

	grace := spec.Grace
	if grace <= 0 {
		grace = DefaultGrace
	}
	groupGrace := spec.GroupGrace
	if groupGrace <= 0 {
		groupGrace = DefaultGroupGrace
	}
	waitDelay := spec.WaitDelay
	if waitDelay <= 0 {
		waitDelay = DefaultWaitDelay
	}
	limit := spec.MaxOutputBytes
	if limit <= 0 {
		limit = DefaultMaxOutput
	}

	if err := ctx.Err(); err != nil {
		result.StartError = err.Error()
		result.Duration = time.Since(started)
		return result, err
	}

	cmd := exec.Command(spec.Path, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	cmd.Stdin = spec.Stdin
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = waitDelay

	stdout, stdoutWrite, err := attachStream(cmd, true, spec.Stdout, limit)
	if err != nil {
		return result, err
	}
	stderr, stderrWrite, err := attachStream(cmd, false, spec.Stderr, limit)
	if err != nil {
		closeWrite(stdoutWrite)
		stdout.abort()
		return result, err
	}

	if err := cmd.Start(); err != nil {
		closeWrite(stdoutWrite)
		closeWrite(stderrWrite)
		stdout.abort()
		stderr.abort()
		result.StartError = err.Error()
		result.Duration = time.Since(started)
		return result, fmt.Errorf("start %s: %w", spec.Path, err)
	}
	result.Started = true

	// The parent must not hold the write ends: EOF depends on every child
	// copy closing them.
	closeWrite(stdoutWrite)
	closeWrite(stderrWrite)

	pgid := cmd.Process.Pid

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	var (
		waitErr  error
		gotWait  bool
		cancel   bool
		timedOut bool
		residual bool
	)

	select {
	case waitErr = <-waitCh:
		gotWait = true
		// Leader exited: cooperating background children get a short grace,
		// then the group is killed so inherited pipes cannot hang us.
		terminateGroup(pgid, groupGrace)
	case <-ctx.Done():
		cancel = true
		timedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)

		signalGroup(pgid, syscall.SIGTERM)
		timer := time.NewTimer(grace)
		select {
		case waitErr = <-waitCh:
			gotWait = true
			timer.Stop()
		case <-timer.C:
			signalGroup(pgid, syscall.SIGKILL)
			timer = time.NewTimer(waitDelay)
			select {
			case waitErr = <-waitCh:
				gotWait = true
			case <-timer.C:
				residual = true
			}
			timer.Stop()
		}
		// Background members may have survived the leader; escalate once more.
		terminateGroup(pgid, groupGrace)
	}

	if gotWait {
		result.ExitCode, result.Signal, result.Signaled = exitStatus(cmd.ProcessState, waitErr)
	}

	// Bound I/O even when a child inherited the pipe and never exits.
	stdout.wait(waitDelay)
	stderr.wait(waitDelay)

	result.Duration = time.Since(started)
	result.Stdout = stdout.bytes()
	result.Stderr = stderr.bytes()
	result.Truncated = stdout.truncated() || stderr.truncated()
	result.TimedOut = timedOut
	result.Cancelled = cancel && !timedOut
	result.Residual = residual

	if cancel {
		if gotWait {
			result.ExitCode = killedAfterSignaled + int(syscall.SIGKILL)
			if timedOut {
				result.ExitCode = 124
			} else {
				result.ExitCode = 130
			}
		}
		if timedOut {
			return result, fmt.Errorf("command deadline exceeded: %w", context.DeadlineExceeded)
		}
		return result, fmt.Errorf("command cancelled: %w", context.Canceled)
	}
	return result, nil
}

func attachStream(cmd *exec.Cmd, stdout bool, w io.Writer, limit int) (*stream, *os.File, error) {
	if f, ok := w.(*os.File); ok && w != nil {
		// A real file is safe to hand to the child directly: no inherited pipe
		// can deadlock, so the log is not capped.
		if stdout {
			cmd.Stdout = f
		} else {
			cmd.Stderr = f
		}
		return newStream(nil, 0), nil, nil
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("create output pipe: %w", err)
	}
	s := newStream(w, limit)
	s.read = pr
	go s.drain(pr)
	if stdout {
		cmd.Stdout = pw
	} else {
		cmd.Stderr = pw
	}
	return s, pw, nil
}

func closeWrite(f *os.File) {
	if f != nil {
		_ = f.Close()
	}
}

func exitStatus(state *os.ProcessState, waitErr error) (int, syscall.Signal, bool) {
	if state != nil {
		if ws, ok := state.Sys().(syscall.WaitStatus); ok {
			if ws.Signaled() {
				return killedAfterSignaled + int(ws.Signal()), ws.Signal(), true
			}
			if ws.Exited() {
				return ws.ExitStatus(), 0, false
			}
		}
		if code := state.ExitCode(); code >= 0 {
			return code, 0, false
		}
	}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) && exitErr.ProcessState != nil {
			return exitErr.ProcessState.ExitCode(), 0, false
		}
	}
	return -1, 0, false
}

// signalGroup signals the group led by pgid. ESRCH is expected once the group
// is empty; EPERM is reported but not fatal.
func signalGroup(pgid int, sig syscall.Signal) {
	if pgid <= 0 {
		return
	}
	err := syscall.Kill(-pgid, sig)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return
	}
	// The caller has no logger here. The error is intentionally swallowed:
	// signalling is best effort and children may already be gone. EPERM means
	// the process was not ours to signal; killing further is pointless.
	_ = err
}

func groupAlive(pgid int) bool {
	if pgid <= 0 {
		return false
	}
	err := syscall.Kill(-pgid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// terminateGroup sends TERM, waits up to grace, and escalates to KILL.
func terminateGroup(pgid int, grace time.Duration) {
	if !groupAlive(pgid) {
		return
	}
	signalGroup(pgid, syscall.SIGTERM)
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if !groupAlive(pgid) {
			return
		}
		time.Sleep(signalPollInterval)
	}
	signalGroup(pgid, syscall.SIGKILL)
}

// stream drains one child pipe with a bounded wait and a byte cap.
type stream struct {
	mu        sync.Mutex
	writer    io.Writer
	limit     int
	buf       bytes.Buffer
	remaining int
	cut       bool
	writeErr  error

	read     *os.File
	done     chan struct{}
	readOnce sync.Once
}

func newStream(w io.Writer, limit int) *stream {
	return &stream{
		writer:    w,
		limit:     limit,
		remaining: limit,
		done:      make(chan struct{}),
	}
}

func (s *stream) drain(r *os.File) {
	defer close(s.done)
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			_, _ = s.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func (s *stream) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := 0
	if s.limit > 0 && s.remaining > 0 {
		n = len(p)
		if n > s.remaining {
			n = s.remaining
		}
		s.buf.Write(p[:n])
		s.remaining -= n
		if s.writer != nil {
			if _, err := s.writer.Write(p[:n]); err != nil && s.writeErr == nil {
				s.writeErr = err
			}
		}
	} else if s.limit <= 0 {
		n = len(p)
		s.buf.Write(p[:n])
		if s.writer != nil {
			if _, err := s.writer.Write(p[:n]); err != nil && s.writeErr == nil {
				s.writeErr = err
			}
		}
	}

	if n < len(p) && !s.cut {
		s.cut = true
		s.buf.WriteString(truncationMarker)
	}
	// Always report a full write: the child must never see EPIPE because we
	// chose to stop capturing.
	return len(p), nil
}

// wait blocks until the drain goroutine sees EOF, bounded by delay. Closing
// the read end unblocks a drain whose child inherited the write end.
func (s *stream) wait(delay time.Duration) {
	if s.read == nil {
		return
	}
	if delay <= 0 {
		delay = DefaultWaitDelay
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-s.done:
	case <-timer.C:
		s.readOnce.Do(func() { _ = s.read.Close() })
		// Bound the post-close wait too: a user-supplied writer could itself
		// block. Captured bytes remain mutex-protected.
		again := time.NewTimer(delay)
		defer again.Stop()
		select {
		case <-s.done:
		case <-again.C:
		}
	}
}

func (s *stream) abort() {
	if s.read != nil {
		s.readOnce.Do(func() { _ = s.read.Close() })
	}
}

func (s *stream) bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]byte, s.buf.Len())
	copy(out, s.buf.Bytes())
	return out
}

func (s *stream) truncated() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cut
}
