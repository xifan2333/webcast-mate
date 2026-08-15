package bilibili

import "errors"

var (
	ErrNotConfigured = errors.New("bilibili not configured")
	ErrNotLoggedIn   = errors.New("bilibili not logged in")
	ErrQRExpired     = errors.New("qrcode expired")
	ErrQRTimeout     = errors.New("qrcode login timeout")
	ErrFaceTimeout   = errors.New("face auth timeout")
)
