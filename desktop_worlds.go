package main

import (
	"cmp"
	"context"
	"path/filepath"
	"slices"
	"strings"

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
	entries    []saves.World
	visible    []saves.World
	query      string
	modeFilter string
	order      string
	list       ui.ListState
	selected   int
	loading    bool
	cancel     context.CancelFunc
	opening    bool
	openErr    string
	pinning    bool
	pinErr     string
	retry      *taskRecord
	copy       *worldCopyView
	packs      *worldPacksView
	thumbnail  *worldThumbnail
	usage      *worldUsage
}

func (a *desktop) browseWorlds(i launcher.Instance, retry *taskRecord) {
	if a.ctx == nil || !a.ready || a.busy || a.running != nil {
		return
	}
	if old := a.form.worlds; old != nil {
		old.closeWorldCopy()
		old.closeWorldPacks()
		old.closeWorldThumbnail()
		old.closeWorldUsage()
		if old.cancel != nil {
			old.cancel()
		}
	}
	p := &worldBrowser{selected: -1, retry: retry}
	a.form = editor{mode: "worlds", open: true, target: i, worlds: p}
	a.readWorlds(p, i)
}

func (a *desktop) readWorlds(p *worldBrowser, i launcher.Instance) {
	previous, _ := p.selectedWorld()
	p.loading, p.openErr, p.pinErr, a.form.err = true, "", "", ""
	p.closeWorldThumbnail()
	p.closeWorldUsage()
	ctx, cancel := context.WithCancel(a.ctx)
	p.cancel = cancel
	go func() {
		defer cancel()
		entries, err := saves.Worlds(ctx, i.GameDir)
		a.post(func() {
			if !a.form.open || a.form.worlds != p || a.form.target != i {
				return
			}
			p.loading = false
			if err != nil {
				a.form.err = err.Error()
				return
			}
			if p.retry != nil {
				idx := slices.IndexFunc(entries, func(w saves.World) bool { return w.Folder() == p.retry.World })
				if idx < 0 {
					a.form.err = "原任务的世界已移动，请从实例的游戏文件页重新选择。"
					return
				}
				p.entries, p.visible = entries[idx:idx+1], entries[idx:idx+1]
				p.selected = 0
				return
			}
			p.entries, p.visible = entries, entries
			p.selected = slices.IndexFunc(entries, func(w saves.World) bool { return w.Folder() == previous.Folder() })
			p.filter(a.state.PinnedWorlds[i.ID])
		})
	}()
}

func (a *desktop) backupWorld(i launcher.Instance, world saves.World) {
	a.runSaveBackup(i, world.Folder(), func(ctx context.Context, root string) (string, error) {
		return saves.BackupWorld(ctx, i.GameDir, root, i.ID, world)
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

func (p *worldBrowser) filter(pinned map[string]bool) {
	selected, _ := p.selectedWorld()
	query := strings.ToLower(strings.TrimSpace(p.query))
	filtered := query != "" || p.modeFilter != ""
	p.visible = p.entries
	if filtered {
		p.visible = nil
		for _, world := range p.entries {
			if p.modeFilter != "" && worldModeLabel(world.DefaultGameMode()) != p.modeFilter {
				continue
			}
			if query == "" || strings.Contains(strings.ToLower(world.Folder()), query) ||
				strings.Contains(strings.ToLower(world.LevelName()), query) {
				p.visible = append(p.visible, world)
			}
		}
	}
	if p.order == sortRecent || len(pinned) > 0 {
		if !filtered {
			// Preserve the directory order shared with the default view.
			p.visible = slices.Clone(p.visible)
		}
		slices.SortStableFunc(p.visible, func(a, b saves.World) int {
			if pa, pb := pinned[a.Folder()], pinned[b.Folder()]; pa != pb {
				if pa {
					return -1
				}
				return 1
			}
			if p.order == sortRecent {
				return b.LastPlayed().Compare(a.LastPlayed())
			}
			return 0
		})
	}
	p.selected = slices.IndexFunc(p.visible, func(world saves.World) bool { return world.Folder() == selected.Folder() })
	p.list = ui.ListState{}
	if p.selected >= 0 {
		p.list.ScrollIntoView(p.selected)
	}
}

var worldModeOptions = func() []field.SelectOption {
	options := []field.SelectOption{{Label: "全部模式"}}
	for _, mode := range []saves.GameMode{
		saves.GameModeSurvival, saves.GameModeCreative, saves.GameModeAdventure,
		saves.GameModeSpectator, saves.GameModeUnknown,
	} {
		label := worldModeLabel(mode)
		options = append(options, field.SelectOption{Value: label, Label: label})
	}
	return options
}()

func worldSavedVersionLabel(world saves.World) string {
	return "存档保存版本 · " + cmp.Or(world.SavedVersion(), "未知")
}

func (a *desktop) worldEditor(c *ui.Context) {
	p, i := a.form.worlds, a.form.target
	if p.copy != nil {
		p.closeWorldUsage()
		a.worldCopyEditor(c, p, i, p.copy)
		return
	}
	if p.packs != nil {
		p.closeWorldUsage()
		a.worldPacksEditor(c, p, i, p.packs)
		return
	}
	current, ok := a.state.Find(i.ID)
	stale := !ok || current != i
	if p.retry != nil {
		_, canRetry := launcher.RetryTarget(*p.retry, a.state)
		stale = stale || !canRetry
	}
	previous, _ := p.selectedWorld()
	pinned := a.state.PinnedWorlds[i.ID]
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			panelHeading(c, lucide.FolderOpen(), i.Name, "实例配置版本 · Minecraft "+i.Version+" · "+loaderName(i))
		})
		if p.retry == nil && icon(c.Key("refresh"), lucide.RefreshCw(), "刷新存档", stale || p.loading || p.opening || p.pinning || a.busy || a.running != nil || a.ctx == nil) {
			a.readWorlds(p, i)
		}
	})
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
			queryBefore, modeBefore, orderBefore := p.query, p.modeFilter, p.order
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					field.SearchBar(c, &p.query, field.SearchBarProps{Placeholder: "搜索世界名或目录名"}).Label("搜索世界名或目录名")
				})
				field.Select(c.Key("world-mode-filter"), &p.modeFilter, field.SelectProps{Options: worldModeOptions}).
					Label("默认游戏模式").Width(112)
				field.Select(c, &p.order, field.SelectProps{Options: []field.SelectOption{
					{Value: "", Label: "按目录名"},
					{Value: sortRecent, Label: "最近游玩"},
				}}).Label("存档排序").Width(112)
			})
			if p.query != queryBefore || p.modeFilter != modeBefore || p.order != orderBefore {
				p.filter(pinned)
			}
		}
		if len(p.visible) == 0 {
			muted(c, "没有匹配的世界。")
		} else {
			p.list.Selected = &p.selected
			p.list.Key = func(idx int) any { return p.visible[idx].Folder() }
			p.list.Label = func(idx int) string { return p.visible[idx].Folder() }
			maxRows := 5
			if p.openErr != "" || p.pinErr != "" {
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
					if p.retry == nil && pinned[world.Folder()] {
						ui.Icon(rc, lucide.Pin()).FontSize(16).Label("已置顶 " + world.Folder())
					} else {
						ui.Icon(rc, lucide.Folder()).FontSize(16)
					}
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
		p.openErr, p.pinErr = "", ""
	}
	if selected && !stale && !p.loading && a.form.err == "" {
		a.worldSelectionDetails(c, p, i, world)
	} else {
		p.closeWorldThumbnail()
		p.closeWorldUsage()
	}
	if p.openErr != "" && !stale {
		feedback.Alert(c, "无法打开世界目录", feedback.AlertProps{Type: feedback.Warning, Variant: feedback.Plain, Description: p.openErr})
	}
	if p.pinErr != "" && !stale {
		feedback.Alert(c, "未能保存置顶状态", feedback.AlertProps{Type: feedback.Warning, Variant: feedback.Plain, Description: p.pinErr})
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
		if p.retry == nil && button(c, "复制世界…", action.Default, disabled || a.ctx == nil) {
			p.copy = &worldCopyView{world: world}
		}
		if p.retry == nil && button(c, "查看数据包", action.Default, disabled || a.ctx == nil) {
			a.openWorldPacks(p, i, world)
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
