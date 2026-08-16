package xiaohongshu

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/appcfg"
)

// OpenConfig is interactive open prefs.
type OpenConfig struct {
	Title string
	Cover string
}

func ResolveOpenConfig(ctx context.Context, opts adapter.StartOpts) (*OpenConfig, error) {
	_ = ctx
	_ = appcfg.EnsureExists()
	file, err := appcfg.Load()
	if err != nil {
		return nil, err
	}
	p := file.GetPlatform("xiaohongshu")
	cfg := &OpenConfig{Title: p.Title, Cover: p.Cover}

	if opts.Yes || !isInteractive() {
		return cfg, nil
	}

	title, cover := cfg.Title, cfg.Cover
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("标题").
			Description("留空则使用准备接口返回的名称").
			Value(&title),
		huh.NewInput().
			Title("封面").
			Description("封面图链接（可选，留空用默认）").
			Value(&cover),
	)).WithTheme(huh.ThemeCharm())
	if err := form.Run(); err != nil {
		return nil, err
	}
	cfg.Title, cfg.Cover = title, cover
	p.Title, p.Cover = title, cover
	if err := file.SetPlatform("xiaohongshu", p); err != nil {
		fmt.Fprintf(os.Stderr, "xiaohongshu: warn save config: %v\n", err)
	}
	return cfg, nil
}
