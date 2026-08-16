package douyin

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

// Adapter: streamingtool QR + create_info/a_bogus/create + ping LIVING/FINISH.
type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) ID() platform.ID { return platform.Douyin }

func (a *Adapter) Start(ctx context.Context, opts adapter.StartOpts) (*adapter.StartResult, error) {
	cli, err := NewClient()
	if err != nil {
		return nil, err
	}
	sec, err := cli.EnsureLogin(ctx)
	if err != nil {
		return nil, err
	}
	ocfg, err := ResolveOpenConfig(ctx, opts)
	if err != nil {
		return nil, err
	}
	cr, err := cli.CreateRoom(ocfg.Title)
	if err != nil {
		return nil, err
	}
	file, _ := appcfg.Load()
	vbr, abr := 4000, 128
	if file != nil {
		vbr, abr = file.Bitrate("douyin")
	}
	if err := live.Upsert("douyin", live.Target{
		RoomID:       cr.RoomID,
		StreamID:     cr.StreamID,
		Server:       cr.Server,
		Key:          cr.Key,
		VideoBitrate: vbr,
		AudioBitrate: abr,
		StartedAt:    time.Now().UTC(),
	}); err != nil {
		return nil, err
	}
	// refresh cookie after create
	cookie := cli.cookieHeader()
	if cookie == "" && sec != nil {
		cookie = sec.Cookie
	}
	// keep secrets fresh
	if sec != nil {
		sec.Cookie = cookie
		_ = secrets.Save("douyin", sec)
	}
	fmt.Fprintln(os.Stderr, "douyin: live started; push with server/key from stdout")
	return &adapter.StartResult{
		Platform: string(platform.Douyin),
		RoomID:   cr.RoomID,
		Cookie:   cookie,
		Server:   cr.Server,
		Key:      cr.Key,
	}, nil
}

func (a *Adapter) Stop(ctx context.Context) (*adapter.StopResult, error) {
	_ = ctx
	res := &adapter.StopResult{
		Platform: string(platform.Douyin),
		Status:   "stopped",
	}
	t, ok := live.Get("douyin")
	if ok {
		res.RoomID = t.RoomID
	}
	cli, err := NewClient()
	if err != nil {
		_ = live.Remove("douyin")
		return res, nil
	}
	if s, e := secrets.Load("douyin"); e == nil {
		cli.setCookieHeader(s.Cookie)
	}
	if ok && t.RoomID != "" && t.StreamID != "" {
		if err := cli.PingAnchor(t.RoomID, t.StreamID, RoomFinish); err != nil {
			fmt.Fprintf(os.Stderr, "douyin: ping FINISH: %v (clearing local anyway)\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "douyin: ping FINISH ok")
		}
	}
	_ = live.Remove("douyin")
	return res, nil
}

func (a *Adapter) Status(ctx context.Context) (*adapter.StatusResult, error) {
	_ = ctx
	out := &adapter.StatusResult{
		Platform: string(platform.Douyin),
		Status:   "idle",
	}
	if t, ok := live.Get("douyin"); ok && (t.Server != "" || t.Key != "") {
		out.RoomID = t.RoomID
		out.Server = t.Server
		out.Key = t.Key
		out.Status = "live"
	}
	cli, err := NewClient()
	if err != nil {
		return out, nil
	}
	if s, e := secrets.Load("douyin"); e == nil {
		cli.setCookieHeader(s.Cookie)
		out.Cookie = s.Cookie
	}
	// remote: get_pc_obs_status or check_exist
	if m, err := cli.PCObsStatus(); err == nil && m != nil {
		// best-effort parse
		if data := mapData(m); data != nil {
			if st := anyString(data["status"]); st != "" {
				// unknown enum — if room id present treat living
			}
			if rid := anyString(data["room_id"]); rid != "" {
				out.RoomID = rid
			}
			if rid := anyString(data["room_id_str"]); rid != "" {
				out.RoomID = rid
			}
			// living flags vary; if we have local stream keep live
		}
		sc, _ := m["status_code"].(float64)
		if sc == 0 && out.RoomID == "" {
			// leave as is
		}
	}
	if out.RoomID != "" && out.Status == "idle" {
		if m, err := cli.CheckRoomExist(out.RoomID); err == nil {
			if data := mapData(m); data != nil {
				// exist true → still something
				if ex, ok := data["exist"].(bool); ok && ex {
					out.Status = "live"
				}
			}
		}
	}
	return out, nil
}
