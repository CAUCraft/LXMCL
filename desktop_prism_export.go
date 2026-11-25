package main

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/tasks"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

const (
	prismExportNoWorlds = iota
	prismExportAllWorlds
	prismExportSelectedWorlds
)

type prismExportDraft struct {
	worldMode                              int
	options, servers, screenshots, reading bool
	worldPreview                           prismWorldPreview
	preview                                *content.PrismExportWorlds
}

func (d *prismExportDraft) needsWorldSelection() bool {
	return d.worldMode == prismExportSelectedWorlds && (d.preview == nil || len(d.worldPreview.selected) == 0)
}

func (a *desktop) prismExportEditor(c *ui.Context, d *prismExportDraft) bool {
	i := a.form.target
	panelHeading(c, lucide.FileArchive(), i.Name, "Minecraft "+i.Version+" · 原版")
	memory := "内存继承"
	if i.Launch.Memory != 0 {
		memory = fmt.Sprintf("内存上限 %d MiB", i.Launch.Memory)
	}
	window := "窗口继承"
	if i.Launch.Display == instance.DisplayResolution {
		window = fmt.Sprintf("窗口 %d × %d", i.Launch.Width, i.Launch.Height)
	}
	muted(c, memory+" · "+window).FontSize(13)
	wait := a.pickerBusy || a.busy || a.running != nil
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		display.Text(c, "存档", display.TextProps{}).SingleLine().Shrink(0)
		previous := d.worldMode
		action.Segmented(c, &d.worldMode, []string{"不包含", "全部包含", "只包含所选"},
			action.SegmentedProps{Size: theme.Small}).Label("存档导出范围").Disabled(wait || d.reading).Shrink(0)
		if previous != d.worldMode {
			d.preview, d.worldPreview = nil, prismWorldPreview{}
			a.form.err = ""
			if d.worldMode == prismExportSelectedWorlds {
				a.readPrismExportWorlds(d, i)
			}
		}
		if d.worldMode != prismExportSelectedWorlds {
			return
		}
		ui.Spacer(c).Grow(1)
		switch {
		case d.reading:
			muted(c, "正在读取存档…").FontSize(12)
		case d.preview == nil:
			if button(c, "重新读取存档", action.Default, wait) {
				a.readPrismExportWorlds(d, i)
			}
		case len(d.worldPreview.folders) == 0:
			muted(c, "没有可导出的存档").FontSize(12)
		default:
			d.worldPreview.render(c.Key("prism-export-worlds"), true, wait, "仅包含勾选世界的原版文件")
		}
	})
	if d.worldMode != prismExportNoWorlds {
		muted(c, "数据包、脚本和额外文件留在原实例。").FontSize(12)
	}
	choice.Checkbox(c, &d.options, "游戏选项（options.txt）", choice.CheckboxProps{}).Disabled(wait)
	choice.Checkbox(c, &d.servers, "游戏内服务器列表", choice.CheckboxProps{}).Disabled(wait)
	muted(c, "勾选后，服务器地址将写入 ZIP。").FontSize(12)
	choice.Checkbox(c, &d.screenshots, "游戏截图（PNG）", choice.CheckboxProps{}).Disabled(wait)
	if a.form.err != "" {
		feedback.Alert(c, a.form.err, feedback.AlertProps{Type: feedback.Error})
	}
	submit := false
	ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
		if button(c, "取消", action.Default, false) {
			a.form.open = false
			return
		}
		submit = button(c, "选择目标文件…", action.Primary, wait || d.needsWorldSelection())
	})
	return submit
}

func (a *desktop) readPrismExportWorlds(d *prismExportDraft, i launcher.Instance) {
	if a.ctx == nil || d.reading {
		return
	}
	d.reading, a.form.err = true, ""
	go func() {
		preview, err := content.PreviewPrismExportWorlds(a.ctx, i)
		a.post(func() {
			if !a.form.open || a.form.prismExport != d {
				return
			}
			d.reading = false
			if err != nil {
				a.form.err = err.Error()
				return
			}
			d.preview = preview
			d.worldPreview = prismWorldPreview{folders: preview.WorldFolders(), known: true}
		})
	}()
}

func (a *desktop) savePrismExport(d *prismExportDraft) {
	if d == nil || a.pickerBusy || a.busy || a.running != nil || a.ctx == nil || !a.form.open || a.form.prismExport != d {
		return
	}
	i := a.form.target
	files := content.PrismFiles{Options: d.options, Servers: d.servers, Screenshots: d.screenshots}
	var selected []string
	preview := d.preview
	switch d.worldMode {
	case prismExportNoWorlds:
	case prismExportAllWorlds:
		files.Worlds = true
	case prismExportSelectedWorlds:
		if d.needsWorldSelection() {
			a.form.err = "请至少勾选一个存档目录。"
			return
		}
		files.Worlds = true
		selected = slices.Sorted(maps.Keys(d.worldPreview.selected))
	default:
		return
	}
	name := i.Name + ".zip"
	if !storage.ValidFilename(name) {
		name = fmt.Sprintf("instance-%d.zip", i.ID)
	}
	dialog := mygo.SaveDialogOptions{Parent: a.window, Title: "导出 Prism / MultiMC 实例", DefaultPath: name, ButtonLabel: "导出", Filters: []mygo.FileFilter{{Name: "实例 ZIP", Extensions: []string{"zip"}}}}
	a.pickerBusy = true
	go func() {
		filename, err := mygo.Dialog.Save(dialog)
		a.post(func() {
			a.pickerBusy = false
			if !a.form.open || a.form.prismExport != d {
				return
			}
			if err != nil {
				a.form.err = err.Error()
				return
			}
			if filename == "" {
				return
			}
			current, ok := a.state.Find(i.ID)
			if !ok || current != i || a.running != nil || a.busy {
				a.form.err = "实例状态已变化，请重新打开导出。"
				return
			}
			a.form.open = false
			var artifact tasks.PrismExport
			a.runContentTaskWithResult(i, tasks.PrismExportAction, nil, func(ctx context.Context, report func(launcher.Progress)) error {
				result, err := content.ExportPrismTaskZIP(ctx, i, filename, files, preview, selected, report)
				artifact = result
				return err
			}, func(row *taskRecord) {
				row.PrismExport = artifact
			})
		})
	}()
}

func (a *desktop) locatePrismExport(row taskRecord) {
	if a.ctx == nil || row.PrismExportPath() == "" {
		return
	}
	go func() {
		filename, err := row.LocatePrismExport()
		a.post(func() {
			if err != nil {
				a.message("无法定位导出 ZIP", err)
				return
			}
			mygo.Shell.ShowItemInFolder(filename)
		})
	}()
}
