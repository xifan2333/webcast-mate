package bilibili

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/xifan2333/webcast-mate/internal/adapter"
)

// ResolveConfig loads saved config, then either uses it (-y) or runs huh prompts.
func ResolveConfig(ctx context.Context, cli *Client, opts adapter.StartOpts) (*Config, error) {
	_ = ctx
	cfg, path, err := LoadConfigFile()
	if err != nil {
		return nil, err
	}

	if opts.Yes {
		if err := cfg.ValidateForStart(); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	// Non-TTY: cannot prompt
	if !isInteractive() {
		if err := cfg.ValidateForStart(); err != nil {
			return nil, fmt.Errorf("%w (non-interactive; pass -y with a complete config)", err)
		}
		return cfg, nil
	}

	fmt.Fprintf(os.Stderr, "bilibili: configure stream (saved → %s)\n", path)

	// Prefer live area list when logged in; fall back to free text.
	var areaOptions []huh.Option[string]
	if areas, err := cli.ListAreas(); err == nil && len(areas) > 0 {
		for _, a := range areas {
			label := fmt.Sprintf("%s / %s (%s)", a.ParentName, a.Name, a.ID)
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

	fields := []huh.Field{
		huh.NewInput().
			Title("Room ID").
			Description("Real room id (link center), not the short id").
			Value(&roomID).
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("required")
				}
				return nil
			}),
		huh.NewInput().
			Title("Title").
			Description("Live title (empty = keep current on server)").
			Value(&title),
	}

	if len(areaOptions) > 0 {
		fields = append(fields, huh.NewSelect[string]().
			Title("Area").
			Description("Secondary partition (area_v2)").
			Options(areaOptions...).
			Value(&areaV2).
			Height(12).
			Filtering(true))
	} else {
		fields = append(fields, huh.NewInput().
			Title("Area v2 id").
			Description("Secondary partition id (e.g. 21)").
			Value(&areaV2).
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("required")
				}
				return nil
			}))
	}

	fields = append(fields, huh.NewInput().
		Title("Cover").
		Description("Local image path or hdslb.com URL (empty = skip)").
		Value(&cover))

	form := huh.NewForm(huh.NewGroup(fields...)).WithTheme(huh.ThemeCharm())
	if err := form.Run(); err != nil {
		return nil, err
	}

	out := &Config{
		RoomID: roomID,
		AreaV2: areaV2,
		Title:  title,
		Cover:  cover,
	}
	if err := out.ValidateForStart(); err != nil {
		return nil, err
	}
	if err := SaveConfig(out); err != nil {
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
