package bilibili

import (
	"context"
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/appcfg"
	"github.com/xifan2333/webcast-mate/internal/conv"
	"github.com/xifan2333/webcast-mate/internal/platform"
)

// ResolveOpenConfig loads prefs, pulls own room via blink GetInfo (no room_id typing).
// Prompt: Title + Area only (no cover).
func ResolveOpenConfig(ctx context.Context, cli *Client, opts adapter.StartOpts) (*OpenConfig, error) {
	_ = ctx
	_ = appcfg.EnsureExists()
	file, err := appcfg.Load()
	if err != nil {
		return nil, err
	}
	cfg := fromPlatform(file.GetPlatform(platform.Bilibili))

	if info, err := cli.GetBlinkRoomInfo(); err != nil {
		if cfg.RoomID == "" {
			return nil, fmt.Errorf("%w: cannot resolve room_id (%v)", ErrNotConfigured, err)
		}
	} else {
		cfg.RoomID = info.RoomID
		if cfg.Title == "" && info.Title != "" {
			cfg.Title = info.Title
		}
		if info.AreaV2ID != "" && (cfg.Area == "" || cfg.Area == "21") {
			cfg.Area = info.AreaV2ID
		}
	}

	if opts.Yes || !conv.IsInteractive() {
		if err := cfg.ValidateForStart(); err != nil {
			return nil, err
		}
		prev := file.GetPlatform(platform.Bilibili)
		_ = file.SetPlatform(platform.Bilibili, cfg.toPlatform(prev))
		return cfg, nil
	}

	var areaOptions []huh.Option[string]
	if areas, err := cli.ListAreas(); err == nil {
		for _, a := range areas {
			label := a.Name
			if a.ParentName != "" {
				label = a.ParentName + " / " + a.Name
			}
			areaOptions = append(areaOptions, huh.NewOption(label, a.ID))
		}
	}
	title, area := cfg.Title, cfg.Area
	if area == "" {
		area = "21"
	}
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
	} else {
		fields = append(fields, huh.NewInput().Title("Area").Value(&area))
	}
	if err := huh.NewForm(huh.NewGroup(fields...)).WithTheme(huh.ThemeCharm()).Run(); err != nil {
		return nil, err
	}
	out := &OpenConfig{RoomID: cfg.RoomID, Area: area, Title: title}
	if err := out.ValidateForStart(); err != nil {
		return nil, err
	}
	prev := file.GetPlatform(platform.Bilibili)
	_ = file.SetPlatform(platform.Bilibili, out.toPlatform(prev))
	return out, nil
}
