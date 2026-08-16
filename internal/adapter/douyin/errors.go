package douyin

import (
	"errors"
	"fmt"

	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/conv"
)

var (
	ErrNotConfigured = fmt.Errorf("douyin not configured: %w", adapter.ErrNotConfigured)
	ErrNotLoggedIn   = fmt.Errorf("douyin not logged in: %w", adapter.ErrNotLoggedIn)
	ErrQRTimeout     = fmt.Errorf("qrcode login timeout: %w", adapter.ErrAuthTimeout)
	ErrQRExpired     = fmt.Errorf("qrcode expired: %w", adapter.ErrAuthTimeout)
	ErrQRRefused     = errors.New("qrcode login refused")
	ErrQRRateLimit   = errors.New("qrcode rate limited (error_code=7); wait, do not spam")
	ErrABogus        = errors.New("a_bogus generation failed")

	// Create-path classes (companion createRoomResultHandle).
	ErrFaceRequired    = fmt.Errorf("face verification required: %w", adapter.ErrAuthTimeout)
	ErrFaceFailed      = fmt.Errorf("face verification failed: %w", adapter.ErrRiskControl)
	ErrPassportVerify  = fmt.Errorf("passport secondary verify required: %w", adapter.ErrNotLoggedIn)
	ErrLiveAgreement   = fmt.Errorf("live agreement / realname required: %w", adapter.ErrRiskControl)
	ErrAccountBanned   = fmt.Errorf("account banned from living: %w", adapter.ErrRiskControl)
	ErrPaidLiveInvalid = fmt.Errorf("paid-live ticket invalid or delisted: %w", adapter.ErrRiskControl)
	ErrCreateBlocked   = fmt.Errorf("create blocked (policy / health / gift): %w", adapter.ErrRiskControl)
	ErrCreateFailed    = errors.New("room create failed")
)

// CreateError is a classified room/create failure for CLI JSONL.
type CreateError struct {
	Code    int
	Kind    string // face | ban | agreement | passport | block | paid | unknown
	Prompt  string
	AuthURL string
	Err     error
}

func (e *CreateError) Error() string {
	if e == nil {
		return "room/create failed"
	}
	msg := e.Prompt
	if msg == "" && e.Err != nil {
		msg = e.Err.Error()
	}
	if msg == "" {
		msg = "room/create failed"
	}
	return fmt.Sprintf("room/create status_code=%d kind=%s: %s", e.Code, e.Kind, msg)
}

func (e *CreateError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func promptFromCreate(m map[string]any) string {
	if data, _ := m["data"].(map[string]any); data != nil {
		if p := conv.AnyString(data["prompts"]); p != "" {
			return p
		}
	}
	return ""
}

// classifyCreateResponse maps companion createRoomResultHandle branches.
// Recoverable face challenge returns kind=face with AuthURL set (caller handles).
func classifyCreateResponse(m map[string]any) *CreateError {
	sc := conv.AnyInt(m["status_code"])
	prompt := promptFromCreate(m)
	extra, _ := m["extra"].(map[string]any)

	switch sc {
	case 0:
		return nil
	case createFaceRequired: // 4003028
		fc := parseFaceChallenge(m)
		ce := &CreateError{Code: sc, Kind: "face", Prompt: prompt, Err: ErrFaceRequired}
		if fc != nil {
			ce.AuthURL = fc.AuthURL
			if ce.Prompt == "" {
				ce.Prompt = fc.Prompt
			}
		}
		return ce
	case 4003173: // face fail without re-open path in companion
		return &CreateError{Code: sc, Kind: "face", Prompt: prompt, Err: ErrFaceFailed}
	case 20054:
		if prompt == "" {
			prompt = "complete live agreement / realname in latest Douyin app"
		}
		return &CreateError{Code: sc, Kind: "agreement", Prompt: prompt, Err: ErrLiveAgreement}
	case 10018, 20006, 4003035:
		if prompt == "" {
			prompt = "account banned from living"
		}
		banType := ""
		if extra != nil {
			banType = conv.AnyString(extra["ban_type"])
		}
		if banType != "" {
			prompt = fmt.Sprintf("%s (ban_type=%s)", prompt, banType)
		}
		return &CreateError{Code: sc, Kind: "ban", Prompt: prompt, Err: ErrAccountBanned}
	case 4003134:
		return &CreateError{Code: sc, Kind: "block", Prompt: prompt, Err: ErrCreateBlocked}
	case 4003102:
		if prompt == "" {
			prompt = "paid-live activity unavailable"
		}
		return &CreateError{Code: sc, Kind: "paid", Prompt: prompt, Err: ErrPaidLiveInvalid}
	case 4003163:
		if prompt == "" {
			prompt = "create blocked (health score / gift / policy dialog)"
		}
		// surface block detail keys when present
		if extra != nil {
			if d := conv.AnyString(extra["create_block_detail"]); d != "" {
				prompt = prompt + "; block_detail=" + conv.Truncate(d, 160)
			} else if d := conv.AnyString(extra["block_dialog_info"]); d != "" {
				prompt = prompt + "; dialog=" + conv.Truncate(d, 160)
			}
		}
		return &CreateError{Code: sc, Kind: "block", Prompt: prompt, Err: ErrCreateBlocked}
	default:
		if prompt == "" {
			prompt = fmt.Sprintf("unhandled status_code=%d", sc)
		}
		return &CreateError{Code: sc, Kind: "unknown", Prompt: prompt, Err: ErrCreateFailed}
	}
}
