package main

import (
	"context"
	"errors"
	"fmt"
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
)

type prismDraft struct {
	source                   content.PrismInstance
	group                    string
	worlds, options, reading bool
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
		*d = prismDraft{reading: true}
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
				d.source, d.worlds, d.options = p, p.Worlds, p.Options
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
		muted(c, p.Source).FontSize(12).SingleLine().Ellipsis("…")
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			display.Text(c, "实例名称", display.TextProps{}).SingleLine().Shrink(0)
			field.Input(c, &a.form.name, field.InputProps{Placeholder: "实例名称"}).Label("导入后的实例名称").Grow(1).MinWidth(0)
			if len(groups) > 0 || d.group != "" {
				display.Text(c, "分组", display.TextProps{}).SingleLine().Shrink(0)
				groupDestinationPicker(c.Key("prism-group"), &d.group, groups, "选择导入实例分组", wait).Width(140).Shrink(0)
			}
		})
		if p.Worlds {
			choice.Checkbox(c, &d.worlds, "存档（saves）", choice.CheckboxProps{})
		}
		if p.Options {
			choice.Checkbox(c, &d.options, "游戏选项（options.txt）", choice.CheckboxProps{})
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
		if !p.Worlds && !p.Options {
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
		if button(c, "确认导入", action.Primary, wait || p.Version == "" || groupMissing || strings.TrimSpace(a.form.name) == "") {
			a.importPrism(d)
		}
	})
}

func (a *desktop) importPrism(d *prismDraft) {
	if a.ctx == nil || !a.ready || a.busy || a.pickerBusy || d.reading || !a.form.open || a.form.prism != d {
		return
	}
	if d.group != "" && !slices.Contains(a.state.GroupNames(), d.group) {
		return
	}
	p, worlds, opts := d.source, d.worlds, d.options
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
		err := store.ImportPrism(ctx, next, target, p, worlds, opts, func(progress launcher.Progress) {
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
				if errors.Is(err, context.Canceled) {
					a.message("本地实例导入已取消", nil)
				} else {
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
