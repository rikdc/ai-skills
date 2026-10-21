// Package command implements the Milestone A command adapter. It executes a
// whole agent or judge with /bin/bash -c and implements executor.AgentExecutor
// only; it is deliberately not a ChatProvider.
package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rikdc/ai-skills/internal/executor"
	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/platform/errs"
	"github.com/rikdc/ai-skills/internal/platform/execx"
	"github.com/rikdc/ai-skills/internal/usage"
)

// Stop reasons specific to the command adapter.
const (
	stopAgentError      = "agent_error"
	stopCredentialError = "credential_error"
)

// defaultCredentialPaths are the known CLI login/config files. They are only
// copied when --auth-cli is enabled.
var defaultCredentialPaths = []string{
	".claude.json",
	".claude/.credentials.json",
	".claude/settings.json",
	".codex/auth.json",
	".codex/config.toml",
	".config/gh/hosts.yml",
	".local/share/opencode/auth.json",
}

// AuthOptions mirrors the split credential flags. Nothing is copied by
// default; each opt-in requires --auth-cli.
type AuthOptions struct {
	CLI      bool
	Keychain bool
	Paths    []string
	// RealHome overrides the source home directory. Empty means the invoking
	// user's home.
	RealHome string
}

// Validate enforces the flag contract (exit 2 for misuse).
func (a AuthOptions) Validate() error {
	if a.Keychain && !a.CLI {
		return errs.Usage("--keychain requires --auth-cli")
	}
	if len(a.Paths) > 0 && !a.CLI {
		return errs.Usage("--auth-path requires --auth-cli")
	}
	if a.Keychain && runtime.GOOS != "darwin" {
		return errs.Usage("--keychain is only supported on macOS")
	}
	for _, p := range a.Paths {
		if err := validateAuthPath(p); err != nil {
			return err
		}
	}
	return nil
}

func validateAuthPath(p string) error {
	if p == "" {
		return errs.Usage("--auth-path must not be empty")
	}
	if filepath.IsAbs(p) {
		return errs.Usage("--auth-path must be relative to the home directory: %s", p)
	}
	clean := filepath.Clean(p)
	if clean == "." || clean == ".." ||
		strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return errs.Usage("--auth-path must stay under the home directory: %s", p)
	}
	return nil
}

func resolveHome(realHome string) string {
	if realHome != "" {
		return realHome
	}
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return h
	}
	return os.Getenv("HOME")
}

// prepare copies the selected credentials into home and returns an idempotent
// cleanup function. Credentials are removed immediately after the agent exits.
func prepareAuth(auth AuthOptions, home string) (func(), error) {
	noop := func() {}
	if !auth.CLI {
		return noop, nil
	}
	realHome := resolveHome(auth.RealHome)
	if realHome == "" {
		return noop, nil
	}

	var copied []string
	var links []string
	cleanup := func() {
		for _, p := range copied {
			_ = os.Remove(p)
			removeEmptyParents(home, filepath.Dir(p))
		}
		for _, p := range links {
			if fi, err := os.Lstat(p); err == nil && fi.Mode()&os.ModeSymlink != 0 {
				_ = os.Remove(p)
			}
			removeEmptyParents(home, filepath.Dir(p))
		}
	}

	selected := append([]string{}, defaultCredentialPaths...)
	for _, p := range auth.Paths {
		if err := validateAuthPath(p); err != nil {
			cleanup()
			return noop, err
		}
		selected = append(selected, p)
	}
	seen := make(map[string]bool, len(selected))
	for _, rel := range selected {
		rel = filepath.Clean(rel)
		if seen[rel] {
			continue
		}
		seen[rel] = true
		src := filepath.Join(realHome, rel)
		fi, err := os.Stat(src)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			cleanup()
			return noop, credentialCopyError(src, err)
		}
		if fi.IsDir() {
			continue
		}
		dest := filepath.Join(home, rel)
		if err := copyFile(src, dest); err != nil {
			cleanup()
			return noop, credentialCopyError(src, err)
		}
		copied = append(copied, dest)
	}

	if auth.Keychain {
		src := filepath.Join(realHome, "Library", "Keychains")
		if fi, err := os.Stat(src); err == nil && fi.IsDir() {
			dest := filepath.Join(home, "Library", "Keychains")
			if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
				cleanup()
				return noop, credentialCopyError(src, err)
			}
			if err := os.Symlink(src, dest); err != nil {
				cleanup()
				return noop, credentialCopyError(src, err)
			}
			links = append(links, dest)
		}
	}

	var once sync.Once
	return func() { once.Do(cleanup) }, nil
}

func credentialCopyError(src string, err error) error {
	return errs.Operational("cannot copy agent credentials from %s: %v; rerun without --auth-cli to skip credential copying, or pass --auth-path for the file your agent CLI actually uses", src, err)
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dest)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dest)
		return err
	}
	return nil
}

func removeEmptyParents(stop, dir string) {
	stop = filepath.Clean(stop)
	for {
		dir = filepath.Clean(dir)
		if dir == stop || dir == "." || dir == string(filepath.Separator) {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

// Config configures a command executor.
type Config struct {
	Command    string
	ShellPath  string
	Timeout    time.Duration
	Grace      time.Duration
	GroupGrace time.Duration
	WaitDelay  time.Duration
	Auth       AuthOptions
	// ParentEnv is the base environment; nil uses os.Environ().
	ParentEnv []string
	// PassEnv lists extra variables that survive sanitization.
	PassEnv []string
	// UsageCmd is the optional post-run usage hook.
	UsageCmd string
	// Pricing and Model enable cost calculation from known usage.
	Pricing usage.Pricing
	Model   string
}

// Executor implements executor.AgentExecutor.
type Executor struct {
	cfg      Config
	identity executor.RuntimeIdentity
}

var _ executor.AgentExecutor = (*Executor)(nil)

// New validates cfg and returns a command executor.
func New(cfg Config) (*Executor, error) {
	if strings.TrimSpace(cfg.Command) == "" {
		return nil, errs.Usage("command adapter requires a non-empty command")
	}
	if err := cfg.Auth.Validate(); err != nil {
		return nil, err
	}
	if cfg.ShellPath == "" {
		cfg.ShellPath = "/bin/bash"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = model.DefaultTimeout
	}
	return &Executor{
		cfg: cfg,
		identity: executor.RuntimeIdentity{
			Kind:    "command",
			Command: cfg.Command,
			Model:   cfg.Model,
		},
	}, nil
}

// Identity returns the command runtime identity.
func (e *Executor) Identity() executor.RuntimeIdentity { return e.identity }

// Run executes the command once with req.Task on stdin.
func (e *Executor) Run(ctx context.Context, req executor.AgentRunRequest) (executor.AgentRunResult, error) {
	started := time.Now()
	if strings.TrimSpace(req.Workspace) == "" {
		return e.fail(stopAgentError, "agent workspace is required", started)
	}
	if strings.TrimSpace(req.LogDir) == "" {
		return e.fail(stopAgentError, "agent log directory is required", started)
	}
	home := req.HomeDir
	if home == "" {
		home = filepath.Join(req.Workspace, "home")
	}
	if err := os.MkdirAll(req.LogDir, 0o700); err != nil {
		return e.fail(stopAgentError, fmt.Sprintf("create log directory: %v", err), started)
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return e.fail(stopAgentError, fmt.Sprintf("create agent home: %v", err), started)
	}

	cleanupAuth, err := prepareAuth(e.cfg.Auth, home)
	if err != nil {
		res, _ := e.fail(stopCredentialError, err.Error(), started)
		return res, err
	}
	cleanupOnce := sync.Once{}
	cleanup := func() { cleanupOnce.Do(cleanupAuth) }
	defer cleanup()

	logName := req.LogFile
	if logName == "" {
		logName = "agent.log"
		if req.Kind == model.KindJudge {
			logName = "judge.log"
		}
	}
	logPath := filepath.Join(req.LogDir, filepath.Base(logName))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return e.fail(stopAgentError, fmt.Sprintf("create agent log: %v", err), started)
	}

	timeout := req.Limits.Timeout
	if timeout <= 0 {
		timeout = e.cfg.Timeout
	}
	runCtx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	res, runErr := execx.Run(runCtx, execx.Spec{
		Path:       e.cfg.ShellPath,
		Args:       []string{"-c", e.cfg.Command},
		Dir:        req.Workspace,
		Env:        sanitizeEnv(e.parentEnv(), home, e.cfg.PassEnv),
		Stdin:      strings.NewReader(req.Task),
		Stdout:     logFile,
		Stderr:     logFile,
		Grace:      e.cfg.Grace,
		GroupGrace: e.cfg.GroupGrace,
		WaitDelay:  e.cfg.WaitDelay,
	})
	_ = logFile.Close()

	// Credentials must be gone before any post-run hook or evidence capture.
	cleanup()

	out := e.mapResult(res, runErr, started)
	if !res.Started {
		out.Usage = model.NotApplicable("command")
		return out, runErr
	}

	out.Usage = e.usage(req, logPath, res.ExitCode)
	return out, nil
}

func (e *Executor) parentEnv() []string {
	if e.cfg.ParentEnv != nil {
		return e.cfg.ParentEnv
	}
	return os.Environ()
}

func (e *Executor) usage(req executor.AgentRunRequest, logPath string, exitCode int) model.Usage {
	var stderr io.Writer = io.Discard
	var usageLog *os.File
	if e.cfg.UsageCmd != "" {
		if f, err := os.OpenFile(filepath.Join(req.LogDir, "usage.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			usageLog = f
			stderr = f
			defer f.Close()
		}
	}
	kind := string(req.Kind)
	if kind == "" {
		kind = "agent"
	}
	u := usage.Parse(usage.Options{
		LogPath:   logPath,
		UsageCmd:  e.cfg.UsageCmd,
		RunDir:    req.LogDir,
		Sandbox:   req.Workspace,
		Arm:       req.Arm,
		Run:       strconv.Itoa(req.Run),
		AgentExit: strconv.Itoa(exitCode),
		Kind:      kind,
		Stderr:    stderr,
	})
	if usageLog != nil {
		_ = usageLog.Sync()
	}
	return e.cfg.Pricing.Apply(e.cfg.Model, u)
}

func (e *Executor) mapResult(res execx.Result, runErr error, started time.Time) executor.AgentRunResult {
	out := executor.AgentRunResult{
		TimedOut:  res.TimedOut,
		Cancelled: res.Cancelled,
		Duration:  time.Since(started),
	}
	if res.Started {
		code := res.ExitCode
		out.ExitCode = &code
	}

	switch {
	case !res.Started:
		switch {
		case errors.Is(runErr, context.DeadlineExceeded):
			out.Status = model.StatusTimedOut
			out.StopReason = model.StopTimeout
			out.TimedOut = true
		case errors.Is(runErr, context.Canceled):
			out.Status = model.StatusCancelled
			out.StopReason = model.StopCancelled
			out.Cancelled = true
		default:
			out.Status = model.StatusAgentFailed
			out.StopReason = stopAgentError
		}
		if runErr != nil {
			out.Error = runErr.Error()
		} else if res.StartError != "" {
			out.Error = res.StartError
		} else {
			out.Error = "command did not start"
		}
	case res.TimedOut || res.ExitCode == 124 || res.ExitCode == 143:
		out.Status = model.StatusTimedOut
		out.StopReason = model.StopTimeout
		out.TimedOut = true
		if out.ExitCode == nil {
			code := 124
			out.ExitCode = &code
		}
		if runErr != nil {
			out.Error = runErr.Error()
		}
	case res.Cancelled:
		out.Status = model.StatusCancelled
		out.StopReason = model.StopCancelled
		out.Cancelled = true
		if runErr != nil {
			out.Error = runErr.Error()
		}
	case res.ExitCode == 0:
		out.Status = model.StatusOK
		out.StopReason = model.StopCompleted
	default:
		out.Status = model.StatusAgentFailed
		out.StopReason = stopAgentError
		if runErr != nil {
			out.Error = runErr.Error()
		} else {
			out.Error = fmt.Sprintf("agent exited with status %d", res.ExitCode)
		}
	}
	return out
}

func (e *Executor) fail(stopReason, message string, started time.Time) (executor.AgentRunResult, error) {
	return executor.AgentRunResult{
		Status:     model.StatusAgentFailed,
		StopReason: stopReason,
		Error:      message,
		Usage:      model.NotApplicable("command"),
		Duration:   time.Since(started),
	}, nil
}

// sanitizeEnv keeps only the documented variables and removes credential-like
// names. This prevents accidental API-key propagation; it is not isolation.
func sanitizeEnv(base []string, home string, passEnv []string) []string {
	allowed := map[string]bool{
		"PATH": true, "LANG": true, "LC_ALL": true, "LC_CTYPE": true,
		"TMPDIR": true, "TZ": true, "USER": true, "LOGNAME": true,
		"SHELL": true,
	}
	for _, name := range passEnv {
		allowed[name] = true
	}

	values := make(map[string]string, len(base)+3)
	for _, entry := range base {
		i := strings.IndexByte(entry, '=')
		if i <= 0 {
			continue
		}
		name, value := entry[:i], entry[i+1:]
		if deniedEnv(name) {
			continue
		}
		if allowed[name] || strings.HasPrefix(name, "EVAL_") {
			values[name] = value
		}
	}
	values["HOME"] = home
	values["TERM"] = "dumb"
	if _, ok := values["PATH"]; !ok {
		values["PATH"] = "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	}

	out := make([]string, 0, len(values))
	for name, value := range values {
		out = append(out, name+"="+value)
	}
	return out
}

func deniedEnv(name string) bool {
	switch {
	case strings.HasPrefix(name, "OPENAI_"),
		strings.HasPrefix(name, "EVALCTL_"),
		strings.HasSuffix(name, "_KEY"),
		strings.HasSuffix(name, "_TOKEN"),
		strings.Contains(name, "_SECRET"),
		strings.Contains(name, "_PASSWORD"):
		return true
	default:
		return false
	}
}
