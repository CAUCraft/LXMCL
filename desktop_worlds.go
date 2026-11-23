package main

import (
	"context"
	"slices"
	"time"

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
	retry    *taskRecord
}

func (a *desktop) browseWorlds(i launcher.Instance, retry *taskRecord) {
	if a.ctx == nil || !a.ready || a.busy || a.running != nil {
		return
	}
	p := &worldBrowser{loading: true, selected: -1, retry: retry}
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
			if retry != nil {
				idx := slices.IndexFunc(entries, func(w saves.World) bool { return w.Folder() == retry.World })
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

func (a *desktop) backupWorld(i launcher.Instance, world saves.World) {
	root := a.store.Root
	record := taskRecord{Action: taskBackup, InstanceID: i.ID, Revision: i.Revision, Name: i.Name, Version: i.Version, World: world.Folder(), At: time.Now()}
	a.runFileTask(record, func(ctx context.Context, _ func(launcher.Progress)) error {
		_, err := saves.BackupWorld(ctx, i.GameDir, root, i.ID, world)
		return err
	}, func(err error) {
		if err == nil && a.backups != nil && a.backups.instanceID == i.ID {
			a.refreshBackups(i.ID)
		}
	})
}

func (a *desktop) worldEditor(c *ui.Context) {
	p, i := a.form.worlds, a.form.target
	current, ok := a.state.Find(i.ID)
	stale := !ok || current != i
	if p.retry != nil {
		_, canRetry := launcher.RetryTarget(*p.retry, a.state)
		stale = stale || !canRetry
	}
	panelHeading(c, lucide.FolderOpen(), i.Name, "Minecraft "+i.Version+" · "+loaderName(i))
	switch {
	case stale:
		feedback.Alert(c, "实例或启动身份已更改，请关闭列表并重新选择。", feedback.AlertProps{Type: feedback.Warning})
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
	canLaunch := p.retry == nil || p.retry.Action == taskLaunch
	canBackup := p.retry == nil || p.retry.Action == taskBackup
	supported := instance.SupportedRelease(i.Version)
	if canLaunch && !supported {
		muted(c, "LXMCL 的世界快速启动支持 1.20.1 及以后的正式版。").FontSize(12)
	}
	disabled := a.busy || a.running != nil || stale || p.selected < 0 || p.selected >= len(p.entries)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if canBackup && button(c, "备份所选世界", action.Default, disabled) {
			world := p.entries[p.selected]
			a.form.open = false
			a.backupWorld(i, world)
		}
		ui.Spacer(c).Grow(1)
		if button(c, "取消", action.Default, false) {
			a.form.open = false
		}
		if canLaunch && button(c, "进入世界", action.Primary, disabled || !supported) {
			world := p.entries[p.selected]
			a.form.open = false
			a.startGame(i, false, "", &world)
		}
	})
}
