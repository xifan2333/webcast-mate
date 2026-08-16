package xiaohongshu

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/appcfg"
)

// OpenConfig holds the phone OBS 6-digit code (and optional notes).
type OpenConfig struct {
	// Code is the 6-digit code from 手机预直播页 → 电脑 tab.
	Code string
}

func ResolveOpenConfig(ctx context.Context, opts adapter.StartOpts) (*OpenConfig, error) {
	_ = ctx
	_ = appcfg.EnsureExists()
	file, err := appcfg.Load()
	if err != nil {
		return nil, err
	}
	// store last code? not really durable — just prompt
	_ = file

	cfg := &OpenConfig{}
	if opts.Yes {
		// allow WEBCAST_MATE_XHS_CODE env for scripts
		if c := os.Getenv("WEBCAST_MATE_XHS_CODE"); c != "" {
			cfg.Code = c
			return cfg, nil
		}
		return nil, fmt.Errorf("%w: need OBS code (interactive or WEBCAST_MATE_XHS_CODE)", ErrNotConfigured)
	}
	if !isInteractive() {
		if c := os.Getenv("WEBCAST_MATE_XHS_CODE"); c != "" {
			cfg.Code = c
			return cfg, nil
		}
		return nil, fmt.Errorf("%w: non-interactive; set WEBCAST_MATE_XHS_CODE", ErrNotConfigured)
	}

	code := ""
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("连接码").
			Description("手机小红书预直播页 →「电脑」tab 显示的 6 位码").
			Value(&code).
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("必填")
				}
				return nil
			}),
	)).WithTheme(huh.ThemeCharm())
	if err := form.Run(); err != nil {
		return nil, err
	}
	cfg.Code = code
	return cfg, nil
}
