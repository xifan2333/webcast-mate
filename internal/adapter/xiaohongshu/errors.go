package xiaohongshu

import (
	"errors"
	"fmt"

	"github.com/xifan2333/webcast-mate/internal/adapter"
)

var (
	ErrNotConfigured = fmt.Errorf("xiaohongshu not configured: %w", adapter.ErrNotConfigured)
	ErrNotLoggedIn   = fmt.Errorf("xiaohongshu not logged in: %w", adapter.ErrNotLoggedIn)
	ErrQRTimeout     = fmt.Errorf("qrcode login timeout: %w", adapter.ErrAuthTimeout)
	ErrQRExpired     = fmt.Errorf("qrcode expired: %w", adapter.ErrAuthTimeout)
	ErrStartDenied   = errors.New("before/start denied")
)
