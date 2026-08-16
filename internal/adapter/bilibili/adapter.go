package bilibili

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/appcfg"
	"github.com/xifan2333/webcast-mate/internal/live"
	"github.com/xifan2333/webcast-mate/internal/platform"
	"github.com/xifan2333/webcast-mate/internal/secrets"
)

// Adapter implements adapter.Adapter for bilibili.
type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) ID() platform.ID { return platform.Bilibili }

func (a *Adapter) Start(ctx context.Context, opts adapter.StartOpts) (*adapter.StartResult, error) {
	cli, err := NewClient()
	if err != nil {
		return nil, err
	}
	sess, err := cli.EnsureLogin(ctx)
	if err != nil {
		return nil, mapErr(err)
	}

	cfg, err := ResolveConfig(ctx, cli, opts)
	if err != nil {
		return nil, mapErr(err)
	}

	server, key, err := cli.StartLive(ctx, cfg)
	if err != nil {
		return nil, mapErr(err)
	}

	file, _ := appcfg.Load()
	vbr, abr := 3200, 128
	if file != nil {
		vbr, abr = file.Bitrate("bilibili")
	}
	if err := live.Upsert("bilibili", live.Target{
		RoomID:       cfg.RoomID,
		Server:       server,
		Key:          key,
		VideoBitrate: vbr,
		AudioBitrate: abr,
		StartedAt:    time.Now().UTC(),
	}); err != nil {
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

	roomID := ""
	if t, ok := live.Get("bilibili"); ok {
		roomID = t.RoomID
	}
	if roomID == "" {
		if f, err := appcfg.Load(); err == nil {
			roomID = f.GetPlatform("bilibili").RoomID
		}
	}
	res.RoomID = roomID

	if roomID == "" {
		_ = live.Remove("bilibili")
		return res, nil
	}

	cli, err := NewClient()
	if err != nil {
		return nil, err
	}
	if s, e := secrets.Load("bilibili"); e == nil {
		_ = cli.setCookieHeader(s.Cookie)
	} else {
		_ = live.Remove("bilibili")
		return res, nil
	}

	if err := cli.StopLive(roomID); err != nil {
		fmt.Fprintf(os.Stderr, "bilibili: stop: %v\n", err)
	}
	_ = live.Remove("bilibili")
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
