package main

import (
	"fmt"

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
	icon    ui.ImageSource
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
				status.message = "无法查询状态"
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
	if err := next.AddServer(a.serverName, a.serverAddress); err != nil {
		a.serverError = err.Error()
		return
	}
	a.save(next, 0, func(err error) {
		if err != nil {
			a.serverError = err.Error()
			return
		}
		a.serverName, a.serverAddress, a.serverError = "", "", ""
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
		heading(c, "收藏服务器")
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			settingsInput(c, "服务器名称", &a.serverName, "名称（可选）", 180, a.busy)
			submitted := false
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				submitted = field.Input(c, &a.serverAddress, field.InputProps{Placeholder: "域名或 IP[:端口]"}).
					Label("服务器地址").Disabled(a.busy).Submitted()
			})
			if button(c, "添加服务器", action.Primary, a.busy || a.serverAddress == "") || submitted && !a.busy {
				a.addServer()
			}
		})
		if a.serverError != "" {
			feedback.Alert(c, a.serverError, feedback.AlertProps{Type: feedback.Error})
		}
		i, selected := a.state.Find(a.state.Selected)
		note := "离线账号仅能加入允许离线登录的服务器。"
		if a.state.StationKey != "" {
			note = "皮肤站账号：" + a.accountName() + "。请加入同一认证体系的服务器。"
		} else if a.state.MicrosoftProfile != "" {
			note = "Microsoft 账号：" + a.accountName() + "。"
		}
		if selected {
			note += " 启动实例：" + i.Name + " · " + i.Version
		} else {
			note += " 请先选择启动实例。"
		}
		muted(c, note).FontSize(12)
		if len(a.state.Servers) == 0 {
			w, _ := c.Size()
			splitWorkspace(c, listWidth(w), func() {
				muted(c, "服务器列表").Padding(3, 5)
				muted(c, "暂无收藏").Padding(3, 5)
			}, func() {
				ui.Column(c).Grow(1).Center().Children(func() {
					display.Empty(c, display.EmptyProps{Icon: lucide.Server(), Title: "暂无收藏", Description: "添加服务器后可查看状态并直接加入。"})
				})
			})
			return
		}
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
			muted(c, "服务器列表").Padding(3, 5)
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
				sectionPane(c).Padding(14).Gap(10).Children(func() {
					ui.Row(c).Gap(9).AlignItems(ui.Center).Children(func() {
						a.serverBadge(c, server.Address, 36)
						ui.Column(c).Gap(3).Grow(1).MinWidth(0).Children(func() {
							display.Text(c, server.Name, display.TextProps{Strong: true, FontSize: 15, Rows: 1}).Ellipsis("…")
							muted(c, server.Address).SingleLine().Ellipsis("…")
						})
					})
					check := a.serverStatus[server.Address]
					if check != nil && check.pending {
						flightLoading(c, check.message)
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
						if icon(c, lucide.X(), "移除服务器 "+server.Name, a.busy) {
							a.removeServer(server)
						}
					})
				})
			})
		})
	})
}
