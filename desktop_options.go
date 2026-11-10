package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/overlay"
)

// Catalogs use a bounded Lobe list in a popover. Select and AutoComplete in
// the pinned Lobe release limit the popup itself, but do not scroll its rows.
func catalogPicker(c *ui.Context, value *string, options []field.SelectOption, label, fallback string, disabled bool) ui.Element {
	caption := fallback
	for _, option := range options {
		if option.Value == *value && fallback == "" {
			caption = option.Label
			break
		}
	}
	if caption == "" {
		caption = "请选择"
	}
	anchor := action.StyleButton(c, ui.ButtonBase(c).Label(label), "", action.ButtonProps{Variant: action.Default}).
		Disabled(disabled || len(options) == 0)
	anchor.Children(func() {
		display.Text(c, caption, display.TextProps{Rows: 1}).Grow(1).MinWidth(0).Ellipsis("…")
		ui.Icon(c, lucide.ChevronDown()).FontSize(14).Shrink(0)
	})
	open := ui.Local(anchor, "open", func() bool { return false })
	query := ui.Local(anchor, "query", func() string { return "" })
	focus := ui.Local(anchor, "focus", func() bool { return false })
	scroll := ui.Local(anchor, "scroll", func() ui.ScrollState { return ui.ScrollState{} })
	if disabled || len(options) == 0 {
		*open = false
		return anchor
	}
	if !*open {
		*query, *focus, scroll.Y = "", true, 0
	}
	width := min(440, max(200, anchor.Bounds().W-24))
	overlay.Popover(c, anchor, open, overlay.PopoverProps{Placement: overlay.Bottom}, func() {
		ui.Column(c).Width(width).Gap(6).Children(func() {
			if len(options) > 6 {
				previous := *query
				input := field.SearchBar(c, query, field.SearchBarProps{Placeholder: "筛选选项"}).Label("筛选" + label)
				if *focus {
					input.Focus()
					*focus = false
				}
				if previous != *query {
					scroll.Y = 0
				}
			}
			q := strings.ToLower(strings.TrimSpace(*query))
			items := make([]display.ListItem, 0, len(options))
			for _, option := range options {
				if q != "" && !strings.Contains(strings.ToLower(option.Label), q) &&
					!strings.Contains(strings.ToLower(option.Value), q) {
					continue
				}
				items = append(items, display.ListItem{Key: "v:" + option.Value,
					Label: option.Label, Icon: option.Icon, Disabled: option.Disabled})
			}
			if len(items) == 0 {
				muted(c, "无匹配选项").Padding(6)
				return
			}
			layout.ScrollArea(c, layout.ScrollAreaProps{State: scroll}, func() {
				if key := display.List(c, display.ListProps{Items: items, Active: "v:" + *value, Compact: true}); key != "" {
					*value, *open = strings.TrimPrefix(key, "v:"), false
					anchor.Focus()
					c.Invalidate()
				}
			}).Height(min(168, float32(34*len(items)-2))).Clip()
		})
	})
	return anchor
}
