package main

import (
	"cmp"
	"fmt"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

type duplicateModRow struct {
	id   string
	file content.ModIDFile
}

type duplicateModView struct {
	rows   []duplicateModRow
	scroll ui.ListState
}

func (v *contentView) duplicateModNotice(c *ui.Context) {
	if len(v.duplicates) == 0 {
		return
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, lucide.TriangleAlert()).FontSize(16)
		label := fmt.Sprintf("重复模组 ID (%d)", len(v.duplicates))
		if action.Button(c, label, action.ButtonProps{Size: theme.Small, Variant: action.Default}).
			Label("重复模组 ID").Clicked() {
			view := &duplicateModView{}
			for _, group := range v.duplicates {
				view.rows = append(view.rows, duplicateModRow{id: group.ID})
				for _, file := range group.Files {
					view.rows = append(view.rows, duplicateModRow{file: file})
				}
			}
			v.duplicateView = view
		}
	})
}

func (v *contentView) showDuplicateMods(c *ui.Context, p *duplicateModView) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if icon(c, lucide.ArrowLeft(), "返回模组列表", false) {
			v.duplicateView = nil
		}
		display.Text(c, "重复模组 ID", display.TextProps{Strong: true, FontSize: 16})
	})
	muted(c, "不同启用文件声明了相同的模组 ID。实际加载结果以游戏为准。")
	ui.List(c.Key(p), &p.scroll, len(p.rows)).Grow(1).MinHeight(0).Rows(func(row ui.ListRow) {
		r, c := p.rows[row.Index], row.Context
		if r.id != "" {
			ui.Row(c).Height(34).Padding(0, 8).AlignItems(ui.Center).Children(func() {
				display.Text(c, r.id, display.TextProps{Strong: true, Rows: 1}).
					Grow(1).MinWidth(0).Ellipsis("…").Tooltip(r.id).Selectable()
			})
			return
		}
		ui.Row(c).Height(28).Padding(0, 16).Gap(12).AlignItems(ui.Center).Children(func() {
			display.Text(c, r.file.Name, display.TextProps{Rows: 1}).
				Grow(2).MinWidth(0).Ellipsis("…").Tooltip(r.file.Name).Selectable()
			version := cmp.Or(r.file.Version, "版本未知")
			muted(c, version).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Tooltip(version).Selectable()
		})
	})
}
