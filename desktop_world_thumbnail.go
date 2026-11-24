package main

import (
	"context"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/saves"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

type worldThumbnail struct {
	folder  string
	cancel  context.CancelFunc
	picture ui.ImageSource
}

func (p *worldBrowser) closeWorldThumbnail() {
	if p.thumbnail != nil {
		p.thumbnail.cancel()
		p.thumbnail = nil
	}
}

func (a *desktop) loadWorldThumbnail(p *worldBrowser, i launcher.Instance, world saves.World) {
	if a.ctx == nil || p.thumbnail != nil && p.thumbnail.folder == world.Folder() {
		return
	}
	p.closeWorldThumbnail()
	ctx, cancel := context.WithCancel(a.ctx)
	request := &worldThumbnail{folder: world.Folder(), cancel: cancel}
	p.thumbnail = request
	go func() {
		defer cancel()
		img, err := world.Icon(ctx, i.GameDir)
		if err != nil {
			return
		}
		picture := ui.NewBitmap(img)
		a.post(func() {
			if !a.form.open || a.form.worlds != p || a.form.target != i || p.thumbnail != request {
				return
			}
			selected, ok := p.selectedWorld()
			current, exists := a.state.Find(i.ID)
			if ok && selected.Folder() == request.folder && exists && current == i {
				request.picture = picture
			}
		})
	}()
}

func (a *desktop) worldSelectionDetails(c *ui.Context, p *worldBrowser, i launcher.Instance, world saves.World) {
	a.loadWorldThumbnail(p, i, world)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if p.thumbnail != nil && p.thumbnail.picture != nil {
			ui.Image(c, p.thumbnail.picture).Size(40, 40).Fit(ui.Contain).Shrink(0).Label("世界缩略图")
		} else {
			ui.Box(c).Size(40, 40).Center().Shrink(0).Children(func() {
				ui.Icon(c, lucide.Folder()).FontSize(20).Label("世界文件夹图标")
			})
		}
		muted(c, worldSavedVersionLabel(world)).FontSize(12).SingleLine().Ellipsis("…")
	})
}
