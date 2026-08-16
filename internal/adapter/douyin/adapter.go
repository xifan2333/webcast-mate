package douyin

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

func (a *Adapter) ID() platform.ID { return platform.Douyin }

func (a *Adapter) Login(ctx context.Context) (*adapter.LoginResult, error) {
	cli := NewClient()
	sec, err := cli.EnsureLogin(ctx)
	if err != nil {
		return nil, err
	}
	at := ""
	if !sec.LoginAt.IsZero() {
		at = sec.LoginAt.UTC().Format(time.RFC3339)
	}
	return &adapter.LoginResult{
		Platform: string(platform.Douyin), UserID: sec.UserID, UserName: sec.UserName,
		AuthBuckets: adapter.AuthFromSecrets(sec), LoginAt: at,
	}, nil
}

func (a *Adapter) Logout(ctx context.Context) (*adapter.LogoutResult, error) {
	_ = ctx
	_ = StopKeepalive()
	_ = secrets.Clear(platform.Douyin)
	return &adapter.LogoutResult{Platform: string(platform.Douyin), Status: "logged_out"}, nil
}

func (a *Adapter) Start(ctx context.Context, opts adapter.StartOpts) (*adapter.StartResult, error) {
	cli := NewClient()
	sec, err := cli.EnsureLogin(ctx)
	if err != nil {
		return nil, err
	}
	ocfg, err := ResolveOpenConfig(ctx, cli, opts)
	if err != nil {
		return nil, err
	}
	cr, err := cli.CreateRoom(ctx, ocfg.Title, ocfg.Area)
	if err != nil {
		return nil, err
	}
	file, _ := appcfg.Load()
	vbr, abr := 4000, 128
	if file != nil {
		vbr, abr = file.Bitrate(platform.Douyin)
	}
	if err := live.Upsert(platform.Douyin, live.Target{
		RoomID: cr.RoomID, StreamID: cr.StreamID, Server: cr.Server, Key: cr.Key,
		VideoBitrate: vbr, AudioBitrate: abr, StartedAt: time.Now().UTC(),
	}); err != nil {
		return nil, err
	}
	if sec != nil {
		if h := cli.cookieHeader(); h != "" {
			sec.SetCookieHeader(h)
		}
		_ = secrets.Save(platform.Douyin, sec)
	}
	_ = StartKeepalive()
	return &adapter.StartResult{
		Platform: string(platform.Douyin), RoomID: cr.RoomID,
		AuthBuckets: adapter.AuthFromSecrets(sec), Server: cr.Server, Key: cr.Key,
	}, nil
}

func (a *Adapter) Stop(ctx context.Context) (*adapter.StopResult, error) {
	_ = ctx
	res := &adapter.StopResult{Platform: string(platform.Douyin), Status: "stopped"}
	t, ok := live.Get(platform.Douyin)
	if ok {
		res.RoomID = t.RoomID
	}
	_ = StopKeepalive()
	cli := NewClient()
	if s, e := secrets.Load(platform.Douyin); e == nil {
		cli.ApplySecrets(s)
	}
	if ok && t.RoomID != "" && t.StreamID != "" {
		_ = cli.PingAnchor(t.RoomID, t.StreamID, RoomFinish)
	}
	_ = live.Remove(platform.Douyin)
	return res, nil
}

func (a *Adapter) Status(ctx context.Context) (*adapter.StatusResult, error) {
	_ = ctx
	out := &adapter.StatusResult{Platform: string(platform.Douyin), Status: "idle"}
	if t, ok := live.Get(platform.Douyin); ok && (t.Server != "" || t.Key != "") {
		out.RoomID, out.Server, out.Key, out.Status = t.RoomID, t.Server, t.Key, "live"
	}
	cli := NewClient()
	if s, e := secrets.Load(platform.Douyin); e == nil {
		cli.ApplySecrets(s)
		out.AuthBuckets = adapter.AuthFromSecrets(s)
	}
	return out, nil
}
