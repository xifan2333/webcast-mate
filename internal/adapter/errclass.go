package adapter

import "errors"

// Shared error classes for CLI exit codes (SPEC §5.5). Platform packages wrap
// these with %w (optionally with a platform prefix in the message).
var (
	ErrNotConfigured  = errors.New("not configured")
	ErrNotLoggedIn    = errors.New("not logged in")
	ErrAuthTimeout    = errors.New("auth timeout") // qr / face / cas poll
	ErrNotImplemented = errors.New("not implemented")
	ErrNetwork        = errors.New("network/upstream api error")
	ErrRiskControl    = errors.New("risk control / verification failed")
	ErrWriteConfig    = errors.New("write config failed")
)

// ExitCode maps err to process exit code (contract with cmd/).
//
//	0 ok, 1 other, 2 config, 3 auth session, 4 network/upstream,
//	5 interactive auth timeout, 6 risk/verification, 10 write conf.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	switch {
	case errors.Is(err, ErrNotConfigured):
		return 2
	case errors.Is(err, ErrNotLoggedIn):
		return 3
	case errors.Is(err, ErrNetwork):
		return 4
	case errors.Is(err, ErrAuthTimeout):
		return 5
	case errors.Is(err, ErrRiskControl):
		return 6
	case errors.Is(err, ErrWriteConfig):
		return 10
	default:
		return 1
	}
}
