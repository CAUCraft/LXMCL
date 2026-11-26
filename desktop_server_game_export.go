package main

import (
	"fmt"
	"slices"

	"github.com/CAUCraft/LXMCL/internal/server"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
)

type serverGameExport struct {
	favorites, preview []server.Server
	checked            map[int]bool
	list               ui.ListState
}

func (a *desktop) serverGameExportView(c *ui.Context, p *serverBackupView, open *bool, locked bool) {
	if p.gameList == nil {
		p.gameList = &serverGameExport{favorites: slices.Clone(a.state.Servers), checked: make(map[int]bool)}
	}
	d := p.gameList
	preview := d.preview != nil
	rows := d.favorites
	switch {
	case preview:
		rows = d.preview
		muted(c, fmt.Sprintf("将导出 %d 条收藏 · 仅名称和地址", len(rows)))
	case len(rows) == 0:
		muted(c, "暂无可导出的启动器收藏")
	default:
		muted(c, fmt.Sprintf("已选 %d 条收藏", len(d.checked)))
	}
	if len(rows) > 0 {
		ui.List(c.Key(preview), &d.list, len(rows)).FillWidth().Height(float32(min(len(rows), 5) * 38)).Rows(func(row ui.ListRow) {
			entry, rc := rows[row.Index], row.Context
			ui.Row(rc).Height(38).Gap(8).AlignItems(ui.Center).Children(func() {
				if !preview {
					on := d.checked[row.Index]
					if choice.Checkbox(rc, &on, "", choice.CheckboxProps{}).Label("选择服务器收藏 " + entry.Address).Disabled(locked).Changed() {
						if on {
							d.checked[row.Index] = true
						} else {
							delete(d.checked, row.Index)
						}
					}
				}
				ui.Column(rc).Grow(1).MinWidth(0).Children(func() {
					ui.Text(rc, entry.Name).SingleLine().Ellipsis("…").Tooltip(entry.Name)
					muted(rc, entry.Address).FontSize(12).SingleLine().Ellipsis("…").Tooltip(entry.Address)
				})
			})
		})
	}
	if p.err != "" {
		feedback.Alert(c, "无法保存游戏服务器列表", feedback.AlertProps{Type: feedback.Warning, Description: p.err})
	}
	ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
		if button(c, "取消", action.Default, locked) {
			*open = false
			return
		}
		if preview {
			if button(c, "返回选择", action.Default, locked) {
				d.preview, d.list, p.err = nil, ui.ListState{}, ""
				return
			}
			if button(c, "保存 servers.dat…", action.Primary, locked) {
				options := mygo.SaveDialogOptions{Title: "导出游戏服务器列表", ButtonLabel: "保存",
					DefaultPath: "servers.dat", Filters: []mygo.FileFilter{{Name: "游戏服务器列表", Extensions: []string{"dat"}}}}
				a.saveServerExport(p, options, d.preview, server.ExportGameList, "游戏服务器列表已导出")
			}
		} else if button(c, "预览所选", action.Primary, locked || len(d.checked) == 0) {
			for i, entry := range d.favorites {
				if d.checked[i] {
					d.preview = append(d.preview, entry)
				}
			}
			d.list = ui.ListState{}
		}
	})
}
