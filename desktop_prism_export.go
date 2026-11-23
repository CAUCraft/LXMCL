package main

import (
	"context"
	"fmt"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

type prismExportDraft struct{ worlds, options bool }

func (a *desktop) prismExportEditor(c *ui.Context, d *prismExportDraft) {
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
	choice.Checkbox(c, &d.worlds, "存档（原版世界数据）", choice.CheckboxProps{}).Disabled(a.pickerBusy)
	choice.Checkbox(c, &d.options, "游戏选项（options.txt）", choice.CheckboxProps{}).Disabled(a.pickerBusy)
	if d.worlds {
		muted(c, "数据包、脚本和额外文件留在原实例。").FontSize(12)
	}
	if a.form.err != "" {
		feedback.Alert(c, a.form.err, feedback.AlertProps{Type: feedback.Error})
	}
}

func (a *desktop) savePrismExport(d *prismExportDraft) {
	if a.pickerBusy || a.busy || a.running != nil || a.ctx == nil {
		return
	}
	i := a.form.target
	worlds, options := d.worlds, d.options
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
			a.runContentTask(i, "导出实例 ZIP", nil, func(ctx context.Context, report func(launcher.Progress)) error {
				return content.ExportPrismZIP(ctx, i, filename, worlds, options, report)
			})
		})
	}()
}
