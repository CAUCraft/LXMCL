package main

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/layout"
)

type modUpdatePreview struct {
	target    launcher.Instance
	request   uint64
	selected  []launcher.InstalledMod
	scroll    ui.ListState
	backup    bool
	backingUp bool
}

func (v *contentView) selectedModUpdates(files []launcher.ContentFile, skipped map[string]bool) []launcher.InstalledMod {
	var updates []launcher.InstalledMod
	for _, file := range v.selection(files) {
		mod := v.mods[file.DiskName()]
		if key := mod.UpdateKey(); key != "" && !skipped[key] {
			updates = append(updates, mod)
		}
	}
	return updates
}

func sameModUpdate(a, b launcher.InstalledMod) bool {
	fileA, _ := a.Update.DownloadFile(launcher.Mods)
	fileB, _ := b.Update.DownloadFile(launcher.Mods)
	return a.DiskName() == b.DiskName() && a.Digest == b.Digest &&
		a.Current.ProjectID == b.Current.ProjectID && a.Current.ID == b.Current.ID && a.Current.Number == b.Current.Number &&
		a.UpdateKey() == b.UpdateKey() && a.Update.Number == b.Update.Number && fileA == fileB &&
		(a.Update.Dependencies == nil) == (b.Update.Dependencies == nil) &&
		slices.Equal(a.Update.Dependencies, b.Update.Dependencies) && slices.Equal(a.Unconfirmed, b.Unconfirmed)
}

func (a *desktop) validModUpdatePreview(i launcher.Instance, p *modUpdatePreview) bool {
	v := &a.content
	if i != p.target || v.id != i.ID || v.revision != i.Revision || v.kind() != launcher.Mods ||
		a.contentRequest != p.request || v.checking || len(p.selected) == 0 {
		return false
	}
	skipped := a.state.SkippedModUpdates[i.ID]
	current := v.selectedModUpdates(v.visibleFiles(skipped), skipped)
	return slices.EqualFunc(current, p.selected, sameModUpdate)
}

func modUpdateDependencyStatus(mod launcher.InstalledMod) string {
	if n := len(mod.Unconfirmed); n != 0 {
		return fmt.Sprintf("必需前置 · %d 项未确认", n)
	}
	if mod.Update.Dependencies == nil {
		return "必需前置 · 未知"
	}
	if n := len(mod.Update.RequiredDependencies()); n != 0 {
		return fmt.Sprintf("必需前置 · 当前已匹配 %d 项", n)
	}
	return "未声明必需前置"
}

func (a *desktop) showModUpdatePreview(c *ui.Context, p *modUpdatePreview) {
	display.Text(c, "预览所选模组更新", display.TextProps{Strong: true, FontSize: 16})
	target := fmt.Sprintf("目标实例 · #%d · %s", p.target.ID, p.target.Name)
	muted(c, target).SingleLine().Ellipsis("…").Tooltip(target)
	ui.List(c.Key(p), &p.scroll, len(p.selected)).Grow(1).MinHeight(0).Rows(func(row ui.ListRow) {
		c, mod := row.Context, p.selected[row.Index]
		if row.Index > 0 {
			layout.Divider(c, layout.DividerProps{})
		}
		ui.Column(c).Gap(2).Padding(4, 0).Children(func() {
			title := mod.DiskName()
			if mod.Title != "" && mod.Title != title {
				title = mod.Title + " · " + title
			}
			display.Text(c, title, display.TextProps{Strong: true, Rows: 1}).Ellipsis("…").Tooltip(title)
			change := cmp.Or(mod.Current.Number, "未知") + " → " + cmp.Or(mod.Update.Number, "未知")
			muted(c, change).SingleLine().Ellipsis("…").Tooltip(change + " · 候选 ID " + mod.Update.ID)
			muted(c, modUpdateDependencyStatus(mod)).FontSize(12)
		})
	})
	muted(c, "前置状态基于当前已启用且识别到的模组。").FontSize(12)
	choice.Checkbox(c.Key("backup"), &p.backup, "更新前备份所选原模组到 ZIP", choice.CheckboxProps{}).Disabled(a.pickerBusy || a.busy)
	ui.Row(c).Gap(8).Children(func() {
		ui.Spacer(c).Grow(1)
		cancel := action.Button(c.Key(p), "取消", action.ButtonProps{Variant: action.Default}).AutoFocus()
		if cancel.Shortcut(0, ui.KeyEscape) || cancel.Clicked() {
			a.cancelModUpdatePreview(p)
			return
		}
		if button(c.Key("confirm"), fmt.Sprintf("确认更新 (%d)", len(p.selected)), action.Primary, a.ctx == nil || a.busy || a.running != nil || a.pickerBusy) {
			if p.backup {
				a.pickModBackupDestination(p.target, func(path string) { a.backUpBeforeModUpdate(p, path) })
			} else {
				a.content.modPreview = nil
				a.updateSelectedMods(p.target, p.selected)
			}
		}
	})
}

func (a *desktop) cancelModUpdatePreview(p *modUpdatePreview) {
	if p.backingUp && a.cancelTask != nil {
		a.taskCanceling = true
		a.cancelTask()
	}
	a.content.modPreview = nil
}

func (a *desktop) inModUpdatePreview(p *modUpdatePreview) bool {
	// The task center can monitor a confirmed backup without dismissing it.
	return a.inspected == p.target.ID && a.detailPage == detailContent &&
		(a.page == pageLibrary || p.backingUp && a.page == pageDownloads && a.downloadPage == downloadTasks)
}

func (a *desktop) currentModUpdatePreview(p *modUpdatePreview) bool {
	i, ok := a.state.Find(p.target.ID)
	return ok && a.content.modPreview == p && a.inModUpdatePreview(p) && a.validModUpdatePreview(i, p)
}

func (a *desktop) backUpBeforeModUpdate(p *modUpdatePreview, path string) {
	if !a.currentModUpdatePreview(p) || a.ctx == nil || a.ctx.Err() != nil || a.busy || a.running != nil {
		a.message("更新预览已失效，请重新选择要更新的模组。", nil)
		return
	}
	i := p.target
	files := make([]launcher.ContentFile, 0, len(p.selected))
	digests := make(map[string]string, len(p.selected))
	for _, mod := range p.selected {
		files = append(files, mod.File)
		digests[mod.DiskName()] = mod.Digest
	}
	row := taskRecord{
		InstanceID: i.ID, Revision: i.Revision, Name: i.Name, Version: i.Version,
		Action: fmt.Sprintf("更新前备份 %d 项模组", len(files)), Resource: &resourceTask{Kind: string(launcher.Mods)},
	}
	p.backingUp = true
	canceled := false
	a.runFileTask(row, func(ctx context.Context, report func(launcher.Progress)) error {
		return content.BackupVerifiedMods(ctx, i, files, digests, path, report)
	}, func(err error) {
		current := a.currentModUpdatePreview(p)
		p.backingUp = false
		if err != nil {
			return
		}
		if canceled {
			a.message("备份已保存，更新已取消。", nil)
			return
		}
		if !current {
			a.message("备份已保存，更新预览已失效，请重新选择。", nil)
			return
		}
		a.content.modPreview = nil
		a.updateSelectedMods(i, p.selected)
	})
	// Cancellation may arrive after ZIP publication, while the task is being saved.
	cancel := a.cancelTask
	a.cancelTask = func() {
		canceled = true
		cancel()
	}
}
