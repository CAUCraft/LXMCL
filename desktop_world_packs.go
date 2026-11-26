package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/saves"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

const (
	worldPacksLocal = iota
	worldPacksEnabled
	worldPacksDisabled
)

type worldPacksView struct {
	world    saves.World
	data     saves.WorldDataPacks
	status   map[string]string
	err      error
	cancel   context.CancelFunc
	loading  bool
	tab      int
	scroll   ui.ListState
	preview  *worldPackPreview
	copyText string
	copyErr  error
	copied   bool
}

func (p *worldBrowser) closeWorldPacks() {
	if p.packs != nil {
		if p.packs.cancel != nil {
			p.packs.cancel()
		}
		p.packs.closePreview()
	}
	p.packs = nil
}

func (a *desktop) openWorldPacks(browser *worldBrowser, i launcher.Instance, world saves.World) {
	if a.ctx == nil {
		return
	}
	browser.closeWorldPacks()
	ctx, cancel := context.WithCancel(a.ctx)
	p := &worldPacksView{world: world, cancel: cancel, loading: true}
	browser.packs = p
	go func() {
		defer cancel()
		data, err := saves.ReadWorldDataPacks(ctx, i.GameDir, world)
		status := make(map[string]string)
		for _, pack := range data.Enabled {
			if name, ok := strings.CutPrefix(pack.Name, "file/"); ok {
				status[name] = "已启用"
			}
		}
		for _, pack := range data.Disabled {
			if name, ok := strings.CutPrefix(pack.Name, "file/"); ok {
				if status[name] == "已启用" {
					status[name] = "记录冲突"
				} else if status[name] != "记录冲突" {
					status[name] = "已停用"
				}
			}
		}
		var copyText string
		if err == nil {
			copyText = worldSavedPackClipboard(data)
		}
		a.post(func() {
			if !a.form.open || a.form.worlds != browser || browser.packs != p || a.form.target != i {
				return
			}
			p.data, p.status, p.err, p.loading, p.cancel = data, status, err, false, nil
			p.copyText = copyText
		})
	}()
}

func (a *desktop) worldPacksEditor(c *ui.Context, browser *worldBrowser, i launcher.Instance, p *worldPacksView) {
	current, exists := a.state.Find(i.ID)
	stale := !exists || current != i
	name := p.world.LevelName()
	if name == "" {
		name = p.world.Folder()
	}
	refresh, copyList := false, false
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			panelHeading(c, lucide.Layers(), name, fmt.Sprintf("%s · #%d · 目录 %s", i.Name, i.ID, p.world.Folder()))
		})
		if p.preview == nil {
			copyList = button(c.Key(&p.copyText), "复制上次保存的清单", action.Default, stale || p.copyText == "")
			refresh = icon(c.Key(p), lucide.RefreshCw(), "刷新数据包", stale || p.loading || a.ctx == nil)
		}
	})
	if refresh {
		a.openWorldPacks(browser, i, p.world)
		browser.packs.tab = p.tab
		return
	}
	switch {
	case stale:
		p.closePreview()
		feedback.Alert(c, "实例已变化，请关闭并重新打开存档列表。", feedback.AlertProps{Type: feedback.Warning})
	case p.err != nil:
		feedback.Alert(c, "无法读取世界数据包", feedback.AlertProps{Type: feedback.Warning, Description: p.err.Error()})
	case p.loading:
		flightLoading(c, "正在读取世界数据包…")
	case p.preview != nil:
		a.showWorldPackPreview(c, browser, i, p)
	default:
		before := p.tab
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			action.Segmented(c, &p.tab, []string{"本地文件", "上次启用", "上次停用"}, action.SegmentedProps{Size: theme.Small}).Label("数据包视图")
			switch {
			case p.copyErr != nil:
				feedback.Alert(c, "复制数据包清单失败，请重试", feedback.AlertProps{
					Type: feedback.Error, Variant: feedback.Plain,
				}).AlignSelf(ui.Center).MinWidth(0)
			case p.copied:
				muted(c, "已复制数据包清单").FontSize(12)
			}
		})
		if before != p.tab {
			p.scroll = ui.ListState{}
		}
		a.worldPackRows(c, browser, i, p)
	}
	ui.Row(c).Justify(ui.End).Children(func() {
		if button(c, "返回存档列表", action.Default, false) {
			browser.closeWorldPacks()
		}
	})
	if copyList && a.currentWorldPacks(browser, i, p) {
		p.copied, p.copyErr = false, nil
		c.WriteClipboardThen(p.copyText, func(err error) {
			if a.currentWorldPacks(browser, i, p) {
				p.copied, p.copyErr = err == nil, err
			}
		})
	}
}

func (a *desktop) currentWorldPacks(browser *worldBrowser, i launcher.Instance, p *worldPacksView) bool {
	if !a.form.open || a.form.worlds != browser || a.form.target != i ||
		browser.packs != p || p.loading || p.preview != nil || p.copyText == "" {
		return false
	}
	current, exists := a.state.Find(i.ID)
	selected, ok := browser.selectedWorld()
	return exists && current == i && ok && selected.Folder() == p.world.Folder()
}

func worldSavedPackClipboard(data saves.WorldDataPacks) string {
	if !data.Saved || data.SavedErr != nil {
		return ""
	}
	var text strings.Builder
	text.WriteString("游戏上次保存的数据包清单")
	for _, section := range []struct {
		title string
		rows  []saves.DataPack
	}{
		{"上次启用", data.Enabled},
		{"上次停用", data.Disabled},
	} {
		text.WriteString("\n\n" + section.title)
		switch {
		case section.rows == nil:
			text.WriteString("\n未知（没有保存记录）")
		case len(section.rows) == 0:
			text.WriteString("\n（空）")
		default:
			for index, pack := range section.rows {
				fmt.Fprintf(&text, "\n%d. %s", index+1, savedPackClipboardLabel(pack.Name))
			}
		}
	}
	return text.String()
}

func (a *desktop) worldPackRows(c *ui.Context, browser *worldBrowser, i launcher.Instance, p *worldPacksView) {
	local := p.tab == worldPacksLocal
	if !local {
		if p.data.SavedErr != nil {
			feedback.Alert(c, "无法读取数据包启停记录", feedback.AlertProps{Type: feedback.Warning, Description: p.data.SavedErr.Error()})
			return
		}
		if !p.data.Saved {
			muted(c, "存档没有数据包启停记录。")
			return
		}
	}
	rows, empty := p.data.Local, "这个世界没有本地数据包文件。"
	switch p.tab {
	case worldPacksEnabled:
		rows, empty = p.data.Enabled, "游戏上次未启用任何数据包。"
	case worldPacksDisabled:
		rows, empty = p.data.Disabled, "游戏上次未停用任何数据包。"
	}
	if !local && rows == nil {
		muted(c, "存档没有保存这份启停列表。")
		return
	}
	if len(rows) == 0 {
		muted(c, empty)
		return
	}
	muted(c, "启停状态来自此世界上次保存的存档。").FontSize(12)
	const rowHeight, maxRows = 40, 5
	list := ui.List(c.Key(p), &p.scroll, len(rows))
	if local {
		list.ItemKey(func(row int) any { return rows[row].Name })
	}
	list.Height(float32(min(maxRows, len(rows)) * rowHeight)).FillWidth().Rows(func(r ui.ListRow) {
		rc, pack := r.Context, rows[r.Index]
		ui.Row(rc).Height(rowHeight).Padding(0, 8).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Column(rc).Grow(1).MinWidth(0).Children(func() {
				display.Text(rc, pack.Name, display.TextProps{Strong: true, Rows: 1}).Ellipsis("…").Selectable()
				muted(rc, worldPackKind(pack)).FontSize(12).SingleLine()
			})
			if local {
				status := p.status[pack.Name]
				if status == "" {
					status = "无保存记录"
					if p.data.Enabled == nil || p.data.Disabled == nil {
						status = "状态未知"
					}
				}
				muted(rc, status).FontSize(12).Shrink(0)
				if (pack.Kind == saves.DataPackZIP || pack.Kind == saves.DataPackFolder) &&
					action.Button(rc.Key(pack.Name), "预览", action.ButtonProps{Variant: action.Default}).
						Label("预览数据包 "+pack.Name).Disabled(a.ctx == nil).Clicked() {
					a.openWorldPackPreview(browser, i, p, pack)
				}
			}
		})
	})
}

func worldPackKind(pack saves.DataPack) string {
	switch pack.Kind {
	case saves.DataPackZIP:
		return "ZIP 文件"
	case saves.DataPackFolder:
		return "文件夹"
	case saves.DataPackMissing:
		return "本地缺失"
	case saves.DataPackNonFile:
		if pack.Name == "vanilla" {
			return "游戏内置标识"
		}
		return "来源未识别"
	default:
		return "未识别的条目"
	}
}
