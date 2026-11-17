package main

import (
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/layout"
)

type serverFormMode int

const (
	serverAdd serverFormMode = iota
	serverEdit
	serverDirect
	serverRemove
)

type serverForm struct {
	mode               serverFormMode
	original           launcher.Server
	name, address, err string
	focus              bool
}

func (a *desktop) openServerForm(mode serverFormMode, original launcher.Server) {
	name := original.Name
	if mode == serverEdit && name == original.Address {
		name = ""
	}
	a.serverForm = &serverForm{mode: mode, original: original, name: name, address: original.Address, focus: true}
}

func (a *desktop) serverDialog(c *ui.Context) {
	p := a.serverForm
	if p == nil {
		return
	}
	title, confirm := "添加服务器", "添加到收藏"
	switch p.mode {
	case serverEdit:
		title, confirm = "编辑服务器", "保存更改"
	case serverDirect:
		title, confirm = "临时直连", "启动并连接"
	case serverRemove:
		title, confirm = "移除服务器", "移除收藏"
	}
	open := true
	feedback.Modal(c, &open, feedback.ModalProps{Title: title, Width: 480, NoFooter: true}, func() {
		ui.Column(c).Gap(10).Children(func() {
			submit := false
			disabled := a.busy
			if p.mode == serverRemove {
				display.Text(c, "从收藏中移除「"+p.original.Name+"」？", display.TextProps{Strong: true})
				muted(c, p.original.Address).Selectable()
			} else {
				name, address := p.name, p.address
				display.Text(c, "服务器地址", display.TextProps{Strong: true})
				input := field.Input(c, &p.address, field.InputProps{Placeholder: "域名或 IP，可带端口"}).Label("服务器地址").Disabled(a.busy)
				if p.focus {
					input.Focus()
					p.focus = false
				}
				submit = input.Submitted()
				if p.mode == serverDirect {
					_, selected := a.serverInstancePicker(c.Key("direct-instance"))
					disabled = disabled || !selected || a.running != nil
				} else {
					display.Text(c, "名称（可选）", display.TextProps{Strong: true})
					nameInput := field.Input(c, &p.name, field.InputProps{Placeholder: "留空时使用服务器地址"}).Label("服务器名称").Disabled(a.busy)
					submit = nameInput.Submitted() || submit
				}
				if address != p.address || name != p.name {
					p.err = ""
				}
				disabled = disabled || strings.TrimSpace(p.address) == ""
			}
			if p.err != "" {
				feedback.Alert(c, p.err, feedback.AlertProps{Type: feedback.Error})
			}
			layout.Divider(c, layout.DividerProps{})
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if button(c, "取消", action.Default, false) {
					open = false
				}
				if button(c, confirm, action.Primary, disabled) || submit && !disabled {
					a.submitServerForm(p)
				}
			})
		})
	})
	if !open && a.serverForm == p {
		a.serverForm = nil
	}
}

func (a *desktop) submitServerForm(p *serverForm) {
	if a.serverForm != p || a.busy {
		return
	}
	if p.mode == serverDirect {
		address, err := launcher.ServerAddress(p.address)
		if err != nil {
			p.err = err.Error()
			return
		}
		if i, ok := a.serverInstance(); ok && a.running == nil {
			a.serverForm = nil
			a.playOnServer(i, false, address)
		}
		return
	}
	next := a.state.Clone()
	var err error
	var selection string
	switch p.mode {
	case serverAdd:
		err = next.AddServer(p.name, p.address)
		if err == nil {
			selection = next.Servers[len(next.Servers)-1].Address
		}
	case serverEdit, serverRemove:
		index := slices.Index(next.Servers, p.original)
		if index < 0 {
			p.err = i18n.Error("server.favorite_changed").Error()
			return
		}
		if p.mode == serverEdit {
			err = next.EditServer(p.original, p.name, p.address)
			selection = next.Servers[index].Address
		} else {
			next.RemoveServer(p.original.Address)
			if len(next.Servers) > 0 {
				selection = next.Servers[min(index, len(next.Servers)-1)].Address
			}
		}
	}
	if err != nil {
		p.err = err.Error()
		return
	}
	a.save(next, 0, func(err error) {
		if err != nil {
			if a.serverForm == p {
				p.err = err.Error()
			} else {
				a.message("无法保存服务器收藏", err)
			}
			return
		}
		if p.original.Address != "" && p.original.Address != selection {
			delete(a.serverStatus, p.original.Address)
		}
		if a.serverForm == p || a.browsedServer == p.original.Address {
			a.browsedServer = selection
		}
		if a.serverForm == p {
			a.serverForm = nil
		}
	})
}
