package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func makeRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tessl.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func envMap(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestResolveRootFlagWins(t *testing.T) {
	root := makeRoot(t)
	explicit := t.TempDir()
	got, err := ResolveRoot(explicit, true, root, envMap(map[string]string{"EVALCTL_ROOT": root}))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(explicit)
	if got != want {
		t.Fatalf("root = %q, want %q", got, want)
	}
}

func TestResolveRootEnvWinsOverDiscovery(t *testing.T) {
	discovered := makeRoot(t)
	envRoot := makeRoot(t)
	got, err := ResolveRoot("", false, discovered, envMap(map[string]string{"EVALCTL_ROOT": envRoot}))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(envRoot)
	if got != want {
		t.Fatalf("root = %q, want %q", got, want)
	}
}

func TestResolveRootDiscoversNearestAncestor(t *testing.T) {
	root := makeRoot(t)
	nested := filepath.Join(root, "plugins", "plugin", "evals", "scenario")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveRoot("", false, nested, envMap(nil))
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("root = %q, want %q", got, root)
	}
}

func TestResolveRootNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := ResolveRoot("", false, dir, envMap(nil))
	if !errors.Is(err, ErrRootNotFound) {
		t.Fatalf("err = %v, want ErrRootNotFound", err)
	}
}

func TestResolveRootExplicitMissingOrFile(t *testing.T) {
	if _, err := ResolveRoot(filepath.Join(t.TempDir(), "missing"), true, t.TempDir(), envMap(nil)); err == nil {
		t.Fatal("missing explicit root accepted")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveRoot(file, true, t.TempDir(), envMap(nil)); err == nil {
		t.Fatal("file root accepted")
	}
}

func TestResolveRootEmptyFlag(t *testing.T) {
	if _, err := ResolveRoot("", true, t.TempDir(), envMap(nil)); err == nil {
		t.Fatal("empty flag root accepted")
	}
}

func TestResolveStringPrecedence(t *testing.T) {
	env := envMap(map[string]string{"A": "", "B": "env-b"})
	if got := ResolveString("flag", true, "def", env, "A", "B"); got != "flag" {
		t.Fatalf("flag precedence = %q", got)
	}
	if got := ResolveString("", false, "def", env, "A", "B"); got != "env-b" {
		t.Fatalf("env precedence = %q", got)
	}
	if got := ResolveString("", false, "def", env, "A", "C"); got != "def" {
		t.Fatalf("default = %q", got)
	}
}

func TestResolveBoolPrecedence(t *testing.T) {
	env := envMap(map[string]string{"DEBUG": "1"})
	if got := ResolveBool(false, true, true, env, "DEBUG"); got != false {
		t.Fatalf("flag precedence = %v", got)
	}
	if got := ResolveBool(false, false, false, env, "DEBUG"); got != true {
		t.Fatalf("env precedence = %v", got)
	}
	if got := ResolveBool(false, false, true, envMap(map[string]string{"DEBUG": "not-a-bool"}), "DEBUG"); got != true {
		t.Fatalf("invalid env should use default, got %v", got)
	}
	if got := ResolveBool(false, false, false, envMap(map[string]string{"OFF": "no"}), "OFF"); got != false {
		t.Fatalf("no should parse false, got %v", got)
	}
}
