package xiaohongshu

import (
	"context"
	"fmt"
	"time"

	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/appcfg"
	"github.com/xifan2333/webcast-mate/internal/conv"
	"github.com/xifan2333/webcast-mate/internal/live"
	"github.com/xifan2333/webcast-mate/internal/platform"
	"github.com/xifan2333/webcast-mate/internal/secrets"
)

// Adapter implements live-helper 4.4.0 open/stop/status.
// Auth exported as cookies/headers/params buckets (access-token → headers).
type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) ID() platform.ID { return platform.XiaoHongShu }

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
		Platform: string(platform.XiaoHongShu), UserID: sec.UserID, UserName: sec.UserName,
		AuthBuckets: adapter.AuthFromSecrets(sec), LoginAt: at,
	}, nil
}

func (a *Adapter) Logout(ctx context.Context) (*adapter.LogoutResult, error) {
	_ = ctx
	_ = secrets.Clear(platform.XiaoHongShu)
	return &adapter.LogoutResult{Platform: string(platform.XiaoHongShu), Status: "logged_out"}, nil
}

func (a *Adapter) Start(ctx context.Context, opts adapter.StartOpts) (*adapter.StartResult, error) {
	cli := NewClient()
	if _, err := cli.EnsureLogin(ctx); err != nil {
		return nil, err
	}
	ocfg, err := ResolveOpenConfig(ctx, cli, opts)
	if err != nil {
		return nil, err
	}
	if t, ok := live.Get(platform.XiaoHongShu); ok && t.RoomID != "" {
		_ = cli.StopRoom(t.RoomID)
		_ = live.Remove(platform.XiaoHongShu)
	}
	roomID, pushURL, _, err := cli.PreRoom()
	if err != nil {
		return nil, err
	}
	if roomID == "" || pushURL == "" {
		return nil, fmt.Errorf("pre: empty room/push")
	}
	if err := cli.BeforeStart(roomID); err != nil {
		return nil, err
	}
	file, _ := appcfg.Load()
	vbr, abr := 4000, 128
	if file != nil {
		vbr, abr = file.Bitrate(platform.XiaoHongShu)
	}
	_ = cli.ReportPushInfo(roomID, pushURL, 1280, 720, vbr, 30)
	if err := cli.StartRoom(roomID, ocfg.Title, ocfg.Cover, ocfg.Distribute, ocfg.Area); err != nil {
		return nil, err
	}
	server, key := SplitPushURL(pushURL)
	if server == "" {
		server, key = pushURL, ""
	}
	sec := cli.ExportSecrets()
	_ = secrets.Save(platform.XiaoHongShu, sec)
	if err := live.Upsert(platform.XiaoHongShu, live.Target{
		RoomID: roomID, Server: server, Key: key,
		VideoBitrate: vbr, AudioBitrate: abr, StartedAt: time.Now().UTC(),
	}); err != nil {
		return nil, err
	}
	return &adapter.StartResult{
		Platform: string(platform.XiaoHongShu), RoomID: roomID,
		AuthBuckets: adapter.AuthFromSecrets(sec), Server: server, Key: key,
	}, nil
}

func (a *Adapter) Stop(ctx context.Context) (*adapter.StopResult, error) {
	_ = ctx
	res := &adapter.StopResult{Platform: string(platform.XiaoHongShu), Status: "stopped"}
	roomID := ""
	if t, ok := live.Get(platform.XiaoHongShu); ok {
		roomID = t.RoomID
		res.RoomID = roomID
	}
	cli := NewClient()
	if s, err := secrets.Load(platform.XiaoHongShu); err == nil {
		cli.ApplySecrets(s)
	}
	if roomID != "" && cli.AccessToken != "" {
		_ = cli.StopRoom(roomID)
	}
	_ = live.Remove(platform.XiaoHongShu)
	return res, nil
}

func (a *Adapter) Status(ctx context.Context) (*adapter.StatusResult, error) {
	_ = ctx
	out := &adapter.StatusResult{Platform: string(platform.XiaoHongShu), Status: "idle"}
	if t, ok := live.Get(platform.XiaoHongShu); ok && (t.Server != "" || t.Key != "") {
		out.RoomID, out.Server, out.Key, out.Status = t.RoomID, t.Server, t.Key, "live"
	}
	cli := NewClient()
	s, err := secrets.Load(platform.XiaoHongShu)
	if err != nil {
		return out, nil
	}
	cli.ApplySecrets(s)
	out.AuthBuckets = adapter.AuthFromSecrets(s)
	ok, _ := cli.CheckLogin()
	if !ok {
		out.Status = "idle"
		return out, nil
	}
	roomID := out.RoomID
	if roomID != "" {
		if m, err := cli.StreamInfo(roomID); err == nil && bizOK(m) {
			if data, _ := m["data"].(map[string]any); data != nil {
				if conv.AnyInt(data["stream_status"]) == 1 {
					out.Status = "live"
					out.RoomID = roomID
				}
			}
		}
	}
	return out, nil
}
