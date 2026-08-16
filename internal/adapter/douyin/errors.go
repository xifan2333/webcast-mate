package douyin

import "errors"

var (
	ErrNotConfigured = errors.New("douyin not configured")
	ErrNotLoggedIn   = errors.New("douyin not logged in")
	ErrQRTimeout     = errors.New("qrcode login timeout")
	ErrQRExpired     = errors.New("qrcode expired")
	ErrQRRefused     = errors.New("qrcode login refused")
	ErrABogus        = errors.New("a_bogus generation failed")
)
