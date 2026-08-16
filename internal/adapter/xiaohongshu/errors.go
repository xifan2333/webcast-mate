package xiaohongshu

import "errors"

var (
	ErrNotConfigured = errors.New("xiaohongshu not configured")
	ErrNotLoggedIn   = errors.New("xiaohongshu not logged in")
	ErrQRTimeout     = errors.New("qrcode login timeout")
	ErrQRExpired     = errors.New("qrcode expired")
	ErrNeedSID       = errors.New("xiaohongshu live needs robs sid (SMS login)")
)
