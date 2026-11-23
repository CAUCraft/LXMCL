package main

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/saves"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

type worldBrowser struct {
	entries  []saves.World
	visible  []saves.World
	query    string
	list     ui.ListState
	selected int
	loading  bool
	cancel   context.CancelFunc
	opening  bool
	openErr  string
	retry    *taskRecord
}

func (a *desktop) browseWorlds(i launcher.Instance, retry *taskRecord) {
	if a.ctx == nil || !a.ready || a.busy || a.running != nil {
		return
	}
	if old := a.form.worlds; old != nil && old.cancel != nil {
		old.cancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	p := &worldBrowser{loading: true, cancel: cancel, selected: -1, retry: retry}
	a.form = editor{mode: "worlds", open: true, target: i, worlds: p}
	go func() {
		defer cancel()
		entries, err := saves.Worlds(ctx, i.GameDir)
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
			p.entries, p.visible = entries, entries
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

func (p *worldBrowser) selectedWorld() (saves.World, bool) {
	if p.selected < 0 || p.selected >= len(p.visible) {
		return saves.World{}, false
	}
	return p.visible[p.selected], true
}

func (a *desktop) openWorldDirectory(p *worldBrowser, i launcher.Instance, world saves.World) {
	p.opening, p.openErr = true, ""
	finish := func(err error) {
		if !a.form.open || a.form.worlds != p {
			return
		}
		p.opening = false
		if selected, ok := p.selectedWorld(); err != nil && ok && selected.Folder() == world.Folder() {
			p.openErr = err.Error()
			p.list.ScrollIntoView(p.selected)
		}
	}
	go func() {
		err := world.Check(i.GameDir)
		a.post(func() {
			if !a.form.open || a.form.worlds != p {
				return
			}
			current, found := a.state.Find(i.ID)
			selected, ok := p.selectedWorld()
			if !found || current != i || !ok || selected.Folder() != world.Folder() {
				finish(nil)
				return
			}
			if err != nil {
				finish(err)
				return
			}
			go func() {
				err := mygo.Shell.OpenPath(filepath.Join(i.GameDir, "saves", world.Folder()))
				a.post(func() { finish(err) })
			}()
		})
	}()
}

func (p *worldBrowser) filter() {
	selected, _ := p.selectedWorld()
	query := strings.ToLower(strings.TrimSpace(p.query))
	p.visible = p.entries
	if query != "" {
		p.visible = nil
		for _, world := range p.entries {
			if strings.Contains(strings.ToLower(world.Folder()), query) {
				p.visible = append(p.visible, world)
			}
		}
	}
	p.selected = slices.IndexFunc(p.visible, func(world saves.World) bool { return world.Folder() == selected.Folder() })
	p.list = ui.ListState{}
}

func (a *desktop) worldEditor(c *ui.Context) {
	p, i := a.form.worlds, a.form.target
	current, ok := a.state.Find(i.ID)
	stale := !ok || current != i
	if p.retry != nil {
		_, canRetry := launcher.RetryTarget(*p.retry, a.state)
		stale = stale || !canRetry
	}
	previous, _ := p.selectedWorld()
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
		if p.retry == nil {
			queryBefore := p.query
			field.SearchBar(c, &p.query, field.SearchBarProps{Placeholder: "搜索世界目录"}).Label("搜索世界目录")
			if p.query != queryBefore {
				p.filter()
			}
		}
		if len(p.visible) == 0 {
			muted(c, "没有匹配的世界目录。")
		} else {
			p.list.Selected = &p.selected
			p.list.Label = func(idx int) string { return p.visible[idx].Folder() }
			maxRows := 5
			if p.openErr != "" {
				maxRows = 4
			}
			const rowHeight = 40
			ui.List(c, &p.list, len(p.visible)).Height(float32(min(maxRows, len(p.visible)) * rowHeight)).FillWidth().Rows(func(r ui.ListRow) {
				world := p.visible[r.Index]
				name := world.LevelName()
				if name == "" {
					name = world.Folder()
				}
				played := "未知"
				if t := world.LastPlayed(); !t.IsZero() {
					played = t.Local().Format("2006-01-02 15:04")
				}
				rc := r.Context
				ui.Row(rc).Height(rowHeight).AlignItems(ui.Center).Gap(8).Padding(0, 10).Children(func() {
					ui.Icon(rc, lucide.Folder()).FontSize(16)
					ui.Column(rc).Grow(1).MinWidth(0).Children(func() {
						ui.Text(rc, name).SingleLine().Ellipsis("…")
						if name != world.Folder() {
							ui.Text(rc, "目录 · "+world.Folder()).FontSize(12).Opacity(0.8).SingleLine().Ellipsis("…")
						}
					})
					ui.Text(rc, "上次游玩 · "+played).FontSize(12).Opacity(0.8).Shrink(0).SingleLine()
					ui.Box(rc).Size(15, 15).Shrink(0).Children(func() {
						if r.Selected() {
							ui.Icon(rc, lucide.Check()).FontSize(15)
						}
					})
				})
			})
		}
	}
	world, selected := p.selectedWorld()
	if previous.Folder() != world.Folder() {
		p.openErr = ""
	}
	if p.openErr != "" && !stale {
		feedback.Alert(c, "无法打开世界目录", feedback.AlertProps{Type: feedback.Warning, Variant: feedback.Plain, Description: p.openErr})
	}
	canLaunch := p.retry == nil || p.retry.Action == taskLaunch
	canBackup := p.retry == nil || p.retry.Action == taskBackup
	supported := instance.SupportedRelease(i.Version)
	if canLaunch && !supported {
		muted(c, "LXMCL 的世界快速启动支持 1.20.1 及以后的正式版。").FontSize(12)
	}
	disabled := a.busy || a.running != nil || stale || a.form.err != "" || p.loading || !selected
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if p.retry == nil && button(c, "打开世界目录", action.Default, disabled || p.opening) {
			a.openWorldDirectory(p, i, world)
		}
		if canBackup && button(c, "备份所选世界", action.Default, disabled) {
			a.form.open = false
			a.backupWorld(i, world)
		}
		ui.Spacer(c).Grow(1)
		if button(c, "取消", action.Default, false) {
			a.form.open = false
		}
		if canLaunch && button(c, "进入世界", action.Primary, disabled || !supported) {
			a.form.open = false
			a.startGame(i, false, "", &world)
		}
	})
}
