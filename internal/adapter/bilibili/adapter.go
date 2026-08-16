package bilibili

import (
	"context"
	"time"

	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/appcfg"
	"github.com/xifan2333/webcast-mate/internal/live"
	"github.com/xifan2333/webcast-mate/internal/platform"
	"github.com/xifan2333/webcast-mate/internal/secrets"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) ID() platform.ID { return platform.Bilibili }

func (a *Adapter) Login(ctx context.Context) (*adapter.LoginResult, error) {
	cli := NewClient()
	sess, err := cli.EnsureLogin(ctx)
	if err != nil {
		return nil, err
	}
	at := ""
	if !sess.LoginAt.IsZero() {
		at = sess.LoginAt.UTC().Format(time.RFC3339)
	}
	return &adapter.LoginResult{
		Platform: string(platform.Bilibili), UserID: sess.UserID, UserName: sess.UserName,
		AuthBuckets: adapter.AuthFromSecrets(sess), LoginAt: at,
	}, nil
}

func (a *Adapter) Logout(ctx context.Context) (*adapter.LogoutResult, error) {
	_ = ctx
	_ = secrets.Clear(platform.Bilibili)
	return &adapter.LogoutResult{Platform: string(platform.Bilibili), Status: "logged_out"}, nil
}

func (a *Adapter) Start(ctx context.Context, opts adapter.StartOpts) (*adapter.StartResult, error) {
	cli := NewClient()
	sess, err := cli.EnsureLogin(ctx)
	if err != nil {
		return nil, err
	}
	cfg, err := ResolveOpenConfig(ctx, cli, opts)
	if err != nil {
		return nil, err
	}
	server, key, err := cli.StartLive(ctx, cfg)
	if err != nil {
		return nil, err
	}
	file, _ := appcfg.Load()
	vbr, abr := 3200, 128
	if file != nil {
		vbr, abr = file.Bitrate(platform.Bilibili)
	}
	if err := live.Upsert(platform.Bilibili, live.Target{
		RoomID: cfg.RoomID, Server: server, Key: key,
		VideoBitrate: vbr, AudioBitrate: abr, StartedAt: time.Now().UTC(),
	}); err != nil {
		return nil, err
	}
	return &adapter.StartResult{
		Platform: string(platform.Bilibili), RoomID: cfg.RoomID,
		AuthBuckets: adapter.AuthFromSecrets(sess), Server: server, Key: key,
	}, nil
}

func (a *Adapter) Stop(ctx context.Context) (*adapter.StopResult, error) {
	_ = ctx
	res := &adapter.StopResult{Platform: string(platform.Bilibili), Status: "stopped"}
	roomID := ""
	if t, ok := live.Get(platform.Bilibili); ok {
		roomID = t.RoomID
		res.RoomID = roomID
	}
	if roomID == "" {
		if f, err := appcfg.Load(); err == nil {
			roomID = f.GetPlatform(platform.Bilibili).RoomID
			res.RoomID = roomID
		}
	}
	if roomID == "" {
		_ = live.Remove(platform.Bilibili)
		return res, nil
	}
	cli := NewClient()
	if s, e := secrets.Load(platform.Bilibili); e == nil {
		cli.ApplySecrets(s)
	} else {
		_ = live.Remove(platform.Bilibili)
		return res, nil
	}
	_ = cli.StopLive(roomID)
	_ = live.Remove(platform.Bilibili)
	return res, nil
}

func (a *Adapter) Status(ctx context.Context) (*adapter.StatusResult, error) {
	_ = ctx
	out := &adapter.StatusResult{Platform: string(platform.Bilibili), Status: "idle"}
	if s, err := secrets.Load(platform.Bilibili); err == nil {
		out.AuthBuckets = adapter.AuthFromSecrets(s)
	}
	if t, ok := live.Get(platform.Bilibili); ok {
		out.Server, out.Key = t.Server, t.Key
		if t.RoomID != "" {
			out.RoomID = t.RoomID
		}
	}
	roomID := out.RoomID
	if roomID == "" {
		if f, err := appcfg.Load(); err == nil {
			roomID = f.GetPlatform(platform.Bilibili).RoomID
		}
	}
	// prefer blink GetInfo when logged in
	cli := NewClient()
	if s, err := secrets.Load(platform.Bilibili); err == nil {
		cli.ApplySecrets(s)
		if info, err := cli.GetBlinkRoomInfo(); err == nil {
			out.RoomID = info.RoomID
			out.Status = LiveStatusString(info.LiveStatus)
			return out, nil
		}
	}
	if roomID == "" {
		return out, nil
	}
	out.RoomID = roomID
	info, err := cli.QueryRoomInfo(roomID)
	if err != nil {
		return out, nil
	}
	if info.RoomID != "" {
		out.RoomID = info.RoomID
	}
	out.Status = LiveStatusString(info.LiveStatus)
	return out, nil
}
