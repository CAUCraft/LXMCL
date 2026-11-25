package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
)

type instanceUsageResult struct {
	stats storage.FileUsage
	err   error
}

type instanceUsageRequest struct {
	target launcher.Instance
	cancel context.CancelFunc
	done   <-chan instanceUsageResult
	result instanceUsageResult
}

func (a *desktop) isUsageTarget(i launcher.Instance) bool {
	current, exists := a.state.Find(i.ID)
	return a.ready && a.loadErr == "" && a.page == pageLibrary && a.detailPage == detailOverview &&
		a.inspected == i.ID && exists && current == i
}

func (a *desktop) clearInstanceUsage() {
	if p := a.instanceUsage; p != nil && !a.isUsageTarget(p.target) {
		p.cancel()
		a.instanceUsage = nil
	}
}

func (a *desktop) startInstanceUsage(c *ui.Context, i launcher.Instance) {
	if a.ctx == nil || !a.isUsageTarget(i) {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	done := make(chan instanceUsageResult, 1)
	a.instanceUsage = &instanceUsageRequest{target: i, cancel: cancel, done: done}
	services, cache := c.Services(), a.game.Cache
	go func() {
		defer cancel()
		stats, err := storage.DirectoryFileUsage(ctx, i.GameDir, cache)
		done <- instanceUsageResult{stats, err}
		services.Invalidate()
	}()
}

func (a *desktop) instanceFileUsage(c *ui.Context, i launcher.Instance) {
	a.clearInstanceUsage()
	p := a.instanceUsage
	if p != nil {
		select {
		case result := <-p.done:
			p.result, p.done = result, nil
		default:
		}
	}
	label, status := "查看文件用量", ""
	if p != nil {
		label = "重新统计"
		switch {
		case p.done != nil:
			label, status = "取消统计", "正在统计普通文件…"
		case errors.Is(p.result.err, context.Canceled):
			status = "已取消统计"
		case p.result.err != nil:
			status = "无法完整统计，请检查目录和访问权限后重试。"
		default:
			status = fmt.Sprintf("%d 个文件 · %s（大小合计）", p.result.stats.Files, fileSizeLabel(p.result.stats.Bytes))
		}
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			property(c, "本地文件用量", status)
			if p != nil && p.done == nil && p.result.err == nil {
				muted(c, "可释放空间取决于文件系统。").FontSize(12)
			}
		})
		if button(c, label, action.Default, a.ctx == nil) {
			if p != nil && p.done != nil {
				p.cancel()
				p.done, p.result = nil, instanceUsageResult{err: context.Canceled}
			} else {
				a.startInstanceUsage(c, i)
			}
			c.Invalidate()
		}
	})
}
