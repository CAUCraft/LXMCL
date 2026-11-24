package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/overlay"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

const (
	prismAllWorlds = iota
	prismSelectedWorlds
	prismNoWorlds
)

type prismDraft struct {
	source                    content.PrismInstance
	group                     string
	worldMode                 int
	options, servers, reading bool
	worldPreview              prismWorldPreview
}

type prismWorldPreview struct {
	folders     []string
	selected    map[string]bool
	known, open bool
	list        ui.ListState
}

func (a *desktop) startPrismImport() {
	d := &prismDraft{}
	a.form = editor{mode: "prism", open: true, prism: d}
}

func (a *desktop) choosePrismSource(d *prismDraft, archive bool) {
	options := mygo.OpenDialogOptions{Title: "选择 Prism 或 MultiMC 实例目录", Directory: true, ButtonLabel: "预览实例"}
	read := content.ReadPrismInstance
	if archive {
		options = mygo.OpenDialogOptions{Title: "选择 Prism 或 MultiMC 实例 ZIP", ButtonLabel: "预览实例", Filters: []mygo.FileFilter{{Name: "实例 ZIP", Extensions: []string{"zip"}}}}
		read = content.ReadPrismZIP
	}
	a.pickPath(options, func(path string) {
		if !a.form.open || a.form.prism != d {
			return
		}
		*d = prismDraft{reading: true, worldMode: d.worldMode}
		a.form.err = ""
		go func() {
			p, err := read(a.ctx, path)
			a.post(func() {
				if !a.form.open || a.form.prism != d {
					return
				}
				d.reading = false
				if err != nil {
					a.form.err = err.Error()
					return
				}
				d.source, d.options = p, p.Options
				d.worldPreview.folders, d.worldPreview.known = p.WorldFolders()
				a.form.name = availableInstanceName(a.state, p.Name, "原版实例")
			})
		}()
	})
}

func (a *desktop) prismEditor(c *ui.Context) {
	d := a.form.prism
	wait := d.reading || a.pickerBusy || a.busy
	p := d.source
	groups := a.state.GroupNames()
	targetReady := false
	ui.Row(c).Gap(8).Children(func() {
		if button(c, "选择实例目录…", action.Default, wait) {
			a.choosePrismSource(d, false)
		}
		if button(c, "选择实例 ZIP…", action.Default, wait) {
			a.choosePrismSource(d, true)
		}
	})
	if d.reading {
		flightLoading(c, "正在读取实例配置…")
	} else if p.Version != "" {
		display.Text(c, "Minecraft "+p.Version+" · 原版", display.TextProps{Strong: true})
		muted(c, "来源 · "+p.Source).FontSize(12).SingleLine().Ellipsis("…").Tooltip(p.Source)
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			display.Text(c, "实例名称", display.TextProps{}).SingleLine().Shrink(0)
			field.Input(c, &a.form.name, field.InputProps{Placeholder: "实例名称"}).Label("导入后的实例名称").Grow(1).MinWidth(0)
			if len(groups) > 0 || d.group != "" {
				display.Text(c, "分组", display.TextProps{}).SingleLine().Shrink(0)
				groupDestinationPicker(c.Key("prism-group"), &d.group, groups, "选择导入实例分组", wait).Width(140).Shrink(0)
			}
		})
		target, err := a.state.PreviewNewInstance(a.form.name, p.Version)
		targetReady = err == nil
		prismTargetPreview(c, target.GameDir, err)
		if p.Worlds || d.worldMode == prismSelectedWorlds {
			d.renderWorlds(c, wait)
		} else if p.Options || p.Servers {
			muted(c, "没有存档目录").FontSize(12)
		}
		if p.Options {
			choice.Checkbox(c, &d.options, "游戏选项（options.txt）", choice.CheckboxProps{})
		}
		if p.Servers {
			choice.Checkbox(c, &d.servers, "游戏内服务器列表", choice.CheckboxProps{})
		}
		var settings []string
		if p.Launch.Memory != 0 {
			settings = append(settings, fmt.Sprintf("内存上限 %d MiB", p.Launch.Memory))
		}
		if p.Launch.Display != "" {
			settings = append(settings, fmt.Sprintf("窗口 %d × %d", p.Launch.Width, p.Launch.Height))
		}
		if len(settings) > 0 {
			muted(c, strings.Join(settings, " · ")).FontSize(13)
		}
		if p.SkippedSettings {
			muted(c, "未支持的内存或窗口值将继承启动器设置。").FontSize(12)
		}
		if !p.Worlds && !p.Options && !p.Servers {
			muted(c, "此实例没有可复制的存档或游戏选项。").FontSize(12)
		}
	} else if a.form.err == "" {
		muted(c, "选择含 instance.cfg 和 mmc-pack.json 的原版实例目录或 ZIP。").FontSize(13)
	}
	if a.form.err != "" {
		feedback.Alert(c, a.form.err, feedback.AlertProps{Type: feedback.Error})
	}
	groupMissing := d.group != "" && !slices.Contains(groups, d.group)
	if groupMissing {
		feedback.Alert(c, i18n.Text("instance.group_missing"), feedback.AlertProps{Type: feedback.Warning})
	}
	muted(c, "创建独立实例。账号、Java 和启动命令保留在原启动器。").FontSize(12)
	ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
		if button(c, "取消", action.Default, false) {
			a.form.open = false
		}
		worldsMissing := d.worldMode == prismSelectedWorlds && len(d.worldPreview.selected) == 0
		if button(c, "确认导入", action.Primary, wait || !targetReady || groupMissing || worldsMissing) {
			a.importPrism(d)
		}
	})
}

func prismTargetPreview(c *ui.Context, directory string, err error) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		display.Text(c, "目标目录（预览）", display.TextProps{}).SingleLine().Shrink(0)
		if err != nil {
			muted(c, err.Error()).FontSize(12).SingleLine().Ellipsis("…").Tooltip(err.Error()).Grow(1).MinWidth(0)
			return
		}
		base := filepath.Base(directory)
		ui.Row(c).AlignItems(ui.Center).Grow(1).MinWidth(0).Tooltip(directory).Children(func() {
			muted(c, strings.TrimSuffix(directory, base)).FontSize(12).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
			muted(c, base).FontSize(12).SingleLine().Shrink(0)
		})
		action.CopyButton(c, directory, action.CopyButtonProps{Size: theme.Small}).Label("复制目标目录").Shrink(0)
	})
}

func (d *prismDraft) renderWorlds(c *ui.Context, disabled bool) {
	p := &d.worldPreview
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		display.Text(c, "存档", display.TextProps{}).SingleLine().Shrink(0)
		previous := d.worldMode
		action.Segmented(c, &d.worldMode, []string{"全部导入", "只导入所选", "不导入"},
			action.SegmentedProps{Size: theme.Small}).Label("存档导入范围").Disabled(disabled).Shrink(0)
		if d.worldMode != previous {
			p.open, p.list = false, ui.ListState{}
			clear(p.selected)
		}
		ui.Spacer(c).Grow(1)
		if !d.source.Worlds {
			muted(c, "没有存档目录").FontSize(12)
			return
		}
		p.render(c.Key("prism-world-folders"), d.worldMode == prismSelectedWorlds, disabled, "仅复制勾选目录及其内容")
	})
}

func (p *prismWorldPreview) render(c *ui.Context, picking, disabled bool, selectionHint string) {
	if !p.known {
		muted(c, "无法完整确认存档目录").FontSize(12)
		return
	}
	if len(p.folders) == 0 {
		muted(c, "没有存档目录").FontSize(12)
		return
	}
	title, label := fmt.Sprintf("%d 个存档目录", len(p.folders)), "查看存档目录"
	if picking {
		title, label = "选择存档目录…", "选择存档目录"
		if n := len(p.selected); n > 0 {
			title = fmt.Sprintf("已选 %d 项", n)
		}
	}
	anchor := action.Button(c, title, action.ButtonProps{Variant: action.Default}).Label(label).Disabled(disabled).Shrink(0)
	overlay.Popover(c, anchor, &p.open, overlay.PopoverProps{Placement: overlay.Bottom}, func() {
		const rowHeight = 28
		p.list.Label = func(n int) string { return p.folders[n] }
		ui.Column(c).Width(294).Gap(6).Children(func() {
			if picking {
				muted(c, selectionHint).FontSize(12)
			}
			ui.List(c, &p.list, len(p.folders)).
				Height(float32(min(5, len(p.folders)) * rowHeight)).FillWidth().Rows(func(row ui.ListRow) {
				rc := row.Context
				folder := p.folders[row.Index]
				ui.Row(rc).Height(rowHeight).Padding(0, 8).AlignItems(ui.Center).Children(func() {
					if !picking {
						ui.Text(rc, folder).SingleLine().Ellipsis("…").Tooltip(folder)
						return
					}
					checked := p.selected[folder]
					if choice.Checkbox(rc, &checked, folder, choice.CheckboxProps{}).
						Label("选择存档 " + folder).Tooltip(folder).MinWidth(0).Grow(1).Clip().Changed() {
						if checked {
							if p.selected == nil {
								p.selected = make(map[string]bool)
							}
							p.selected[folder] = true
						} else {
							delete(p.selected, folder)
						}
					}
				})
			})
			if picking {
				ui.Row(c).Justify(ui.End).Children(func() {
					if button(c, "完成", action.Default, false) {
						p.open = false
					}
				})
			}
		})
	})
}

func (a *desktop) importPrism(d *prismDraft) {
	if a.ctx == nil || !a.ready || a.busy || a.pickerBusy || d.reading || !a.form.open || a.form.prism != d {
		return
	}
	if d.group != "" && !slices.Contains(a.state.GroupNames(), d.group) {
		return
	}
	p := d.source
	files := content.PrismFiles{Options: d.options, Servers: d.servers}
	switch d.worldMode {
	case prismNoWorlds:
	case prismAllWorlds:
		files.Worlds = p.Worlds
	case prismSelectedWorlds:
		if !p.Worlds || len(d.worldPreview.selected) == 0 {
			return
		}
		files.Worlds = true
		files.WorldFolders = slices.Sorted(maps.Keys(d.worldPreview.selected))
	default:
		return
	}
	next, target, err := a.state.PrepareInstallation(a.form.name, p.Version, launcher.LoaderVanilla, "")
	if err != nil {
		a.form.err = err.Error()
		return
	}
	target.Launch = p.Launch
	next.Instances[len(next.Instances)-1] = target
	if d.group != "" {
		next.InstanceGroups[target.ID] = d.group
	}
	ctx, cancel := context.WithCancel(a.ctx)
	store, game := a.store, a.game
	a.form.open = false
	a.busy, a.taskName, a.cancelTask, a.taskCanceling = true, target.Name, cancel, false
	a.task, a.notice = launcher.Progress{Stage: "正在导入本地实例"}, ""
	fromPage, fromID := a.page, a.inspected
	record := (taskRecord{Name: target.Name, Version: target.Version, Action: "导入本地实例", At: time.Now()}).Submitted()
	a.writes.Go(func() {
		defer cancel()
		var last time.Time
		err := store.ImportPrismFiles(ctx, next, target, p, files, func(progress launcher.Progress) {
			if time.Since(last) >= 140*time.Millisecond {
				last = time.Now()
				a.post(func() { a.task = progress })
			}
		})
		cached := false
		if err == nil {
			cached = game.Installed(target)
			record.InstanceID, record.Revision, record.Status, record.Detail = target.ID, target.Revision, taskCompleted, "来自 "+filepath.Base(p.Source)
		}
		render := func() {
			a.busy, a.cancelTask, a.task, a.taskCanceling = false, nil, launcher.Progress{}, false
			if err != nil {
				switch {
				case errors.Is(err, context.Canceled):
					a.message("本地实例导入已取消", nil)
				case errors.Is(err, content.ErrPrismWorldsChanged) &&
					a.page == fromPage && a.inspected == fromID && !a.form.open && a.form.prism == d:
					*d = prismDraft{worldMode: prismSelectedWorlds}
					a.form.open, a.form.err = true, content.ErrPrismWorldsChanged.Error()
				default:
					a.message("无法导入本地实例", err)
				}
				return
			}
			a.state = next
			if a.installed == nil {
				a.installed = make(map[int]bool)
			}
			a.installed[target.ID] = cached
			if a.page == fromPage && a.inspected == fromID {
				a.inspect(target)
			}
			a.message("已导入独立实例 · "+target.Name, nil)
		}
		if err != nil {
			a.post(render)
			return
		}
		a.saveTask(record, render)
	})
}
