package main

import "testing"

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
