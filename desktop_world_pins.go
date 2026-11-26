package main

import (
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/saves"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

func (a *desktop) worldPinButton(c *ui.Context, p *worldBrowser, i launcher.Instance, world saves.World) {
	if p.retry != nil {
		return
	}
	pinned := a.state.PinnedWorlds[i.ID][world.Folder()]
	label, glyph := "置顶世界", lucide.Pin()
	if pinned {
		label, glyph = "取消置顶世界", lucide.PinOff()
	}
	if icon(c.Key("world-pin"), glyph, label, a.ctx == nil || a.busy || a.running != nil || p.pinning) {
		a.setWorldPinned(p, i, world, !pinned)
	}
}

func (a *desktop) setWorldPinned(p *worldBrowser, i launcher.Instance, world saves.World, pinned bool) {
	p.pinning, p.pinErr = true, ""
	go func() {
		err := world.Check(i.GameDir)
		a.post(func() {
			p.pinning = false
			if !a.form.open || a.form.worlds != p || a.form.target != i || a.busy || a.running != nil || p.loading {
				return
			}
			current, found := a.state.Find(i.ID)
			selected, ok := p.selectedWorld()
			if !found || current != i || !ok || selected.Folder() != world.Folder() {
				return
			}
			if err != nil {
				p.pinErr = "存档已变化，请刷新后重新选择。"
				p.list.ScrollIntoView(p.selected)
				return
			}
			next := a.state.Clone()
			if err := next.SetWorldPinned(i.ID, i.Revision, world.Folder(), pinned); err != nil {
				p.pinErr = err.Error()
				return
			}
			a.save(next, 0, func(err error) {
				active := a.form.open && a.form.worlds == p && a.form.target == i
				if err != nil {
					selected, _ := p.selectedWorld()
					if active && selected.Folder() == world.Folder() {
						p.pinErr = err.Error()
					} else {
						a.message("未能保存置顶状态", err)
					}
					return
				}
				if active {
					p.filter(a.state.PinnedWorlds[i.ID])
				}
			})
		})
	}()
}
