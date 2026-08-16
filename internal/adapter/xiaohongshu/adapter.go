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
	"github.com/xifan2333/webcast-mate/internal/secrets"
)

// Adapter implements live-helper 4.4.0 open/stop/status.
// stdout.cookie is always "" — xhs danmaku uses browser cookies, not helper AT.
type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) ID() platform.ID { return platform.XiaoHongShu }

func (a *Adapter) Start(ctx context.Context, opts adapter.StartOpts) (*adapter.StartResult, error) {
	cli := NewClient()
	if _, err := cli.EnsureLogin(ctx); err != nil {
		return nil, err
	}

	ocfg, err := ResolveOpenConfig(ctx, cli, opts)
	if err != nil {
		return nil, err
	}

	if t, ok := live.Get("xiaohongshu"); ok && t.RoomID != "" {
		_ = cli.StopRoom(t.RoomID)
		_ = live.Remove("xiaohongshu")
	}

	roomID, pushURL, _, err := cli.PreRoom()
	if err != nil {
		return nil, err
	}
	if roomID == "" || pushURL == "" {
		return nil, fmt.Errorf("pre: empty room/push")
	}
	fmt.Fprintf(os.Stderr, "xiaohongshu: pre room=%s\n", roomID)

	if err := cli.BeforeStart(roomID); err != nil {
		return nil, err
	}
	fmt.Fprintln(os.Stderr, "xiaohongshu: before/start ok")

	file, _ := appcfg.Load()
	vbr, abr := 4000, 128
	if file != nil {
		vbr, abr = file.Bitrate("xiaohongshu")
	}
	_ = cli.ReportPushInfo(roomID, pushURL, 1280, 720, vbr, 30)

	if err := cli.StartRoom(roomID, ocfg.Title, ocfg.Cover, ocfg.Distribute); err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "xiaohongshu: start ok distribute=%d — begin RTMP push now\n", ocfg.Distribute)

	server, key := SplitPushURL(pushURL)
	if server == "" {
		server, key = pushURL, ""
	}

	// refresh secrets cookie string (a1 may have rotated); room stays in live.json only
	_ = secrets.Save("xiaohongshu", cli.SecretsFile())

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

	return &adapter.StartResult{
		Platform: string(platform.XiaoHongShu),
		RoomID:   roomID,
		Cookie:   "", // danmaku: use browser cookie separately
		Server:   server,
		Key:      key,
	}, nil
}

func (a *Adapter) Stop(ctx context.Context) (*adapter.StopResult, error) {
	_ = ctx
	res := &adapter.StopResult{
		Platform: string(platform.XiaoHongShu),
		Status:   "stopped",
	}
	roomID := ""
	if t, ok := live.Get("xiaohongshu"); ok {
		roomID = t.RoomID
		res.RoomID = roomID
	}

	cli := NewClient()
	if s, err := secrets.Load("xiaohongshu"); err == nil {
		cli.LoadSecrets(s)
	}
	if roomID != "" && cli.AccessToken != "" {
		if err := cli.StopRoom(roomID); err != nil {
			fmt.Fprintf(os.Stderr, "xiaohongshu: stop: %v (clearing local)\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "xiaohongshu: stop ok")
		}
	}
	_ = live.Remove("xiaohongshu")
	return res, nil
}

func (a *Adapter) Status(ctx context.Context) (*adapter.StatusResult, error) {
	_ = ctx
	out := &adapter.StatusResult{
		Platform: string(platform.XiaoHongShu),
		Status:   "idle",
		Cookie:   "", // never export helper AT as danmaku cookie
	}
	if t, ok := live.Get("xiaohongshu"); ok && (t.Server != "" || t.Key != "") {
		out.RoomID = t.RoomID
		out.Server = t.Server
		out.Key = t.Key
		out.Status = "live"
	}

	cli := NewClient()
	s, err := secrets.Load("xiaohongshu")
	if err != nil {
		return out, nil
	}
	cli.LoadSecrets(s)

	ok, _ := cli.CheckLogin()
	if !ok {
		out.Status = "idle"
		return out, nil
	}
	roomID := out.RoomID
	if roomID != "" {
		if m, err := cli.StreamInfo(roomID); err == nil && bizOK(m) {
			if data, _ := m["data"].(map[string]any); data != nil {
				if anyInt(data["stream_status"]) == 1 {
					out.Status = "live"
					out.RoomID = roomID
				}
			}
		}
	}
	return out, nil
}
