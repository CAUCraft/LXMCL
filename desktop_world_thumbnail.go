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

func worldModeLabel(mode saves.GameMode) string {
	switch mode {
	case saves.GameModeSurvival:
		return "生存"
	case saves.GameModeCreative:
		return "创造"
	case saves.GameModeAdventure:
		return "冒险"
	case saves.GameModeSpectator:
		return "旁观"
	default:
		return "未知"
	}
}

func worldDifficultyLabel(difficulty saves.Difficulty) string {
	switch difficulty {
	case saves.DifficultyPeaceful:
		return "和平"
	case saves.DifficultyEasy:
		return "简单"
	case saves.DifficultyNormal:
		return "普通"
	case saves.DifficultyHard:
		return "困难"
	default:
		return "未知"
	}
}

func (a *desktop) worldSelectionDetails(c *ui.Context, p *worldBrowser, i launcher.Instance, world saves.World) {
	a.loadWorldThumbnail(p, i, world)
	hardcore := "未知"
	if enabled, known := world.Hardcore(); known {
		hardcore = "关闭"
		if enabled {
			hardcore = "开启"
		}
	}
	settings := "默认游戏模式 · " + worldModeLabel(world.DefaultGameMode()) + "   硬核 · " + hardcore
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if p.thumbnail != nil && p.thumbnail.picture != nil {
			ui.Image(c, p.thumbnail.picture).Size(40, 40).Fit(ui.Contain).Shrink(0).Label("世界缩略图")
		} else {
			ui.Box(c).Size(40, 40).Center().Shrink(0).Children(func() {
				ui.Icon(c, lucide.Folder()).FontSize(20).Label("世界文件夹图标")
			})
		}
		ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
			muted(c, worldSavedVersionLabel(world)).FontSize(12).SingleLine().Ellipsis("…")
			muted(c, settings).FontSize(12).SingleLine().Ellipsis("…").Tooltip(settings)
			muted(c, "存档难度 · "+worldDifficultyLabel(world.Difficulty())).FontSize(12).SingleLine()
		})
		a.worldPinButton(c, p, i, world)
		a.worldUsageDetails(c, p, i, world)
	})
}
