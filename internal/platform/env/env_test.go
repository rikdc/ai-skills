package env

import (
	"sort"
	"testing"
)

func TestDenied(t *testing.T) {
	for _, name := range []string{
		"OPENAI_API_KEY", "OPENAI_BASE_URL", "EVALCTL_ROOT", "EVALCTL_PRICING",
		"SOME_KEY", "SOME_TOKEN", "AWS_SECRET_ACCESS_KEY", "DB_PASSWORD",
		"MY_PASSWORD_HASH",
	} {
		if !Denied(name) {
			t.Errorf("Denied(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"PATH", "LANG", "LC_NUMERIC", "TMPDIR", "EVAL_WORKSPACE", "OPENCODE_BASE_URL", "KEYRING", "TOKENIZER"} {
		if Denied(name) {
			t.Errorf("Denied(%q) = true, want false", name)
		}
	}
}

func TestFilter(t *testing.T) {
	base := []string{
		"PATH=/usr/bin:/bin",
		"LANG=C",
		"LC_ALL=",
		"OPENCODE_API_KEY=sk-opencode",
		"GITHUB_TOKEN=ghp-secret",
		"EVALCTL_ROOT=/tmp/root",
		"EVAL_WORKSPACE=/tmp/ws",
		"CUSTOM=https://example.test/?a=b",
		"NOEQUALS",
		"=broken",
	}
	allowed := map[string]bool{"PATH": true, "LANG": true, "LC_ALL": true}
	kept := []string{"EVAL_WORKSPACE=/tmp/ws", "LANG=C", "LC_ALL=", "PATH=/usr/bin:/bin"}

	cases := []struct {
		name    string
		passEnv []string
		want    []string
	}{
		{
			name: "no opt-in strips credentials and unknown names",
			want: kept,
		},
		{
			name:    "opt-in overrides the deny list",
			passEnv: []string{"OPENCODE_API_KEY"},
			want:    append([]string{"OPENCODE_API_KEY=sk-opencode"}, kept...),
		},
		{
			name:    "opt-in is per name and keeps values containing =",
			passEnv: []string{"CUSTOM", "GITHUB_TOKEN", "EVALCTL_ROOT", "UNSET_NAME"},
			want: append([]string{
				"CUSTOM=https://example.test/?a=b",
				"EVALCTL_ROOT=/tmp/root",
				"GITHUB_TOKEN=ghp-secret",
			}, kept...),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := List(Filter(base, allowed, tc.passEnv))
			sort.Strings(got)
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if len(got) != len(want) {
				t.Fatalf("env = %q, want %q", got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("env = %q, want %q", got, want)
				}
			}
		})
	}
}

func TestFilterLastValueWins(t *testing.T) {
	got := Filter([]string{"LANG=C", "LANG=en_US.UTF-8"}, map[string]bool{"LANG": true}, nil)
	if value := got["LANG"]; value != "en_US.UTF-8" {
		t.Fatalf("LANG = %q, want last value", value)
	}
}
