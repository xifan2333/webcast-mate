package xiaohongshu

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
)

// Adapter implements adapter.Adapter for xiaohongshu.
type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) ID() platform.ID { return platform.XiaoHongShu }

func (a *Adapter) Start(ctx context.Context, opts adapter.StartOpts) (*adapter.StartResult, error) {
	cli := NewClient()
	sec, err := cli.EnsureLogin(ctx)
	if err != nil {
		return nil, err
	}
	// Live APIs need robs sid
	if cli.Sid == "" {
		if isInteractive() {
			fmt.Fprintln(os.Stderr, "xiaohongshu: need SMS login for live open")
			if s2, err := cli.loginSMS(ctx); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrNeedSID, err)
			} else {
				sec = s2
			}
		} else {
			return nil, fmt.Errorf("%w: run without -y to SMS login, or put sid in secrets", ErrNeedSID)
		}
	}

	ocfg, err := ResolveOpenConfig(ctx, opts)
	if err != nil {
		return nil, err
	}

	pre, err := cli.LivePre()
	if err != nil {
		return nil, err
	}
	name := ocfg.Title
	if name == "" {
		name = pre.Name
	}
	cover := ocfg.Cover
	if cover == "" {
		cover = pre.Cover
	}
	if err := cli.LiveStart(pre.RoomID, name, cover); err != nil {
		return nil, err
	}
	server, key := SplitPushURL(pre.URL.PushURL)
	if server == "" {
		server = pre.URL.PushURL
	}

	file, _ := appcfg.Load()
	vbr, abr := 4000, 128
	if file != nil {
		vbr, abr = file.Bitrate("xiaohongshu")
	}
	if err := live.Upsert("xiaohongshu", live.Target{
		RoomID:       pre.RoomID,
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
		// expose sid as cookie field for pipelines that need auth string
		if sec.Sid != "" {
			cookie = "sid=" + sec.Sid
			if sec.Cookie != "" {
				cookie = sec.Cookie + "; " + cookie
			}
		} else {
			cookie = sec.Cookie
		}
	}

	return &adapter.StartResult{
		Platform: string(platform.XiaoHongShu),
		RoomID:   pre.RoomID,
		Cookie:   cookie,
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
	}
	res.RoomID = roomID

	cli := NewClient()
	if s, err := loadSecret(); err == nil {
		cli.applySecret(s)
	}
	if roomID != "" && cli.Sid != "" {
		if err := cli.LiveStop(roomID); err != nil {
			fmt.Fprintf(os.Stderr, "xiaohongshu: stop: %v\n", err)
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
	}
	if t, ok := live.Get("xiaohongshu"); ok {
		out.RoomID = t.RoomID
		out.Server = t.Server
		out.Key = t.Key
	}
	cli := NewClient()
	if s, err := loadSecret(); err == nil {
		cli.applySecret(s)
		if s.Sid != "" {
			out.Cookie = "sid=" + s.Sid
			if s.Cookie != "" {
				out.Cookie = s.Cookie + "; " + out.Cookie
			}
		} else {
			out.Cookie = s.Cookie
		}
	}
	if cli.Sid == "" {
		return out, nil
	}
	living, roomID, err := cli.CheckLive()
	if err != nil && !errors.Is(err, ErrNeedSID) {
		return nil, err
	}
	if roomID != "" {
		out.RoomID = roomID
	}
	if living {
		out.Status = "live"
	} else {
		out.Status = "idle"
	}
	return out, nil
}
