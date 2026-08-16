package bilibili

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/appcfg"
)

// ResolveConfig loads appcfg platforms.bilibili, then prompts or uses -y.
func ResolveConfig(ctx context.Context, cli *Client, opts adapter.StartOpts) (*Config, error) {
	_ = ctx
	_ = appcfg.EnsureExists()
	file, err := appcfg.Load()
	if err != nil {
		return nil, err
	}
	cfg := fromPlatform(file.GetPlatform("bilibili"))

	if opts.Yes {
		if err := cfg.ValidateForStart(); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	if !isInteractive() {
		if err := cfg.ValidateForStart(); err != nil {
			return nil, fmt.Errorf("%w (non-interactive; pass -y with a complete config)", err)
		}
		return cfg, nil
	}

	var areaOptions []huh.Option[string]
	if areas, err := cli.ListAreas(); err == nil && len(areas) > 0 {
		for _, a := range areas {
			label := a.Name
			if a.ParentName != "" {
				label = a.ParentName + " / " + a.Name
			}
			areaOptions = append(areaOptions, huh.NewOption(label, a.ID))
		}
	}

	roomID := cfg.RoomID
	title := cfg.Title
	areaV2 := cfg.AreaV2
	cover := cfg.Cover
	if areaV2 == "" {
		areaV2 = "21"
	}

	// One page. No Filtering(true) (hides Title). Height(10) for options only.
	fields := []huh.Field{
		huh.NewInput().
			Title("直播间号").
			Description("直播姬 / 直播中心显示的房间号（不是短号）").
			Value(&roomID).
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("必填")
				}
				return nil
			}),
		huh.NewInput().
			Title("标题").
			Description("留空则不修改当前标题").
			Value(&title),
	}
	if len(areaOptions) > 0 {
		fields = append(fields, huh.NewSelect[string]().
			Title("分区").
			Description("本次直播所属分区").
			Options(areaOptions...).
			Value(&areaV2).
			Height(10))
	} else {
		fields = append(fields, huh.NewInput().
			Title("分区").
			Description("未能拉取列表时，可填写分区编号").
			Value(&areaV2).
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("必填")
				}
				return nil
			}))
	}
	fields = append(fields, huh.NewInput().
		Title("封面").
		Description("已有封面图链接（可选，留空跳过）").
		Value(&cover))

	form := huh.NewForm(huh.NewGroup(fields...)).WithTheme(huh.ThemeCharm())
	if err := form.Run(); err != nil {
		return nil, err
	}

	out := &Config{RoomID: roomID, AreaV2: areaV2, Title: title, Cover: cover}
	if err := out.ValidateForStart(); err != nil {
		return nil, err
	}
	prev := file.GetPlatform("bilibili")
	if err := file.SetPlatform("bilibili", out.toPlatform(prev)); err != nil {
		fmt.Fprintf(os.Stderr, "bilibili: warn save config: %v\n", err)
	}
	return out, nil
}

func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
