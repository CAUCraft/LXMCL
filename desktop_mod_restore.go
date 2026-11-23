package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
)

type modRestoreView struct {
	filename string
	plan     *content.ModRestorePlan
	err      error
	selected map[string]bool
	scroll   ui.ListState
}

func (a *desktop) modRestoreButton(c *ui.Context, i launcher.Instance, disabled bool) {
	if a.content.kind() != launcher.Mods || !button(c, "从备份恢复…", action.Default, disabled) || a.ctx == nil {
		return
	}
	request := a.contentRequest
	a.pickPath(mygo.OpenDialogOptions{Title: "选择本地模组备份 ZIP", ButtonLabel: "预览备份", Filters: []mygo.FileFilter{{Name: "模组备份 ZIP", Extensions: []string{"zip"}}}}, func(path string) {
		current, ok := a.state.Find(i.ID)
		if !ok || current != i || request != a.contentRequest || a.page != pageLibrary || a.inspected != i.ID || a.detailPage != detailContent {
			return
		}
		a.previewModRestore(i, path)
	})
}

func (a *desktop) closeModRestore() {
	if a.contentCancel != nil {
		a.contentCancel()
	}
	a.content.restoreMods = nil
}

func (a *desktop) previewModRestore(i launcher.Instance, path string) {
	a.closeModRestore()
	ctx, cancel := context.WithCancel(a.ctx)
	a.contentCancel = cancel
	p := &modRestoreView{filename: filepath.Base(path), selected: make(map[string]bool)}
	a.content.restoreMods = p
	go func() {
		defer cancel()
		plan, err := content.PreviewModRestore(ctx, i, path)
		a.post(func() {
			if a.content.restoreMods == p {
				p.plan, p.err = plan, err
			}
		})
	}()
}

func (a *desktop) modRestore(c *ui.Context, i launcher.Instance, p *modRestoreView) {
	display.Text(c, "从备份恢复模组", display.TextProps{Strong: true, FontSize: 16})
	display.Text(c, fmt.Sprintf("目标 · %s · #%d · %s %s", i.Name, i.ID, i.Loader, i.Version), display.TextProps{Strong: true, Rows: 1}).Ellipsis("…").Selectable()
	muted(c, p.filename).SingleLine().Ellipsis("…").Selectable()
	switch {
	case p.err != nil:
		feedback.Alert(c, "无法预览备份", feedback.AlertProps{Type: feedback.Error, Description: p.err.Error()})
	case p.plan == nil:
		flightLoading(c, "正在读取备份并核对已装模组…")
	default:
		ui.List(c, &p.scroll, len(p.plan.Items)).Grow(1).MinHeight(0).Rows(func(row ui.ListRow) {
			c, item := row.Context, p.plan.Items[row.Index]
			disk := item.File.DiskName()
			ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(0, 8).MinHeight(44).Children(func() {
				checked := p.selected[disk]
				if choice.Checkbox(c, &checked, "", choice.CheckboxProps{}).Label("选择备份模组 " + disk).Disabled(item.Conflict != "").Changed() {
					if checked {
						p.selected[disk] = true
					} else {
						delete(p.selected, disk)
					}
				}
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					display.Text(c, item.File.Name, display.TextProps{Strong: true, Rows: 1}).Ellipsis("…").Selectable()
					status := "已启用"
					if !item.File.Enabled {
						status = "已停用"
					}
					if item.Conflict != "" {
						status = "跳过 · " + item.Conflict
					}
					muted(c, status).FontSize(12).SingleLine().Ellipsis("…").Selectable()
				})
			})
		})
		muted(c, "按原启停状态恢复。请自行核对游戏版本和前置模组。").FontSize(12)
	}
	if p.plan == nil {
		ui.Spacer(c).Grow(1)
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		muted(c, fmt.Sprintf("已选 %d 项", len(p.selected))).Grow(1)
		if button(c, "取消", action.Default, false) {
			a.closeModRestore()
			return
		}
		if button(c, "恢复所选", action.Primary, len(p.selected) == 0 || a.busy || a.running != nil || a.pickerBusy) {
			a.closeModRestore()
			a.restoreSelectedMods(i, p)
		}
	})
}

func (a *desktop) restoreSelectedMods(i launcher.Instance, p *modRestoreView) {
	row := taskRecord{
		InstanceID: i.ID, Revision: i.Revision, Name: i.Name, Version: i.Version,
		Action: "从备份恢复模组", Resource: &resourceTask{Kind: string(launcher.Mods)},
	}
	var restored, skipped int
	a.runFileTask(row, func(ctx context.Context, report func(launcher.Progress)) error {
		var err error
		restored, skipped, err = p.plan.Restore(ctx, p.selected, report)
		if err != nil {
			return fmt.Errorf("已恢复 %d 项，跳过 %d 项：%w", restored, skipped, err)
		}
		return nil
	}, func(err error) {
		if err == nil {
			message := fmt.Sprintf("%s · 已恢复 %d 项模组", i.Name, restored)
			if skipped > 0 {
				message += fmt.Sprintf("，跳过 %d 项冲突", skipped)
			}
			a.message(message, nil)
		}
		if a.content.id == i.ID && a.content.revision == i.Revision {
			a.refreshContent(i)
		}
	})
}
