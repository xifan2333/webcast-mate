package adapter

import (
	"errors"
	"fmt"
	"testing"
)

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, 0},
		{"not configured", ErrNotConfigured, 2},
		{"not logged in", ErrNotLoggedIn, 3},
		{"network", ErrNetwork, 4},
		{"auth timeout", ErrAuthTimeout, 5},
		{"risk control", ErrRiskControl, 6},
		{"write config", ErrWriteConfig, 10},
		{"not implemented", ErrNotImplemented, 1},
		{"other", errors.New("boom"), 1},
		{"wrapped risk", fmt.Errorf("douyin: %w", ErrRiskControl), 6},
		{"wrapped config", fmt.Errorf("bili: %w", ErrNotConfigured), 2},
		{"double wrapped", fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", ErrNetwork)), 4},
	}
	for _, c := range cases {
		if got := ExitCode(c.err); got != c.want {
			t.Errorf("ExitCode(%s) = %d, want %d", c.name, got, c.want)
		}
	}
}
