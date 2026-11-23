package main

import (
	"context"
	"errors"
	"io/fs"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/server"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

type gameServerView struct {
	instance         launcher.Instance
	cancel           context.CancelFunc
	loading, missing bool
	err              string
	rows             []server.Server
	list             ui.ListState
}

func (a *desktop) closeGameServers() {
	if p := a.gameServers; p != nil {
		if p.cancel != nil {
			p.cancel()
		}
		a.gameServers = nil
	}
}

func (a *desktop) loadGameServers(i launcher.Instance) {
	a.closeGameServers()
	p := &gameServerView{instance: i, loading: true}
	a.gameServers = p
	if a.ctx == nil {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	p.cancel = cancel
	go func() {
		rows, err := server.ReadGameList(ctx, i.GameDir)
		a.post(func() {
			if ctx.Err() != nil || a.gameServers != p {
				return
			}
			p.loading = false
			switch {
			case errors.Is(err, fs.ErrNotExist):
				p.missing = true
			case err != nil:
				var pathErr *fs.PathError
				if errors.As(err, &pathErr) {
					p.err = "请检查此实例的 servers.dat 是否可读取，然后刷新。"
				} else {
					p.err = err.Error()
				}
			default:
				p.rows = rows
			}
		})
	}()
}

func (a *desktop) gameServerLibrary(c *ui.Context) {
	i, ok := a.serverInstancePicker(c, "联机实例")
	if !ok {
		a.closeGameServers()
		return
	}
	if p := a.gameServers; p == nil || p.instance != i {
		a.loadGameServers(i)
	}
	p := a.gameServers
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		muted(c, i.Name+" · servers.dat").Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		if button(c, "刷新列表", action.Default, p.loading) {
			a.loadGameServers(i)
		}
	})
	p = a.gameServers
	switch {
	case p.loading:
		flightLoading(c, "正在读取游戏内收藏…")
	case p.err != "":
		feedback.Alert(c, "无法读取游戏内收藏", feedback.AlertProps{Type: feedback.Warning, Description: p.err})
	case p.missing:
		emptyPanel(c, lucide.Server(), "此实例还没有 servers.dat")
	case len(p.rows) == 0:
		emptyPanel(c, lucide.Server(), "此实例没有可见的游戏内收藏")
	default:
		p.list.Label = func(idx int) string { return p.rows[idx].Name + " " + p.rows[idx].Address }
		ui.List(c.Key(i.ID), &p.list, len(p.rows)).Grow(1).MinHeight(0).FillWidth().Rows(func(row ui.ListRow) {
			rc := row.Context
			s := p.rows[row.Index]
			name, address := s.Name, s.Address
			if name == "" {
				name = "未命名服务器"
			}
			if address == "" {
				address = "未填写地址"
			}
			ui.Row(rc).Height(52).Padding(0, 10).Gap(10).AlignItems(ui.Center).Children(func() {
				leadIcon(rc, lucide.Server(), 28)
				ui.Column(rc).Grow(1).MinWidth(0).Children(func() {
					display.Text(rc, name, display.TextProps{Strong: true, Rows: 1}).Ellipsis("…")
					muted(rc, address).SingleLine().Ellipsis("…")
				})
				action.CopyButton(rc, s.Address, action.CopyButtonProps{}).Label("复制游戏内地址 " + name).Disabled(s.Address == "")
			})
		})
	}
}
