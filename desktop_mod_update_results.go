package main

import (
	"cmp"
	"context"
	"fmt"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/tasks"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/layout"
)

func (a *desktop) updateSelectedMods(i launcher.Instance, selected []launcher.InstalledMod) {
	items := make([]tasks.ModUpdateItem, len(selected))
	for n, mod := range selected {
		items[n] = tasks.ModUpdateItem{
			File: mod.DiskName(), From: mod.Current.Number, To: mod.Update.Number, Status: tasks.ModUpdateNotRun,
		}
	}
	game := a.game
	a.runContentTaskWithResult(i, tasks.ModUpdateAction, nil, func(ctx context.Context, report func(launcher.Progress)) error {
		return runModUpdateBatch(ctx, items, func(n int) error {
			mod := selected[n]
			report(launcher.Progress{Stage: "更新 " + mod.DiskName(), Done: n, Total: len(selected)})
			return game.UpdateMod(ctx, i, mod)
		})
	}, func(row *taskRecord) {
		row.ModUpdates = tasks.SummarizeModUpdates(items)
	})
}

func runModUpdateBatch(ctx context.Context, items []tasks.ModUpdateItem, update func(int) error) error {
	failed := 0
	for n := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		// A successful replacement stays successful if cancellation arrives afterward.
		items[n].Status = tasks.Outcome(update(n))
		switch items[n].Status {
		case tasks.Canceled:
			return context.Canceled
		case tasks.Failed:
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d 项模组未能更新，请查看逐项结果。", failed)
	}
	return nil
}

func modUpdateDetails(c *ui.Context, row taskRecord) {
	report := row.ModUpdates
	if row.Action != tasks.ModUpdateAction || row.InstanceID <= 0 || len(report.Items) == 0 && report.Omitted == 0 {
		return
	}
	layout.Divider(c, layout.DividerProps{})
	display.Text(c, fmt.Sprintf("逐项结果 · 实例 #%d", row.InstanceID), display.TextProps{Strong: true})
	ui.Column(c).Gap(10).Children(func() {
		for _, item := range report.Items {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					file := cmp.Or(item.File, "文件名未知")
					display.Text(c, file, display.TextProps{Strong: true, Rows: 1}).Ellipsis("…").Tooltip(file)
					version := cmp.Or(item.From, "未知") + " → " + cmp.Or(item.To, "未知")
					muted(c, version).FontSize(12).SingleLine().Ellipsis("…").Tooltip(version)
				})
				label, color := "未知", display.TagGray
				switch item.Status {
				case tasks.Completed:
					label, color = "已更新", display.TagSuccess
				case tasks.Failed:
					label, color = tasks.Failed, display.TagError
				case tasks.Canceled, tasks.ModUpdateNotRun:
					label = item.Status
				}
				display.Tag(c, label, display.TagProps{Color: color, Round: true})
			})
		}
		if report.Omitted > 0 {
			muted(c, fmt.Sprintf("还有 %d 项未保存逐项结果。", report.Omitted)).FontSize(12)
		}
	})
}
