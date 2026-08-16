package bilibili

import (
	"fmt"

	"github.com/xifan2333/webcast-mate/internal/adapter"
)

var (
	ErrNotConfigured = fmt.Errorf("bilibili not configured: %w", adapter.ErrNotConfigured)
	ErrNotLoggedIn   = fmt.Errorf("bilibili not logged in: %w", adapter.ErrNotLoggedIn)
	ErrQRExpired     = fmt.Errorf("qrcode expired: %w", adapter.ErrAuthTimeout)
	ErrQRTimeout     = fmt.Errorf("qrcode login timeout: %w", adapter.ErrAuthTimeout)
	ErrFaceTimeout   = fmt.Errorf("face auth timeout: %w", adapter.ErrAuthTimeout)
)
