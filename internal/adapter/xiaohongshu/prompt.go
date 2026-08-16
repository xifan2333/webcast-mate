package xiaohongshu

import (
	"context"

	"github.com/charmbracelet/huh"
	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/appcfg"
	"github.com/xifan2333/webcast-mate/internal/conv"
	"github.com/xifan2333/webcast-mate/internal/platform"
)

// OpenConfig start-time prefs for live-helper path.
// Prompt: Title + Area only. Cover silent from last_room_info.
type OpenConfig struct {
	Title      string
	Cover      string // silent from last_room_info
	Area       string // leaf id or CategoryOtherValue
	Distribute int    // always 1 (public)
}

func ResolveOpenConfig(ctx context.Context, cli *Client, opts adapter.StartOpts) (*OpenConfig, error) {
	_ = ctx
	_ = appcfg.EnsureExists()
	file, err := appcfg.Load()
	if err != nil {
		return nil, err
	}
	cfg := &OpenConfig{Distribute: 1}
	if p := file.GetPlatform(platform.XiaoHongShu); true {
		if p.Title != "" {
			cfg.Title = p.Title
		}
		if p.Area != "" {
			cfg.Area = p.Area
		}
	}
	if t, c, err := cli.LastRoomInfo(); err == nil {
		if cfg.Title == "" && t != "" {
			cfg.Title = t
		}
		cfg.Cover = c
	}
	if id, err := cli.LastCategoryID(); err == nil && cfg.Area == "" {
		cfg.Area = id
	}

	var areaOptions []huh.Option[string]
	if list, err := cli.ListCategories(""); err == nil {
		for _, c := range list {
			areaOptions = append(areaOptions, huh.NewOption(c.Label(), c.ID))
		}
	} else {
		areaOptions = append(areaOptions, huh.NewOption("其他 / 其他", CategoryOtherValue))
	}
	if cfg.Area == "" {
		cfg.Area = CategoryOtherValue
	}

	if opts.Yes || !conv.IsInteractive() {
		if cfg.Title == "" {
			cfg.Title = "Live"
		}
		return cfg, nil
	}

	title := cfg.Title
	area := cfg.Area
	fields := []huh.Field{
		huh.NewInput().Title("Title").Value(&title),
	}
	if len(areaOptions) > 0 {
		fields = append(fields, huh.NewSelect[string]().
			Title("Area").
			Description("/ to filter").
			Options(areaOptions...).
			Value(&area).
			Height(12).
			Filtering(true))
	}
	if err := huh.NewForm(huh.NewGroup(fields...)).WithTheme(huh.ThemeCharm()).Run(); err != nil {
		return nil, err
	}
	if title == "" {
		title = "Live"
	}
	cfg.Title = title
	cfg.Area = area
	cfg.Distribute = 1
	p := file.GetPlatform(platform.XiaoHongShu)
	p.Title = title
	p.Area = area
	file.Platforms[string(platform.XiaoHongShu)] = p
	_ = appcfg.Save(file)
	return cfg, nil
}
