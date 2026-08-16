package xiaohongshu

import "errors"

var (
	ErrNotConfigured = errors.New("xiaohongshu not configured")
	ErrNotLoggedIn   = errors.New("xiaohongshu not logged in")
	ErrQRTimeout     = errors.New("qrcode login timeout")
	ErrQRExpired     = errors.New("qrcode expired")
)
