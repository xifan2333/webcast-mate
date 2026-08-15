package bilibili

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/platform"
)

// Adapter implements adapter.Adapter for bilibili.
type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) ID() platform.ID { return platform.Bilibili }

func (a *Adapter) Start(ctx context.Context) (*adapter.StartResult, error) {
	if _, err := WriteExampleConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "bilibili: warn write example config: %v\n", err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}

	cli, err := NewClient()
	if err != nil {
		return nil, err
	}
	sess, err := cli.EnsureLogin(ctx)
	if err != nil {
		return nil, mapErr(err)
	}

	server, key, err := cli.StartLive(ctx, cfg)
	if err != nil {
		return nil, mapErr(err)
	}

	st := &RoomState{
		RoomID:    cfg.RoomID,
		Server:    server,
		Key:       key,
		AreaV2:    cfg.AreaV2,
		Title:     cfg.Title,
		StartedAt: time.Now().UTC(),
	}
	if err := SaveRoomState(st); err != nil {
		return nil, err
	}

	return &adapter.StartResult{
		Platform: string(platform.Bilibili),
		RoomID:   cfg.RoomID,
		Cookie:   sess.Cookie,
		Server:   server,
		Key:      key,
	}, nil
}

func (a *Adapter) Stop(ctx context.Context) (*adapter.StopResult, error) {
	_ = ctx
	res := &adapter.StopResult{
		Platform: string(platform.Bilibili),
		RoomID:   "",
		Status:   "stopped",
	}

	st, err := LoadRoomState()
	roomID := ""
	if err == nil && st != nil {
		roomID = st.RoomID
	}
	if roomID == "" {
		// try config room id
		if cfg, e := LoadConfig(); e == nil {
			roomID = cfg.RoomID
		}
	}
	res.RoomID = roomID

	if roomID == "" {
		// nothing to stop
		_ = ClearRoomState()
		return res, nil
	}

	cli, err := NewClient()
	if err != nil {
		return nil, err
	}
	// need cookies
	if sess, e := LoadSession(); e == nil {
		_ = cli.setCookieHeader(sess.Cookie)
	} else {
		// no session — still clear local state
		_ = ClearRoomState()
		return res, nil
	}

	if err := cli.StopLive(roomID); err != nil {
		// if already not live, still clear state
		fmt.Fprintf(os.Stderr, "bilibili: stop: %v\n", err)
	}
	_ = ClearRoomState()
	return res, nil
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, ErrNotConfigured):
		return err
	case errors.Is(err, ErrNotLoggedIn):
		return err
	case errors.Is(err, ErrQRTimeout), errors.Is(err, ErrQRExpired):
		return err
	case errors.Is(err, ErrFaceTimeout):
		return err
	default:
		return err
	}
}
