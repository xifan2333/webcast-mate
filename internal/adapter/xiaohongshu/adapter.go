package xiaohongshu

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/appcfg"
	"github.com/xifan2333/webcast-mate/internal/live"
	"github.com/xifan2333/webcast-mate/internal/platform"
)

// Adapter: web QR login + phone OBS 6-digit code → push_url.
type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) ID() platform.ID { return platform.XiaoHongShu }

func (a *Adapter) Start(ctx context.Context, opts adapter.StartOpts) (*adapter.StartResult, error) {
	cli := NewClient()
	sec, err := cli.EnsureLogin(ctx)
	if err != nil {
		return nil, err
	}

	ocfg, err := ResolveOpenConfig(ctx, opts)
	if err != nil {
		return nil, err
	}

	obs, err := cli.FetchObsPushURL(ocfg.Code)
	if err != nil {
		return nil, err
	}
	server, key := obs.Server, obs.Key
	if server == "" {
		server, key = SplitPushURL(obs.PushURL)
	}
	if server == "" {
		server = obs.PushURL
	}

	file, _ := appcfg.Load()
	vbr, abr := 4000, 128
	if file != nil {
		vbr, abr = file.Bitrate("xiaohongshu")
	}
	roomID := obs.RoomID
	if roomID == "" {
		roomID = ocfg.Code // fallback identifier
	}
	if err := live.Upsert("xiaohongshu", live.Target{
		RoomID:       roomID,
		Server:       server,
		Key:          key,
		VideoBitrate: vbr,
		AudioBitrate: abr,
		StartedAt:    time.Now().UTC(),
	}); err != nil {
		return nil, err
	}

	cookie := ""
	if sec != nil {
		cookie = sec.Cookie
	} else {
		cookie = cli.cookieHeader()
	}

	fmt.Fprintln(os.Stderr, "xiaohongshu: got push_url; start streaming, then tap 进入直播 on phone if needed")

	return &adapter.StartResult{
		Platform: string(platform.XiaoHongShu),
		RoomID:   roomID,
		Cookie:   cookie,
		Server:   server,
		Key:      key,
	}, nil
}

func (a *Adapter) Stop(ctx context.Context) (*adapter.StopResult, error) {
	_ = ctx
	// Web OBS path: stop is primarily on the phone; clear local live.json.
	res := &adapter.StopResult{
		Platform: string(platform.XiaoHongShu),
		Status:   "stopped",
	}
	if t, ok := live.Get("xiaohongshu"); ok {
		res.RoomID = t.RoomID
	}
	_ = live.Remove("xiaohongshu")
	fmt.Fprintln(os.Stderr, "xiaohongshu: cleared local live.json (end live on phone if still on air)")
	return res, nil
}

func (a *Adapter) Status(ctx context.Context) (*adapter.StatusResult, error) {
	_ = ctx
	out := &adapter.StatusResult{
		Platform: string(platform.XiaoHongShu),
		Status:   "idle",
	}
	if t, ok := live.Get("xiaohongshu"); ok && (t.Server != "" || t.Key != "") {
		out.RoomID = t.RoomID
		out.Server = t.Server
		out.Key = t.Key
		out.Status = "live" // local session active; remote stop is phone-side
	}
	if s, err := loadSecret(); err == nil {
		out.Cookie = s.Cookie
	}
	// best-effort remote: living_push_url
	cli := NewClient()
	if s, err := loadSecret(); err == nil {
		cli.applySecret(s)
	}
	if cli.WebSession != "" {
		if obs, err := cli.LivingPushURL(); err == nil && obs != nil && obs.PushURL != "" {
			out.Status = "live"
			if obs.RoomID != "" {
				out.RoomID = obs.RoomID
			}
			if out.Server == "" {
				out.Server, out.Key = SplitPushURL(obs.PushURL)
			}
		}
	}
	return out, nil
}
