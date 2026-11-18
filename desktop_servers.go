package main

import (
	"fmt"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

type serverCheck struct {
	pending bool
	message string
	err     error
	icon    ui.ImageSource
}

type favoriteForm struct {
	open, focus        bool
	address, name, err string
}

func (a *desktop) checkServer(server launcher.Server) {
	if a.serverStatus == nil {
		a.serverStatus = make(map[string]*serverCheck)
	}
	status := &serverCheck{pending: true, message: "正在查询…"}
	a.serverStatus[server.Address] = status
	go func() {
		reply, err := launcher.PingServer(a.ctx, server.Address)
		var picture ui.ImageSource
		if reply.Icon != nil {
			picture = ui.NewBitmap(reply.Icon)
		}
		a.post(func() {
			status.pending = false
			if err != nil {
				status.err = err
			} else {
				status.icon = picture
				status.message = fmt.Sprintf("%s · %d/%d 人 · %d ms", reply.Version, reply.Online, reply.Max, reply.Latency.Milliseconds())
			}
		})
	}()
}

func (a *desktop) serverBadge(c *ui.Context, address string, size float32) {
	if status := a.serverStatus[address]; status != nil && status.icon != nil {
		display.Avatar(c.Key(address), "", display.AvatarProps{Size: size, Image: status.icon, Shape: display.AvatarSquare})
	} else {
		leadIcon(c.Key(address), lucide.Server(), size)
	}
}

func (a *desktop) addServer() {
	next := a.state.Clone()
	if err := next.AddServer(a.favorite.name, a.favorite.address); err != nil {
		a.favorite.err = err.Error()
		return
	}
	added := next.Servers[len(next.Servers)-1]
	a.save(next, 0, func(err error) {
		if err != nil {
			if a.favorite.open {
				a.favorite.err = err.Error()
			} else {
				a.message("无法保存收藏", err)
			}
			return
		}
		a.favorite.open, a.browsedServer = false, added.Address
	})
}

func (a *desktop) favoriteDialog(c *ui.Context) {
	f := &a.favorite
	feedback.Modal(c, &f.open, feedback.ModalProps{Title: "收藏服务器", Width: 420, NoFooter: true}, func() {
		ui.Column(c).Gap(8).Children(func() {
			display.Text(c, "服务器地址", display.TextProps{Strong: true})
			address := field.Input(c, &f.address, field.InputProps{Placeholder: "域名或 IP，可附端口"}).Label("收藏地址").Disabled(a.busy)
			display.Text(c, "名称（可选）", display.TextProps{Strong: true})
			name := field.Input(c, &f.name, field.InputProps{Placeholder: "默认使用服务器地址"}).Label("收藏名称").Disabled(a.busy)
			if f.focus {
				if f.address == "" {
					address.Focus()
				} else {
					name.Focus()
				}
				f.focus = false
			}
			if address.Changed() || name.Changed() {
				f.err = ""
			}
			if f.err != "" {
				feedback.Alert(c, f.err, feedback.AlertProps{Type: feedback.Error})
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if button(c, "取消", action.Default, false) {
					f.open = false
				}
				canSave := f.open && !a.busy && strings.TrimSpace(f.address) != ""
				if (button(c, "保存收藏", action.Primary, !canSave) || address.Submitted() || name.Submitted()) && canSave {
					a.addServer()
				}
			})
		})
	})
}

func (a *desktop) removeServer(server launcher.Server) {
	next := a.state.Clone()
	next.RemoveServer(server.Address)
	a.save(next, 0, func(err error) {
		if err != nil {
			a.message("无法移除服务器", err)
		} else {
			delete(a.serverStatus, server.Address)
		}
	})
}

func (a *desktop) network(c *ui.Context) {
	workspace(c, func() {
		i, selected := a.state.Find(a.state.Selected)
		heading(c, "连接服务器")
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			submitted := false
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				input := field.Input(c, &a.serverAddress, field.InputProps{Placeholder: "输入服务器地址，可附端口", Invalid: a.serverError != ""}).Label("服务器地址")
				if input.Changed() {
					a.serverError = ""
				}
				submitted = input.Submitted()
			})
			canJoin := !a.busy && a.running == nil && selected && strings.TrimSpace(a.serverAddress) != ""
			if (button(c, "直接连接", action.Primary, !canJoin) || submitted) && canJoin {
				address, err := launcher.ServerAddress(a.serverAddress)
				if err != nil {
					a.serverError = err.Error()
				} else {
					a.playOnServer(i, false, address)
				}
			}
		})
		if a.serverError != "" {
			feedback.Alert(c, a.serverError, feedback.AlertProps{Type: feedback.Error})
		}
		note := a.accountName() + "（离线） · 适用于允许离线登录的服务器"
		if a.state.StationKey != "" {
			note = a.accountName() + "（皮肤站） · 适用于同一认证服务的服务器"
		} else if a.state.MicrosoftProfile != "" {
			note = a.accountName() + "（Microsoft）"
		}
		if selected {
			muted(c, "启动实例  "+i.Name+" · "+i.Version+"   |   "+note).FontSize(12)
		} else {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				muted(c, "连接服务器需要启动实例。")
				if len(a.state.Instances) == 0 {
					if button(c, "安装实例…", action.Default, false) {
						a.create()
					}
				} else if button(c, "选择启动实例", action.Default, false) {
					a.page = pageHome
				}
			})
		}
		layout.Divider(c, layout.DividerProps{})
		addFavorite := func() {
			if button(c, "添加收藏…", action.Default, a.busy) {
				a.favorite = favoriteForm{open: true, focus: true, address: strings.TrimSpace(a.serverAddress)}
			}
		}
		if len(a.state.Servers) == 0 {
			heading(c, "服务器收藏")
			ui.Column(c).Grow(1).Center().Children(func() {
				display.Empty(c, display.EmptyProps{Icon: lucide.Server(), Title: "暂无收藏", Description: "收藏后可查看在线人数和延迟。", Action: addFavorite})
			})
			return
		}
		heading(c, "服务器收藏", addFavorite)
		server := a.state.Servers[0]
		items := make([]display.ListItem, 0, len(a.state.Servers))
		for _, entry := range a.state.Servers {
			items = append(items, display.ListItem{
				Key: entry.Address, Label: entry.Name, Description: entry.Address,
				Avatar: func() { a.serverBadge(c, entry.Address, 28) },
			})
			if entry.Address == a.browsedServer {
				server = entry
			}
		}
		w, _ := c.Size()
		splitWorkspace(c, listWidth(w), func() {
			layout.ScrollArea(c, layout.ScrollAreaProps{}, func() {
				if key := display.List(c, display.ListProps{Items: items, Active: server.Address, Compact: true}); key != "" {
					a.browsedServer = key
					c.Invalidate()
					for _, entry := range a.state.Servers {
						if entry.Address == key {
							server = entry
							break
						}
					}
				}
			}).Grow(1).MinHeight(0)
		}, func() {
			detailPane(c.Key(server.Address), func() {
				surface(c).Padding(14).Gap(10).Children(func() {
					ui.Row(c).Gap(9).AlignItems(ui.Center).Children(func() {
						a.serverBadge(c, server.Address, 36)
						ui.Column(c).Gap(3).Grow(1).MinWidth(0).Children(func() {
							display.Text(c, server.Name, display.TextProps{Strong: true, FontSize: 15, Rows: 1}).Ellipsis("…")
							muted(c, server.Address).SingleLine().Ellipsis("…").Selectable()
						})
					})
					check := a.serverStatus[server.Address]
					if check != nil && check.pending {
						flightLoading(c, check.message)
					} else if check != nil && check.err != nil {
						feedback.Alert(c, "状态查询失败", feedback.AlertProps{Type: feedback.Error, Description: check.err.Error()})
					} else if check != nil {
						muted(c, check.message).FontSize(13)
					}
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						if button(c, "启动并加入", action.Primary, !selected || a.busy || a.running != nil) {
							a.playOnServer(i, false, server.Address)
						}
						if button(c, "查询状态", action.Default, check != nil && check.pending) {
							a.checkServer(server)
						}
						ui.Spacer(c).Grow(1)
						if icon(c, lucide.Trash2(), "移除服务器 "+server.Name, a.busy) {
							a.removeServer(server)
						}
					})
				})
			})
		})
	})
}
