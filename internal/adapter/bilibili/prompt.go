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

	if !isInteractive() {
		if err := cfg.ValidateForStart(); err != nil {
			return nil, fmt.Errorf("%w (non-interactive; pass -y with a complete config)", err)
		}
		return cfg, nil
	}

	_ = path

	tree, treeErr := cli.ListAreaTree()

	roomID := cfg.RoomID
	title := cfg.Title
	areaV2 := cfg.AreaV2
	cover := cfg.Cover
	if areaV2 == "" {
		areaV2 = "21"
	}

	// Defaults for two-level area pick
	parentID := ""
	if tree != nil {
		parentID = tree.FindParentOf(areaV2)
		if parentID == "" && len(tree.Parents) > 0 {
			parentID = tree.Parents[0].ID
		}
	}

	// Critical huh Select quirks we avoid:
	// 1) Filtering(true) *starts* filter mode and replaces Title with the search box
	// 2) Height(n) sizes the WHOLE field; lipgloss crops the top → Title vanishes
	// 3) Form clamps tall fields (hundreds of options) via WithHeight → same crop
	// Fix: two-level 分区 (parent then child) keeps option lists short so Title stays.

	gRoom := huh.NewGroup(
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
	)
	gTitle := huh.NewGroup(
		huh.NewInput().
			Title("标题").
			Description("留空则不修改当前标题").
			Value(&title),
	)

	var groups []*huh.Group
	groups = append(groups, gRoom, gTitle)

	if treeErr == nil && tree != nil && len(tree.Parents) > 0 {
		var parentOpts []huh.Option[string]
		for _, p := range tree.Parents {
			parentOpts = append(parentOpts, huh.NewOption(p.Name, p.ID))
		}

		// Capture tree for OptionsFunc
		areaTree := tree
		groups = append(groups,
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("分区").
					Description("先选大类").
					Options(parentOpts...).
					Value(&parentID),
			),
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("分区").
					Description("再选具体分区").
					// OptionsFunc re-evaluates when parentID changes
					OptionsFunc(func() []huh.Option[string] {
						kids := areaTree.Children[parentID]
						opts := make([]huh.Option[string], 0, len(kids))
						for _, k := range kids {
							opts = append(opts, huh.NewOption(k.Name, k.ID))
						}
						if len(opts) == 0 {
							opts = append(opts, huh.NewOption("（无子分区）", areaV2))
						}
						return opts
					}, &parentID).
					Value(&areaV2),
			),
		)
	} else {
		groups = append(groups, huh.NewGroup(
			huh.NewInput().
				Title("分区").
				Description("未能拉取列表时，可填写分区编号").
				Value(&areaV2).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("必填")
					}
					return nil
				}),
		))
	}

	groups = append(groups, huh.NewGroup(
		huh.NewInput().
			Title("封面").
			Description("已有封面图链接（可选，留空跳过）").
			Value(&cover),
	))

	form := huh.NewForm(groups...).WithTheme(huh.ThemeCharm())
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
