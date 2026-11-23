package main

import (
	"slices"

	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/saves"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

type worldBrowser struct {
	entries  []saves.World
	list     ui.ListState
	selected int
	loading  bool
}

func (a *desktop) browseWorlds(i launcher.Instance, retryFolder string) {
	if a.ctx == nil || !a.ready || a.busy || a.running != nil {
		return
	}
	p := &worldBrowser{loading: true, selected: -1}
	a.form = editor{mode: "worlds", open: true, target: i, worlds: p}
	go func() {
		entries, err := saves.Worlds(a.ctx, i.GameDir)
		a.post(func() {
			if !a.form.open || a.form.worlds != p {
				return
			}
			p.loading = false
			if err != nil {
				a.form.err = err.Error()
				return
			}
			if retryFolder != "" {
				idx := slices.IndexFunc(entries, func(w saves.World) bool { return w.Folder() == retryFolder })
				if idx < 0 {
					a.form.err = "原任务的世界已移动，请从实例的游戏文件页重新选择。"
					return
				}
				entries = entries[idx : idx+1]
				p.selected = 0
			}
			p.entries = entries
		})
	}()
}

func (a *desktop) worldEditor(c *ui.Context) {
	p, i := a.form.worlds, a.form.target
	current, ok := a.state.Find(i.ID)
	stale := !ok || current != i
	panelHeading(c, lucide.FolderOpen(), i.Name, "Minecraft "+i.Version+" · "+loaderName(i))
	switch {
	case stale:
		feedback.Alert(c, "实例配置已更改，请关闭列表并重新选择。", feedback.AlertProps{Type: feedback.Warning})
	case a.form.err != "":
		feedback.Alert(c, a.form.err, feedback.AlertProps{Type: feedback.Warning})
	case p.loading:
		flightLoading(c, "正在读取本地世界…")
	case len(p.entries) == 0:
		muted(c, "没有找到本地世界。可在游戏内创建世界，或从备份恢复。")
	default:
		display.Text(c, "世界目录", display.TextProps{Strong: true})
		p.list.Selected = &p.selected
		p.list.Label = func(idx int) string { return p.entries[idx].Folder() }
		ui.List(c, &p.list, len(p.entries)).Height(float32(min(5, len(p.entries))) * 36).FillWidth().Rows(func(r ui.ListRow) {
			rc := r.Context
			ui.Row(rc).Height(36).AlignItems(ui.Center).Gap(8).Padding(0, 10).Children(func() {
				ui.Icon(rc, lucide.Folder()).FontSize(16)
				ui.Text(rc, p.entries[r.Index].Folder()).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
				if r.Selected() {
					ui.Icon(rc, lucide.Check()).FontSize(15)
				}
			})
		})
	}
	supported := instance.SupportedRelease(i.Version)
	if !supported {
		muted(c, "LXMCL 的世界快速启动支持 1.20.1 及以后的正式版。").FontSize(12)
	}
	disabled := a.busy || a.running != nil || stale || p.selected < 0 || p.selected >= len(p.entries)
	ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
		if button(c, "取消", action.Default, false) {
			a.form.open = false
		}
		if button(c, "进入世界", action.Primary, disabled || !supported) {
			world := p.entries[p.selected]
			a.form.open = false
			a.startGame(i, false, "", &world)
		}
	})
}
