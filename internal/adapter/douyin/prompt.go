package douyin

import (
	"context"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/appcfg"
)

// OpenConfig is start-time prefs for douyin.
type OpenConfig struct {
	Title string
}

func ResolveOpenConfig(ctx context.Context, opts adapter.StartOpts) (*OpenConfig, error) {
	_ = ctx
	_ = appcfg.EnsureExists()
	file, err := appcfg.Load()
	if err != nil {
		return nil, err
	}
	cfg := &OpenConfig{}
	if p := file.GetPlatform("douyin"); p.Title != "" {
		cfg.Title = p.Title
	}
	if t := os.Getenv("WEBCAST_MATE_DY_TITLE"); t != "" {
		cfg.Title = t
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
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("标题").
			Description("直播间标题").
			Value(&title),
	)).WithTheme(huh.ThemeCharm())
	if err := form.Run(); err != nil {
		return nil, err
	}
	if title == "" {
		title = "直播"
	}
	cfg.Title = title
	// persist title preference
	p := file.GetPlatform("douyin")
	p.Title = title
	file.Platforms["douyin"] = p
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
