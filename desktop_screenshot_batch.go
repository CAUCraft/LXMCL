package main

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/screenshots"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/display"
)

func (p *screenshotGallery) batchSelection(rows []display.ListItem) []screenshots.Entry {
	var selected []screenshots.Entry
	for _, row := range rows {
		if entry, ok := p.entries[row.Key]; ok && p.batch[row.Key] {
			selected = append(selected, entry)
		}
	}
	return selected
}

func (p *screenshotGallery) restrictBatchSelection(rows []display.ListItem) {
	if len(p.batch) == 0 {
		return
	}
	visible := make(map[string]bool, len(rows))
	for _, row := range rows {
		visible[row.Key] = true
	}
	for name := range p.batch {
		if !visible[name] {
			delete(p.batch, name)
		}
	}
}

func (a *desktop) screenshotBatchActions(c *ui.Context, p *screenshotGallery, rows []display.ListItem) {
	disabled := a.busy || a.pickerBusy || a.ctx == nil
	n := len(p.batchSelection(rows))
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		all := n > 0 && n == len(rows)
		if choice.Checkbox(c, &all, "全选当前结果", choice.CheckboxProps{Indeterminate: n > 0 && n < len(rows)}).
			Disabled(disabled || len(rows) == 0).Changed() {
			clear(p.batch)
			if all {
				for _, row := range rows {
					p.batch[row.Key] = true
				}
			}
			n = len(p.batchSelection(rows))
		}
		ui.Spacer(c).Grow(1)
		if button(c, fmt.Sprintf("导出所选 (%d)…", n), action.Primary, disabled || n == 0) {
			a.exportScreenshotBatch(p)
		}
	})
}

func (a *desktop) screenshotList(c *ui.Context, p *screenshotGallery, rows []display.ListItem) {
	show := func() {
		if name := display.List(c, display.ListProps{Items: rows, Active: p.selected, Compact: true}); name != "" && name != p.selected {
			a.selectScreenshot(p, name)
		}
	}
	if p.batch == nil {
		show()
		return
	}
	ui.Column(c).Gap(2).Children(func() {
		for _, item := range rows {
			rc := c.Key(item.Key)
			ui.Row(rc).Gap(4).AlignItems(ui.Center).Children(func() {
				marked := p.batch[item.Key]
				if choice.Checkbox(rc, &marked, "", choice.CheckboxProps{}).Label("选择截图 " + item.Key).
					Disabled(a.busy || a.pickerBusy).Changed() {
					if marked {
						p.batch[item.Key] = true
					} else {
						delete(p.batch, item.Key)
					}
					c.Invalidate()
				}
				ui.Column(rc).Grow(1).MinWidth(0).Children(func() {
					if name := display.List(rc, display.ListProps{Items: []display.ListItem{item}, Active: p.selected, Compact: true}); name != "" && name != p.selected {
						a.selectScreenshot(p, name)
					}
				})
			})
		}
	})
}

func (a *desktop) exportScreenshotBatch(p *screenshotGallery) {
	i := p.instance
	selected := p.batchSelection(p.visibleRows())
	if len(selected) == 0 {
		return
	}
	names := make(map[string]bool, len(selected))
	for _, entry := range selected {
		names[entry.Name] = true
	}
	a.pickPath(mygo.OpenDialogOptions{Title: "选择截图导出目录", ButtonLabel: "导出到此处", Directory: true, CreateDirectories: true}, func(folder string) {
		live, ok := a.state.Find(i.ID)
		if a.ctx == nil || a.busy || p.exporting || !ok || live != i || a.screenshots != p ||
			a.page != pageLibrary || a.detailPage != detailScreenshots || a.inspected != i.ID {
			return
		}
		current := p.batchSelection(p.visibleRows())
		// Reordering the same selection keeps the original export request valid.
		if len(current) != len(selected) || slices.ContainsFunc(current, func(e screenshots.Entry) bool { return !names[e.Name] }) {
			a.message("截图选择已变化，请重新选择。", nil)
			return
		}
		a.runScreenshotExport(i, selected, folder)
	})
}

func (a *desktop) runScreenshotExport(i launcher.Instance, selected []screenshots.Entry, folder string) {
	row := taskRecord{InstanceID: i.ID, Revision: i.Revision, Name: i.Name, Version: i.Version, Action: "批量导出截图"}
	var result screenshots.BatchExport
	a.runFileTaskWithResult(row, func(ctx context.Context, report func(launcher.Progress)) error {
		outcome, err := screenshots.ExportBatch(ctx, i.GameDir, selected, folder, func(name string, done int) {
			report(launcher.Progress{Stage: "导出 " + name, Done: done, Total: len(selected)})
		})
		result = outcome
		return err
	}, func(row *taskRecord) {
		summary := screenshotExportSummary(result, len(selected))
		if row.Status == taskFailed && row.Detail != "" {
			summary += "\n" + row.Detail
		}
		row.Detail = summary
	}, func(err error) {
		summary := screenshotExportSummary(result, len(selected))
		if errors.Is(err, context.Canceled) {
			a.message("导出已取消 · "+summary, nil)
		} else {
			a.message(summary, err)
		}
	})
}

func screenshotExportSummary(result screenshots.BatchExport, total int) string {
	s := fmt.Sprintf("已导出 %d 张 · 同名跳过 %d 张", result.Exported, result.Skipped)
	if result.Failed > 0 {
		s += fmt.Sprintf(" · 失败 %d 张", result.Failed)
	}
	if remaining := total - result.Exported - result.Skipped - result.Failed; remaining > 0 {
		s += fmt.Sprintf(" · 未处理 %d 张", remaining)
	}
	return s
}
