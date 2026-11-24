package main

import (
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
)

type installCacheSelection struct {
	version, loader, loaderVersion string
}

type installCachePreview struct {
	selection installCacheSelection
	status    launcher.CacheStatus
	loading   bool
	failed    bool
}

func (a *desktop) installCacheDetails(c *ui.Context, p *installView, loader string) {
	selection := installCacheSelection{version: p.version, loader: loader}
	valid := loader == launcher.LoaderVanilla || p.validLoader()
	if loader != launcher.LoaderVanilla {
		selection.loaderVersion = p.loaderVersion
	}
	if p.cache != nil && (!valid || p.cache.selection != selection) {
		p.cache = nil
	}
	preview := p.cache
	loading := preview != nil && preview.loading
	ui.Row(c).MinHeight(38).Gap(8).AlignItems(ui.Center).Children(func() {
		if button(c, "检查本地缓存", action.Default, !valid || loading || a.ctx == nil) {
			a.checkInstallCache(p, selection)
		}
		switch {
		case loading:
			muted(c, "检查中…").FontSize(13)
		case preview == nil:
		case preview.failed:
			muted(c, "无法检查本地缓存").FontSize(13)
		default:
			loaderStatus := "不适用"
			if loader != launcher.LoaderVanilla {
				loaderStatus = cachePresence(preview.status.Loader)
			}
			muted(c, "游戏文件缓存："+cachePresence(preview.status.Game)).FontSize(13)
			muted(c, "加载器缓存："+loaderStatus).FontSize(13)
		}
	})
	if preview != nil && !loading && !preview.failed {
		muted(c, "安装和启动仍会校验文件。").FontSize(12)
	}
}

func cachePresence(found bool) string {
	if found {
		return "已找到"
	}
	return "未找到"
}

func (a *desktop) checkInstallCache(p *installView, selection installCacheSelection) {
	request := &installCachePreview{selection: selection, loading: true}
	p.cache = request
	game := a.game
	go func() {
		status, err := game.CacheStatus(selection.version, selection.loader, selection.loaderVersion)
		a.post(func() {
			if p.cache == request {
				request.status, request.failed, request.loading = status, err != nil, false
			}
		})
	}()
}
