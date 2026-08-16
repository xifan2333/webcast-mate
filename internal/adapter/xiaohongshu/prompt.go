package xiaohongshu

import (
	"context"
	"os"
	"strconv"

	"github.com/charmbracelet/huh"
	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/appcfg"
)

// OpenConfig start-time prefs for live-helper path.
type OpenConfig struct {
	Title      string
	Cover      string
	Distribute int // 1=public distribute (default), 0=trial no feed
}

func ResolveOpenConfig(ctx context.Context, cli *Client, opts adapter.StartOpts) (*OpenConfig, error) {
	_ = ctx
	_ = appcfg.EnsureExists()
	file, err := appcfg.Load()
	if err != nil {
		return nil, err
	}
	cfg := &OpenConfig{Distribute: 1}
	if p := file.GetPlatform("xiaohongshu"); p.Title != "" {
		cfg.Title = p.Title
	}
	// last room defaults
	if t, c, err := cli.LastRoomInfo(); err == nil {
		if cfg.Title == "" && t != "" {
			cfg.Title = t
		}
		cfg.Cover = c
	}
	if t := os.Getenv("WEBCAST_MATE_XHS_TITLE"); t != "" {
		cfg.Title = t
	}
	if d := os.Getenv("WEBCAST_MATE_XHS_DISTRIBUTE"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && (n == 0 || n == 1) {
			cfg.Distribute = n
		}
	}
	if opts.Yes {
		if cfg.Title == "" {
			cfg.Title = "直播"
		}
		return cfg, nil
	}
	if !isInteractive() {
		if cfg.Title == "" {
			cfg.Title = "直播"
		}
		return cfg, nil
	}

	title := cfg.Title
	distLabel := "正常开播（公域分发）"
	if cfg.Distribute == 0 {
		distLabel = "试播（不分发）"
	}
	dist := distLabel
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("标题").
			Description("直播间标题").
			Value(&title),
		huh.NewSelect[string]().
			Title("分发").
			Description("distribute=1 别人能刷到；0=试播不分发").
			Options(
				huh.NewOption("正常开播（公域分发）", "正常开播（公域分发）"),
				huh.NewOption("试播（不分发）", "试播（不分发）"),
			).
			Value(&dist),
	)).WithTheme(huh.ThemeCharm())
	if err := form.Run(); err != nil {
		return nil, err
	}
	if title == "" {
		title = "直播"
	}
	cfg.Title = title
	if dist == "试播（不分发）" {
		cfg.Distribute = 0
	} else {
		cfg.Distribute = 1
	}
	p := file.GetPlatform("xiaohongshu")
	p.Title = title
	file.Platforms["xiaohongshu"] = p
	_ = appcfg.Save(file)
	return cfg, nil
}

func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// silence unused if cover empty
