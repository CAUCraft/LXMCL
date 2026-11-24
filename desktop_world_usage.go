package main

import (
	"context"
	"fmt"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/saves"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
)

type worldUsage struct {
	folder  string
	cancel  context.CancelFunc
	loading bool
	failed  bool
	stats   saves.WorldFileStats
}

func (p *worldBrowser) closeWorldUsage() {
	if p.usage != nil {
		p.usage.cancel()
		p.usage = nil
	}
}

func (a *desktop) loadWorldUsage(p *worldBrowser, i launcher.Instance, world saves.World) {
	p.closeWorldUsage()
	ctx, cancel := context.WithCancel(a.ctx)
	request := &worldUsage{folder: world.Folder(), cancel: cancel, loading: true}
	p.usage = request
	go func() {
		defer cancel()
		stats, err := world.BackupStats(ctx, i.GameDir)
		a.post(func() {
			if !a.form.open || a.form.worlds != p || a.form.target != i ||
				p.usage != request || p.copy != nil || p.packs != nil {
				return
			}
			selected, ok := p.selectedWorld()
			current, exists := a.state.Find(i.ID)
			if ok && selected.Folder() == request.folder && exists && current == i {
				request.loading = false
				request.failed = err != nil
				request.stats = stats
			}
		})
	}()
}

func (a *desktop) worldUsageDetails(c *ui.Context, p *worldBrowser, i launcher.Instance, world saves.World) {
	if p.usage != nil && p.usage.folder != world.Folder() {
		p.closeWorldUsage()
	}
	label, status, loading := "计算大小", "", false
	if usage := p.usage; usage != nil {
		label, loading = "重新计算", usage.loading
		switch {
		case loading:
			label = "计算中…"
		case usage.failed:
			status = "无法完整统计"
		default:
			status = fmt.Sprintf("可备份 · %d 个文件 · %s", usage.stats.FileCount, fileSizeLabel(int64(usage.stats.Bytes)))
		}
	}
	if status != "" {
		muted(c, status).FontSize(12).Shrink(0).SingleLine()
	}
	if button(c, label, action.Default, loading || a.ctx == nil) {
		a.loadWorldUsage(p, i, world)
	}
}
