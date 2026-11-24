package main

import (
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
)

type installSpacePaths struct {
	target, cache string
}

type installSpaceCheck struct {
	paths   installSpacePaths
	loading bool
	target  string
	cache   string
}

func (a *desktop) installSpacePreview(c *ui.Context, p *installView, directory string) {
	paths := installSpacePaths{target: directory, cache: a.game.Cache}
	if p.space != nil && p.space.paths != paths {
		p.space = nil
	}
	if directory == "" {
		return
	}
	check := p.space
	loading := check != nil && check.loading
	ui.Row(c).MinHeight(38).Gap(8).AlignItems(ui.Center).Children(func() {
		if button(c, "查看可用空间", action.Default, loading || a.ctx == nil) {
			a.checkInstallSpace(p, paths)
		}
		switch {
		case loading:
			muted(c, "正在查看可用空间…").FontSize(13)
		case check != nil:
			ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
				muted(c, "目标目录可用："+check.target).FontSize(13).Tooltip(paths.target)
				muted(c, "共享缓存可用："+check.cache).FontSize(13).Tooltip(paths.cache)
			})
		}
	})
}

func (a *desktop) checkInstallSpace(p *installView, paths installSpacePaths) {
	request := &installSpaceCheck{paths: paths, loading: true}
	p.space = request
	go func() {
		target, cache := availableSpaceLabel(paths.target), availableSpaceLabel(paths.cache)
		a.post(func() {
			if p.space == request {
				request.target, request.cache, request.loading = target, cache, false
			}
		})
	}()
}

func availableSpaceLabel(path string) string {
	bytes, err := launcher.AvailableDiskSpace(path)
	if err != nil {
		return "未知"
	}
	return fileSizeLabel(bytes)
}
