package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunExitCodes(t *testing.T) {
	tests := []struct {
		args []string
		want int
	}{
		{[]string{"version"}, 0},
		{[]string{"run"}, 2},
		{[]string{"suite"}, 2},
		{[]string{"no-such-command"}, 2},
	}
	for _, test := range tests {
		if got := run(test.args); got != test.want {
			t.Errorf("run(%v) = %d, want %d", test.args, got, test.want)
		}
	}
}

func TestRunPrintsCommandErrorsOnce(t *testing.T) {
	var stderr bytes.Buffer
	if code := runWithStderr([]string{"suite"}, &stderr); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	message := stderr.String()
	if !strings.Contains(message, "--agent-cmd is required") {
		t.Fatalf("stderr = %q", message)
	}
	if strings.Count(message, "--agent-cmd is required") != 1 {
		t.Fatalf("error printed more than once: %q", message)
	}

	stderr.Reset()
	if code := runWithStderr([]string{"version"}, &stderr); code != 0 {
		t.Fatalf("version exit code = %d", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("version wrote to stderr: %q", stderr.String())
	}
}
