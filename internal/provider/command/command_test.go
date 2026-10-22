package command

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/rikdc/ai-skills/internal/executor"
	"github.com/rikdc/ai-skills/internal/model"
	"github.com/rikdc/ai-skills/internal/platform/errs"
	"github.com/rikdc/ai-skills/internal/usage"
)

func setup(t *testing.T) (workspace, home, logDir string) {
	t.Helper()
	dir := t.TempDir()
	workspace = filepath.Join(dir, "workspace")
	home = filepath.Join(workspace, "home")
	logDir = filepath.Join(dir, "logs")
	for _, d := range []string{workspace, home, logDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return workspace, home, logDir
}

func newExecutor(t *testing.T, cfg Config) *Executor {
	t.Helper()
	e, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func runString(t *testing.T, e *Executor, req executor.AgentRunRequest) executor.AgentRunResult {
	t.Helper()
	res, err := e.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return res
}

func TestStdinCwdAndHome(t *testing.T) {
	workspace, home, logDir := setup(t)
	e := newExecutor(t, Config{Command: `cat > task.txt; pwd > cwd.txt; printf '%s' "$HOME" > home.txt; exit 0`})
	res := runString(t, e, executor.AgentRunRequest{
		Task: "hello from task", Workspace: workspace, HomeDir: home, LogDir: logDir,
	})
	if res.Status != model.StatusOK || res.StopReason != model.StopCompleted {
		t.Fatalf("result: %+v", res)
	}
	if got := readFile(t, filepath.Join(workspace, "task.txt")); got != "hello from task" {
		t.Fatalf("stdin: %q", got)
	}
	gotCwd := strings.TrimSpace(readFile(t, filepath.Join(workspace, "cwd.txt")))
	wantCwd, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	gotCwdResolved, err := filepath.EvalSymlinks(gotCwd)
	if err != nil {
		t.Fatal(err)
	}
	if gotCwdResolved != wantCwd {
		t.Fatalf("cwd: %q want %q", gotCwd, workspace)
	}
	if got := readFile(t, filepath.Join(workspace, "home.txt")); got != home {
		t.Fatalf("HOME: %q want %q", got, home)
	}
}

func TestEnvironmentSanitized(t *testing.T) {
	workspace, home, logDir := setup(t)
	parent := []string{
		"PATH=/usr/bin:/bin",
		"HOME=/real/home",
		"OPENAI_API_KEY=sk-secret",
		"ANTHROPIC_API_KEY=sk-ant",
		"GITHUB_TOKEN=ghp-secret",
		"AWS_SECRET_ACCESS_KEY=aws-secret",
		"DB_PASSWORD=hunter2",
		"EVALCTL_ROOT=/tmp/root",
		"EVAL_HOOK_SOURCE=/tmp/hook",
		"LANG=C",
		"TERM=xterm-256color",
		"CUSTOM_VARIABLE=kept-only-if-passed",
	}
	e := newExecutor(t, Config{Command: `env > env.txt`, ParentEnv: parent})
	runString(t, e, executor.AgentRunRequest{
		Task: "task", Workspace: workspace, HomeDir: home, LogDir: logDir,
	})
	env := readFile(t, filepath.Join(workspace, "env.txt"))
	for _, leaked := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GITHUB_TOKEN", "AWS_SECRET_ACCESS_KEY", "DB_PASSWORD", "EVALCTL_ROOT", "CUSTOM_VARIABLE"} {
		if strings.Contains(env, leaked+"=") {
			t.Fatalf("%s leaked into the agent environment:\n%s", leaked, env)
		}
	}
	for _, kept := range []string{"PATH=/usr/bin:/bin", "HOME=" + home, "TERM=dumb", "LANG=C", "EVAL_HOOK_SOURCE=/tmp/hook"} {
		if !strings.Contains(env, kept+"\n") {
			t.Fatalf("%s missing from environment:\n%s", kept, env)
		}
	}
}

func TestPassEnvOptIn(t *testing.T) {
	workspace, home, logDir := setup(t)
	e := newExecutor(t, Config{
		Command:   `printf '%s' "$CUSTOM_VARIABLE" > custom.txt`,
		ParentEnv: []string{"PATH=/usr/bin:/bin", "CUSTOM_VARIABLE=opted-in"},
		PassEnv:   []string{"CUSTOM_VARIABLE"},
	})
	runString(t, e, executor.AgentRunRequest{Task: "task", Workspace: workspace, HomeDir: home, LogDir: logDir})
	if got := readFile(t, filepath.Join(workspace, "custom.txt")); got != "opted-in" {
		t.Fatalf("pass-env: %q", got)
	}
}

// TestPassEnvOverridesCredentialDenyList pins the precedence rule: an explicit
// pass-env name wins over the credential deny list for both the agent and the
// judge, and every name that was not opted in is still stripped.
func TestPassEnvOverridesCredentialDenyList(t *testing.T) {
	parent := []string{
		"PATH=/usr/bin:/bin",
		"OPENCODE_API_KEY=sk-opencode-secret",
		"GITHUB_TOKEN=ghp-secret",
		"EVALCTL_SESSION=internal",
	}
	cases := []struct {
		name    string
		kind    model.AttemptKind
		passEnv []string
		want    []string
		absent  []string
	}{
		{
			name:   "agent denies credentials by default",
			kind:   model.KindAgent,
			absent: []string{"OPENCODE_API_KEY", "GITHUB_TOKEN", "EVALCTL_SESSION"},
		},
		{
			name:    "agent passes the opted-in key only",
			kind:    model.KindAgent,
			passEnv: []string{"OPENCODE_API_KEY"},
			want:    []string{"OPENCODE_API_KEY=sk-opencode-secret"},
			absent:  []string{"GITHUB_TOKEN", "EVALCTL_SESSION"},
		},
		{
			name:    "judge passes every opted-in credential",
			kind:    model.KindJudge,
			passEnv: []string{"OPENCODE_API_KEY", "GITHUB_TOKEN", "EVALCTL_SESSION"},
			want: []string{
				"OPENCODE_API_KEY=sk-opencode-secret",
				"GITHUB_TOKEN=ghp-secret",
				"EVALCTL_SESSION=internal",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workspace, home, logDir := setup(t)
			e := newExecutor(t, Config{
				Command:   `env > env.txt`,
				ParentEnv: parent,
				PassEnv:   tc.passEnv,
			})
			runString(t, e, executor.AgentRunRequest{
				Task: "task", Kind: tc.kind, Workspace: workspace, HomeDir: home, LogDir: logDir,
			})
			env := readFile(t, filepath.Join(workspace, "env.txt"))
			for _, want := range tc.want {
				if !hasEnvEntry(env, want) {
					t.Fatalf("%s missing from environment:\n%s", want, env)
				}
			}
			for _, name := range tc.absent {
				if hasEnvEntry(env, name+"=") {
					t.Fatalf("%s leaked into the environment:\n%s", name, env)
				}
			}
		})
	}
}

func hasEnvEntry(env, entry string) bool {
	for _, line := range strings.Split(env, "\n") {
		if strings.HasPrefix(line, entry) {
			return true
		}
	}
	return false
}

func TestStatusMatrix(t *testing.T) {
	cases := []struct {
		name       string
		command    string
		shell      string
		wantStatus model.RunStatus
		wantStop   string
		wantExit   any
	}{
		{"success", "exit 0", "", model.StatusOK, model.StopCompleted, 0},
		{"failure", "exit 1", "", model.StatusAgentFailed, stopAgentError, 1},
		{"timeout code", "exit 124", "", model.StatusTimedOut, model.StopTimeout, 124},
		{"term code", "exit 143", "", model.StatusTimedOut, model.StopTimeout, 143},
		{"start failure", "true", filepath.Join(t.TempDir(), "missing-shell"), model.StatusAgentFailed, stopAgentError, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workspace, home, logDir := setup(t)
			cfg := Config{Command: tc.command}
			if tc.shell != "" {
				cfg.ShellPath = tc.shell
			}
			e := newExecutor(t, cfg)
			res, _ := e.Run(context.Background(), executor.AgentRunRequest{
				Task: "task", Workspace: workspace, HomeDir: home, LogDir: logDir,
			})
			if res.Status != tc.wantStatus || res.StopReason != tc.wantStop {
				t.Fatalf("result: %+v", res)
			}
			switch want := tc.wantExit.(type) {
			case int:
				if res.ExitCode == nil || *res.ExitCode != want {
					t.Fatalf("exit: %v want %d", res.ExitCode, want)
				}
			case nil:
				if res.ExitCode != nil {
					t.Fatalf("exit: %v want nil", *res.ExitCode)
				}
			}
		})
	}
}

func TestFailureDetailPrefersStderr(t *testing.T) {
	workspace, home, logDir := setup(t)
	e := newExecutor(t, Config{Command: `echo stdout-detail; echo stderr-detail >&2; exit 3`})
	res, err := e.Run(context.Background(), executor.AgentRunRequest{
		Task: "task", Workspace: workspace, HomeDir: home, LogDir: logDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != model.StatusAgentFailed {
		t.Fatalf("status = %s", res.Status)
	}
	if !strings.Contains(res.Error, "agent exited with status 3") ||
		!strings.Contains(res.Error, "stderr-detail") {
		t.Fatalf("error = %q", res.Error)
	}
	if strings.Contains(res.Error, "stdout-detail") {
		t.Fatalf("stdout must not win over stderr: %q", res.Error)
	}
}

func TestFailureDetailFallsBackToStdoutAndIsBounded(t *testing.T) {
	workspace, home, logDir := setup(t)
	long := strings.Repeat("z", 800)
	e := newExecutor(t, Config{Command: `printf '%s\n' "` + long + `"; exit 1`})
	res, err := e.Run(context.Background(), executor.AgentRunRequest{
		Task: "task", Workspace: workspace, HomeDir: home, LogDir: logDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(res.Error, "…") {
		t.Fatalf("error must be truncated: %q", res.Error)
	}
	if got := strings.Count(res.Error, "z"); got != maxFailureDetail {
		t.Fatalf("kept %d characters, want %d: %q", got, maxFailureDetail, res.Error)
	}
}

func TestDeadline(t *testing.T) {
	workspace, home, logDir := setup(t)
	e := newExecutor(t, Config{Command: "sleep 30"})
	start := time.Now()
	res, err := e.Run(context.Background(), executor.AgentRunRequest{
		Task: "task", Workspace: workspace, HomeDir: home, LogDir: logDir,
		Limits: model.Limits{Timeout: 100 * time.Millisecond},
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != model.StatusTimedOut || res.StopReason != model.StopTimeout || !res.TimedOut {
		t.Fatalf("result: %+v", res)
	}
	if res.ExitCode == nil || *res.ExitCode != 124 {
		t.Fatalf("exit: %v", res.ExitCode)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("deadline took %v", elapsed)
	}
}

func TestLogFileAndKind(t *testing.T) {
	workspace, home, logDir := setup(t)
	e := newExecutor(t, Config{Command: `echo stdout-line; echo stderr-line >&2`})
	runString(t, e, executor.AgentRunRequest{
		Task: "task", Workspace: workspace, HomeDir: home, LogDir: logDir,
	})
	log := readFile(t, filepath.Join(logDir, "agent.log"))
	if !strings.Contains(log, "stdout-line") || !strings.Contains(log, "stderr-line") {
		t.Fatalf("log: %q", log)
	}
	info, err := os.Stat(filepath.Join(logDir, "agent.log"))
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("log mode: %o", mode)
	}

	workspace2, home2, logDir2 := setup(t)
	runString(t, e, executor.AgentRunRequest{
		Task: "task", Workspace: workspace2, HomeDir: home2, LogDir: logDir2,
		Kind: model.KindJudge,
	})
	if _, err := os.Stat(filepath.Join(logDir2, "judge.log")); err != nil {
		t.Fatalf("judge log: %v", err)
	}
}

func TestCredentialsCopiedAndRemoved(t *testing.T) {
	workspace, home, logDir := setup(t)
	realHome := t.TempDir()
	write(t, filepath.Join(realHome, ".claude.json"), "claude-config")
	write(t, filepath.Join(realHome, ".claude", ".credentials.json"), "credentials")

	e := newExecutor(t, Config{
		Command: `cat "$HOME/.claude.json" > got.txt; cat "$HOME/.claude/.credentials.json" > cred.txt`,
		Auth:    AuthOptions{CLI: true, RealHome: realHome},
	})
	runString(t, e, executor.AgentRunRequest{Task: "task", Workspace: workspace, HomeDir: home, LogDir: logDir})

	if got := readFile(t, filepath.Join(workspace, "got.txt")); got != "claude-config" {
		t.Fatalf("copied config: %q", got)
	}
	if got := readFile(t, filepath.Join(workspace, "cred.txt")); got != "credentials" {
		t.Fatalf("copied credentials: %q", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("copied credential was not removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", ".credentials.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("copied credential was not removed: %v", err)
	}
	// The source files are untouched.
	if got := readFile(t, filepath.Join(realHome, ".claude.json")); got != "claude-config" {
		t.Fatalf("source changed: %q", got)
	}
}

// TestAuthPathsCopyCustomCredential checks that a --auth-path file reaches the
// agent home and is removed with the rest of the copied credentials.
func TestAuthPathsCopyCustomCredential(t *testing.T) {
	workspace, home, logDir := setup(t)
	realHome := t.TempDir()
	write(t, filepath.Join(realHome, ".agent-auth"), "secret")

	e := newExecutor(t, Config{
		Command: `cat "$HOME/.agent-auth" > got.txt`,
		Auth:    AuthOptions{CLI: true, Paths: []string{".agent-auth"}, RealHome: realHome},
	})
	runString(t, e, executor.AgentRunRequest{Task: "task", Workspace: workspace, HomeDir: home, LogDir: logDir})

	if got := readFile(t, filepath.Join(workspace, "got.txt")); got != "secret" {
		t.Fatalf("custom --auth-path did not reach the agent: %q", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".agent-auth")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("copied credential was not removed: %v", err)
	}
}

// TestCredentialCopyFailureNamesSource checks that an unreadable credential
// source aborts the run before the agent starts, with an error naming the
// source.
func TestCredentialCopyFailureNamesSource(t *testing.T) {
	workspace, home, logDir := setup(t)
	realHome := t.TempDir()
	// A self-referential symlink is unreadable for any user, unlike a 000-mode
	// file which root ignores; it exercises the same Stat error branch.
	src := filepath.Join(realHome, ".claude.json")
	if err := os.Symlink(src, src); err != nil {
		t.Fatal(err)
	}

	e := newExecutor(t, Config{Command: "true", Auth: AuthOptions{CLI: true, RealHome: realHome}})
	res, err := e.Run(context.Background(), executor.AgentRunRequest{
		Task: "task", Workspace: workspace, HomeDir: home, LogDir: logDir,
	})
	if err == nil || errs.Code(err) != errs.CodeOperational {
		t.Fatalf("credential copy error = %v, want an operational error", err)
	}
	if !strings.Contains(err.Error(), "cannot copy agent credentials from "+src) {
		t.Fatalf("error must name the source: %v", err)
	}
	if res.StopReason != stopCredentialError {
		t.Fatalf("stop reason = %q, want %q", res.StopReason, stopCredentialError)
	}
	if _, statErr := os.Stat(filepath.Join(logDir, "agent.log")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("the agent started despite the credential failure: %v", statErr)
	}
}

func TestKeychainSymlinkAndCleanup(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("keychain is macOS-only")
	}
	workspace, home, logDir := setup(t)
	realHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(realHome, "Library", "Keychains"), 0o700); err != nil {
		t.Fatal(err)
	}
	e := newExecutor(t, Config{
		Command: `test -L "$HOME/Library/Keychains" && echo linked > keychain.txt`,
		Auth:    AuthOptions{CLI: true, Keychain: true, RealHome: realHome},
	})
	runString(t, e, executor.AgentRunRequest{Task: "task", Workspace: workspace, HomeDir: home, LogDir: logDir})
	if got := strings.TrimSpace(readFile(t, filepath.Join(workspace, "keychain.txt"))); got != "linked" {
		t.Fatalf("keychain link: %q", got)
	}
	if _, err := os.Lstat(filepath.Join(home, "Library", "Keychains")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("keychain link was not removed: %v", err)
	}
}

func TestNoRealHomeWritesByDefault(t *testing.T) {
	workspace, home, logDir := setup(t)
	realHome := t.TempDir()
	e := newExecutor(t, Config{
		Command:   `printf bad > "$HOME/should-not-exist"`,
		ParentEnv: []string{"PATH=/usr/bin:/bin", "HOME=" + realHome},
	})
	runString(t, e, executor.AgentRunRequest{Task: "task", Workspace: workspace, HomeDir: home, LogDir: logDir})
	if _, err := os.Stat(filepath.Join(realHome, "should-not-exist")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("real HOME was written: %v", err)
	}
	if got := readFile(t, filepath.Join(home, "should-not-exist")); got != "bad" {
		t.Fatalf("workspace HOME: %q", got)
	}
}

func TestAuthValidation(t *testing.T) {
	cases := []struct {
		name string
		auth AuthOptions
	}{
		{"keychain without auth-cli", AuthOptions{Keychain: true}},
		{"auth-path without auth-cli", AuthOptions{Paths: []string{".config/test"}}},
		{"absolute auth-path", AuthOptions{CLI: true, Paths: []string{"/etc/passwd"}}},
		{"escaping auth-path", AuthOptions{CLI: true, Paths: []string{"../secret"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(Config{Command: "true", Auth: tc.auth})
			if err == nil {
				t.Fatal("expected validation error")
			}
			if code := errs.Code(err); code != errs.CodeUsage {
				t.Fatalf("exit code %d, want 2: %v", code, err)
			}
		})
	}
	if runtime.GOOS != "darwin" {
		_, err := New(Config{Command: "true", Auth: AuthOptions{CLI: true, Keychain: true}})
		if err == nil || errs.Code(err) != errs.CodeUsage {
			t.Fatalf("non-macOS keychain: %v", err)
		}
	}
}

func TestMissingCommand(t *testing.T) {
	if _, err := New(Config{}); err == nil || errs.Code(err) != errs.CodeUsage {
		t.Fatalf("missing command: %v", err)
	}
}

func TestIdentity(t *testing.T) {
	e := newExecutor(t, Config{Command: "true", Model: "command-model"})
	id := e.Identity()
	if id.Kind != "command" || id.Command != "true" || id.Model != "command-model" {
		t.Fatalf("identity: %+v", id)
	}
}

func TestUsageParsedFromLog(t *testing.T) {
	workspace, home, logDir := setup(t)
	e := newExecutor(t, Config{Command: `printf '%s\n' '{"type":"step_finish","part":{"tokens":{"input":10,"output":2,"total":12},"cost":0.01}}'`})
	res := runString(t, e, executor.AgentRunRequest{Task: "task", Workspace: workspace, HomeDir: home, LogDir: logDir})
	if !res.Usage.Known || res.Usage.Source != usage.SourceOpencode {
		t.Fatalf("usage: %+v", res.Usage)
	}
	if res.Usage.InputTokens == nil || *res.Usage.InputTokens != 10 {
		t.Fatalf("input: %v", res.Usage.InputTokens)
	}
}

func TestUsageHookErrorLogged(t *testing.T) {
	workspace, home, logDir := setup(t)
	e := newExecutor(t, Config{
		Command:  `printf '%s\n' '{"type":"step_finish","part":{"tokens":{"input":10}}}'`,
		UsageCmd: `echo hook-diagnostic >&2; exit 3`,
	})
	res := runString(t, e, executor.AgentRunRequest{Task: "task", Workspace: workspace, HomeDir: home, LogDir: logDir})
	if !res.Usage.Known || res.Usage.Source != usage.SourceOpencode {
		t.Fatalf("usage: %+v", res.Usage)
	}
	log := readFile(t, filepath.Join(logDir, "usage.log"))
	if !strings.Contains(log, "usage command failed") || !strings.Contains(log, "hook-diagnostic") {
		t.Fatalf("usage.log: %q", log)
	}
}

func TestPreStartCancellation(t *testing.T) {
	workspace, home, logDir := setup(t)
	e := newExecutor(t, Config{Command: "sleep 30"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := e.Run(ctx, executor.AgentRunRequest{Task: "task", Workspace: workspace, HomeDir: home, LogDir: logDir})
	if err == nil {
		t.Fatal("expected an error")
	}
	if res.Status != model.StatusCancelled || res.StopReason != model.StopCancelled {
		t.Fatalf("result: %+v", res)
	}
}

func TestCancelAfterStart(t *testing.T) {
	workspace, home, logDir := setup(t)
	e := newExecutor(t, Config{Command: "sleep 30"})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	res, _ := e.Run(ctx, executor.AgentRunRequest{Task: "task", Workspace: workspace, HomeDir: home, LogDir: logDir})
	elapsed := time.Since(start)
	cancel()
	if res.Status != model.StatusCancelled || res.StopReason != model.StopCancelled || !res.Cancelled {
		t.Fatalf("result: %+v", res)
	}
	if res.ExitCode == nil || *res.ExitCode != 130 {
		t.Fatalf("exit: %v", res.ExitCode)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("cancellation took %v", elapsed)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
