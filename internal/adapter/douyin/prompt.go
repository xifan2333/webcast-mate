package douyin

import (
	"context"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/appcfg"
	"github.com/xifan2333/webcast-mate/internal/conv"
	"github.com/xifan2333/webcast-mate/internal/platform"
)

// OpenConfig is start-time prefs for douyin.
// Area is "base|leaf" (Category.EncodeValue); empty → "-1|-1" at create.
// Cover is never prompted (create_info uri).
type OpenConfig struct {
	Title string
	Area  string // base|leaf
}

func ResolveOpenConfig(ctx context.Context, cli *Client, opts adapter.StartOpts) (*OpenConfig, error) {
	_ = ctx
	_ = appcfg.EnsureExists()
	file, err := appcfg.Load()
	if err != nil {
		return nil, err
	}
	cfg := &OpenConfig{}
	if p := file.GetPlatform(platform.Douyin); true {
		if p.Title != "" {
			cfg.Title = p.Title
		}
		if p.Area != "" {
			cfg.Area = p.Area
		}
	}
	if t := os.Getenv("WEBCAST_MATE_DY_TITLE"); t != "" {
		cfg.Title = t
	}
	if c := os.Getenv("WEBCAST_MATE_DY_AREA"); c != "" {
		cfg.Area = c
	}

	var areaOptions []huh.Option[string]
	if cli != nil {
		if list, err := cli.ListCategories(); err == nil {
			for _, c := range list {
				areaOptions = append(areaOptions, huh.NewOption(c.Label(), c.EncodeValue()))
			}
		}
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
	} else {
		fields = append(fields, huh.NewInput().Title("Area").Value(&area))
	}
	if err := huh.NewForm(huh.NewGroup(fields...)).WithTheme(huh.ThemeCharm()).Run(); err != nil {
		return nil, err
	}
	if title == "" {
		title = "Live"
	}
	cfg.Title = title
	cfg.Area = area
	p := file.GetPlatform(platform.Douyin)
	p.Title = title
	p.Area = area
	file.Platforms[string(platform.Douyin)] = p
	_ = appcfg.Save(file)
	return cfg, nil
}
