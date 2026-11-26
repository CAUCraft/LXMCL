package main

import (
	"strconv"

	"github.com/CAUCraft/LXMCL/internal/server"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
)

type gameServerImport struct {
	saved    map[string]bool
	selected map[int]bool
}

func (d *gameServerImport) keepVisible(visible []int) {
	if len(d.selected) == 0 {
		return
	}
	selected := make(map[int]bool, len(d.selected))
	for _, index := range visible {
		if d.selected[index] {
			selected[index] = true
		}
	}
	d.selected = selected
}

func (a *desktop) gameServerImportControls(c *ui.Context, p *gameServerView, visible []int) {
	if len(p.rows) == 0 {
		return
	}
	d := p.importing
	if d == nil {
		if button(c, "导入到启动器…", action.Default, a.busy || len(visible) == 0) {
			p.importing = &gameServerImport{saved: a.state.SavedServerAddresses(), selected: make(map[int]bool)}
		}
		return
	}
	if button(c, "取消", action.Default, a.busy) {
		p.importing = nil
		return
	}
	if button(c.Key(p.applied), "确认导入（"+strconv.Itoa(len(d.selected))+"）", action.Primary, a.busy || len(d.selected) == 0) {
		a.importGameServers(p, visible)
	}
}

func (d *gameServerImport) checkbox(c *ui.Context, row int, s server.Server, disabled bool) {
	entry, err := server.NewFavorite(s.Name, s.Address)
	if err != nil {
		muted(c, "无法导入").FontSize(12).Tooltip(err.Error())
		return
	}
	if d.saved[entry.Address] {
		muted(c, "已收藏").FontSize(12)
		return
	}
	picked := d.selected[row]
	if choice.Checkbox(c, &picked, "", choice.CheckboxProps{}).Label("选择导入 " + s.Name + " " + s.Address).Disabled(disabled).Changed() {
		if picked {
			d.selected[row] = true
		} else {
			delete(d.selected, row)
		}
	}
}

func (a *desktop) importGameServers(p *gameServerView, visible []int) {
	i, ok := a.serverInstance()
	if !ok || i != p.instance || a.gameServers != p || p.importing == nil || a.busy || !a.ready {
		return
	}
	var selected []server.Server
	for _, index := range visible {
		if p.importing.selected[index] {
			selected = append(selected, p.rows[index])
		}
	}
	if len(selected) == 0 {
		return
	}
	next := a.state.Clone()
	added := next.ImportServers(selected)
	if added == 0 {
		p.importing = nil
		a.message("所选服务器均已收藏", nil)
		return
	}
	a.save(next, 0, func(err error) {
		if err != nil {
			a.message("无法保存服务器收藏", err)
			return
		}
		p.importing = nil
		notice := "已导入 " + strconv.Itoa(added) + " 个服务器收藏"
		if skipped := len(selected) - added; skipped > 0 {
			notice += "，跳过 " + strconv.Itoa(skipped) + " 个同址条目"
		}
		a.message(notice, nil)
	})
}
