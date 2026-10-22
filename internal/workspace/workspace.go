// Package workspace owns the temporary run workspace: fixture staging, hook
// source staging, unit installation, snapshots, the tar evidence bundle, and
// the sanitized environment setup and checks run in.
//
// The workspace is a working directory, not a sandbox. Fixtures, agents, and
// checks run with the invoking user's privileges.
package workspace

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rikdc/ai-skills/internal/platform/execx"
)

// Workspace is one temporary run workspace.
type Workspace struct {
	Dir     string
	Home    string
	TempDir string

	hookSource string
	passEnv    []string
}

// New creates an empty workspace with a private home and temp directory.
func New(scenarioName string, passEnv []string) (*Workspace, error) {
	prefix := "evalctl-" + sanitize(scenarioName) + "-"
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	ws := &Workspace{
		Dir:     dir,
		Home:    filepath.Join(dir, "home"),
		TempDir: filepath.Join(dir, ".tmp"),
		passEnv: append([]string(nil), passEnv...),
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("chmod workspace: %w", err)
	}
	for _, sub := range []string{ws.Home, ws.TempDir} {
		if err := os.MkdirAll(sub, 0o700); err != nil {
			_ = os.RemoveAll(dir)
			return nil, fmt.Errorf("create workspace directory %s: %w", sub, err)
		}
	}
	return ws, nil
}

// HookSource returns the staged hook source directory, or "".
func (w *Workspace) HookSource() string { return w.hookSource }

// Cleanup removes the workspace. It is idempotent.
func (w *Workspace) Cleanup() error {
	if w == nil || w.Dir == "" {
		return nil
	}
	err := os.RemoveAll(w.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func sanitize(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "run"
	}
	return b.String()
}

// StageIncludes copies every scenario include entry into the workspace root,
// preserving modes. Symlinks that resolve outside the scenario directory are
// rejected.
func (w *Workspace) StageIncludes(scenarioDir string, includes []string) error {
	scenarioDir, err := filepath.Abs(scenarioDir)
	if err != nil {
		return err
	}
	for _, include := range includes {
		if strings.TrimSpace(include) == "" {
			continue
		}
		src := filepath.Join(scenarioDir, include)
		dst := filepath.Join(w.Dir, filepath.Base(filepath.Clean(include)))
		if err := copyEntry(scenarioDir, src, dst); err != nil {
			return fmt.Errorf("stage %s: %w", include, err)
		}
	}
	return nil
}

// StageHookSource writes the with-arm (current plugin source) or without-arm
// (disabled control) hook tree to .eval-plugin. A comparison-mode without arm
// gets same-named disabled hooks so the delta measures the hook.
func (w *Workspace) StageHookSource(pluginDir, arm string) error {
	dest := filepath.Join(w.Dir, ".eval-plugin")
	if err := os.MkdirAll(filepath.Join(dest, "hooks"), 0o700); err != nil {
		return fmt.Errorf("create hook source: %w", err)
	}
	hooksDir := filepath.Join(pluginDir, "hooks")
	entries, err := os.ReadDir(hooksDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read plugin hooks: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		target := filepath.Join(dest, "hooks", name)
		if arm == "with" {
			src := filepath.Join(hooksDir, name)
			if err := copyEntry(pluginDir, src, target); err != nil {
				return fmt.Errorf("stage hook %s: %w", name, err)
			}
			continue
		}
		script := "#!/usr/bin/env bash\n" +
			"# Disabled control hook: accepts every call without scanning or deciding.\n" +
			"cat >/dev/null\nexit 0\n"
		if err := os.WriteFile(target, []byte(script), 0o755); err != nil {
			return fmt.Errorf("write control hook %s: %w", name, err)
		}
	}
	patterns := filepath.Join(pluginDir, "security-patterns.conf")
	if data, err := os.ReadFile(patterns); err == nil {
		if err := os.WriteFile(filepath.Join(dest, "security-patterns.conf"), data, 0o600); err != nil {
			return fmt.Errorf("stage security patterns: %w", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read security patterns: %w", err)
	}
	w.hookSource = dest
	return nil
}

// ValidateSkillDir rejects an absolute or escaping --skill-dir.
func ValidateSkillDir(skillDir string) error {
	if strings.TrimSpace(skillDir) == "" {
		return errors.New("--skill-dir must not be empty")
	}
	if filepath.IsAbs(skillDir) {
		return errors.New("--skill-dir must be relative to the workspace")
	}
	if !filepath.IsLocal(skillDir) {
		return fmt.Errorf("--skill-dir must stay inside the workspace: %s", skillDir)
	}
	return nil
}

// InstallUnits installs the selected plugin units for the with arm. With no
// units it installs every skill and command the plugin ships. Hook units are
// staged separately as hook source.
func (w *Workspace) InstallUnits(pluginDir, plugin, skillDir string, units []string) error {
	if err := ValidateSkillDir(skillDir); err != nil {
		return err
	}
	commandDir := filepath.Join(filepath.Dir(skillDir), "commands", plugin)
	if len(units) == 0 {
		return w.installAll(pluginDir, skillDir, commandDir)
	}
	for _, unit := range units {
		kind, name, ok := splitUnit(unit)
		if !ok {
			return fmt.Errorf("invalid unit %q", unit)
		}
		switch kind {
		case "skills":
			if err := copyEntry(pluginDir, filepath.Join(pluginDir, "skills", name), filepath.Join(w.Dir, skillDir, name)); err != nil {
				return fmt.Errorf("install skill %s: %w", name, err)
			}
		case "commands":
			src := filepath.Join(pluginDir, "commands", name+".md")
			if err := copyEntry(pluginDir, src, filepath.Join(w.Dir, commandDir, name+".md")); err != nil {
				return fmt.Errorf("install command %s: %w", name, err)
			}
		case "hooks":
			// The fixture invokes the hook; the source is staged separately.
		}
	}
	return nil
}

func (w *Workspace) installAll(pluginDir, skillDir, commandDir string) error {
	skillsDir := filepath.Join(pluginDir, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read plugin skills: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		src := filepath.Join(skillsDir, entry.Name())
		dst := filepath.Join(w.Dir, skillDir, entry.Name())
		if err := copyEntry(pluginDir, src, dst); err != nil {
			return fmt.Errorf("install skill %s: %w", entry.Name(), err)
		}
	}
	commandsDir := filepath.Join(pluginDir, "commands")
	commands, err := os.ReadDir(commandsDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read plugin commands: %w", err)
	}
	for _, entry := range commands {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		src := filepath.Join(commandsDir, entry.Name())
		dst := filepath.Join(w.Dir, commandDir, entry.Name())
		if err := copyEntry(pluginDir, src, dst); err != nil {
			return fmt.Errorf("install command %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func splitUnit(unit string) (kind, name string, ok bool) {
	index := strings.IndexByte(unit, '/')
	if index <= 0 || index == len(unit)-1 {
		return "", "", false
	}
	return unit[:index], unit[index+1:], true
}

// copyEntry copies one file, directory, or non-escaping symlink. root is the
// directory symlinks must stay under.
func copyEntry(root, src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		if err := checkSymlinkInside(root, filepath.Dir(src), target); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return err
		}
		return os.Symlink(target, dst)
	}
	if info.IsDir() {
		if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if err := copyEntry(root, filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported include entry %s", src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, info.Mode().Perm())
}

func checkSymlinkInside(root, dir, target string) error {
	resolved := target
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(dir, target)
	}
	resolved = filepath.Clean(resolved)
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("symlink %s escapes %s", target, root)
	}
	return nil
}

// SetupEnvironment returns the sanitized environment for setup.sh.
func (w *Workspace) SetupEnvironment(scenarioDir, pluginDir string) []string {
	return w.environment(scenarioDir, pluginDir, "")
}

// ChecksEnvironment returns the environment for checks.sh, including
// SANDBOX/SCENARIO_DIR/CRITERIA.
func (w *Workspace) ChecksEnvironment(scenarioDir, pluginDir, criteria string) []string {
	return w.environment(scenarioDir, pluginDir, criteria)
}

func (w *Workspace) environment(scenarioDir, pluginDir, criteria string) []string {
	allowed := map[string]bool{
		"PATH": true, "LANG": true, "LC_ALL": true, "LC_CTYPE": true, "LC_NUMERIC": true,
		"TZ": true, "USER": true, "LOGNAME": true, "SHELL": true,
	}
	for _, name := range w.passEnv {
		allowed[name] = true
	}
	values := make(map[string]string, len(allowed)+8)
	for _, entry := range os.Environ() {
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
	values["HOME"] = w.Home
	values["TMPDIR"] = w.TempDir
	values["TERM"] = "dumb"
	if _, ok := values["PATH"]; !ok {
		values["PATH"] = "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	}
	values["EVAL_WORKSPACE"] = w.Dir
	values["EVAL_SCENARIO_DIR"] = scenarioDir
	values["EVAL_PLUGIN_DIR"] = pluginDir
	if w.hookSource != "" {
		values["EVAL_HOOK_SOURCE"] = w.hookSource
	}
	if criteria != "" {
		values["SANDBOX"] = w.Dir
		values["SCENARIO_DIR"] = scenarioDir
		values["CRITERIA"] = criteria
	}

	out := make([]string, 0, len(values))
	for name, value := range values {
		out = append(out, name+"="+value)
	}
	sort.Strings(out)
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

// RunSetup executes inputs/setup.sh with the workspace as cwd and the
// workspace home as HOME. stdout/stderr go to logPath.
func (w *Workspace) RunSetup(ctx context.Context, scenarioDir, pluginDir, script, logPath string, timeout time.Duration) (execx.Result, error) {
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return execx.Result{}, fmt.Errorf("create setup log: %w", err)
	}
	defer logFile.Close()

	runCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return execx.Run(runCtx, execx.Spec{
		Path:   "/bin/bash",
		Args:   []string{script},
		Dir:    w.Dir,
		Env:    w.SetupEnvironment(scenarioDir, pluginDir),
		Stdout: logFile,
		Stderr: logFile,
	})
}

// RunChecks executes checks.sh with stdout and stderr captured separately.
func (w *Workspace) RunChecks(ctx context.Context, scenarioDir, pluginDir, criteria, script, stdoutPath, stderrPath string, timeout time.Duration) (execx.Result, error) {
	stdout, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return execx.Result{}, fmt.Errorf("create checks output: %w", err)
	}
	defer stdout.Close()
	stderr, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return execx.Result{}, fmt.Errorf("create checks log: %w", err)
	}
	defer stderr.Close()

	runCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return execx.Run(runCtx, execx.Spec{
		Path:   "/bin/bash",
		Args:   []string{script},
		Dir:    w.Dir,
		Env:    w.ChecksEnvironment(scenarioDir, pluginDir, criteria),
		Stdout: stdout,
		Stderr: stderr,
	})
}

// Snapshot writes the normalized before/after snapshot document.
func (w *Workspace) Snapshot(path string) error {
	var buf bytes.Buffer
	if _, err := exec.Command("git", "-C", w.Dir, "rev-parse", "--git-dir").Output(); err == nil {
		fmt.Fprintln(&buf, "== git log")
		buf.Write(gitOutput(w.Dir, "log", "--oneline", "--all"))
		fmt.Fprintln(&buf)
		fmt.Fprintln(&buf, "== git status")
		buf.Write(gitOutput(w.Dir, "status", "--porcelain"))
		fmt.Fprintln(&buf)
		fmt.Fprintln(&buf, "== staged diff stat")
		buf.Write(gitOutput(w.Dir, "diff", "--cached", "--stat"))
	} else {
		fmt.Fprintln(&buf, "== not a git repository")
	}
	fmt.Fprintln(&buf)
	fmt.Fprintln(&buf, "== files")
	files, err := w.files()
	if err != nil {
		return err
	}
	for _, file := range files {
		fmt.Fprintf(&buf, "./%s\n", filepath.ToSlash(file))
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}

func gitOutput(dir string, args ...string) []byte {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.Stderr
		}
		return nil
	}
	return out
}

func (w *Workspace) files() ([]string, error) {
	var files []string
	err := filepath.WalkDir(w.Dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(w.Dir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "home" || rel == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// Archive writes an uncompressed tar of the workspace to path, excluding
// ./home. Names are prefixed "./". If the payload exceeds maxBytes the archive
// fails before any bytes are renamed into place.
func (w *Workspace) Archive(path string, maxBytes int64) error {
	type entryInfo struct {
		rel  string
		info fs.FileInfo
	}
	var entries []entryInfo
	var total int64
	err := filepath.Walk(w.Dir, func(p string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(w.Dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "home" {
			return filepath.SkipDir
		}
		if rel == "." {
			entries = append(entries, entryInfo{rel: ".", info: info})
			return nil
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		entries = append(entries, entryInfo{rel: rel, info: info})
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk workspace: %w", err)
	}
	if maxBytes > 0 && total > maxBytes {
		return fmt.Errorf("snapshot is %d bytes, exceeding the %d-byte limit", total, maxBytes)
	}

	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".snapshot.tar.tmp-*")
	if err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}
	tempName := temp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tempName)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	tw := tar.NewWriter(temp)
	for _, entry := range entries {
		header, err := tar.FileInfoHeader(entry.info, "")
		if err != nil {
			_ = tw.Close()
			_ = temp.Close()
			return fmt.Errorf("snapshot header %s: %w", entry.rel, err)
		}
		switch {
		case entry.rel == ".":
			header.Name = "./"
			header.Typeflag = tar.TypeDir
		case entry.info.IsDir():
			header.Name = "./" + entry.rel + "/"
		default:
			header.Name = "./" + entry.rel
		}
		if entry.info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(filepath.Join(w.Dir, filepath.FromSlash(entry.rel)))
			if err != nil {
				_ = tw.Close()
				_ = temp.Close()
				return fmt.Errorf("read symlink %s: %w", entry.rel, err)
			}
			header.Linkname = target
		}
		if !entry.info.IsDir() && !entry.info.Mode().IsRegular() && entry.info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if err := tw.WriteHeader(header); err != nil {
			_ = tw.Close()
			_ = temp.Close()
			return fmt.Errorf("snapshot header %s: %w", entry.rel, err)
		}
		if !entry.info.Mode().IsRegular() {
			continue
		}
		src, err := os.Open(filepath.Join(w.Dir, filepath.FromSlash(entry.rel)))
		if err != nil {
			_ = tw.Close()
			_ = temp.Close()
			return fmt.Errorf("snapshot %s: %w", entry.rel, err)
		}
		if _, err := io.Copy(tw, src); err != nil {
			_ = src.Close()
			_ = tw.Close()
			_ = temp.Close()
			return fmt.Errorf("snapshot %s: %w", entry.rel, err)
		}
		_ = src.Close()
	}
	if err := tw.Close(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("close snapshot: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync snapshot: %w", err)
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("rename snapshot: %w", err)
	}
	renamed = true
	return nil
}
