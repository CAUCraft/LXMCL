package main

import (
	"fmt"
	"strconv"

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
	err     string
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
				status.err = err.Error()
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

func (a *desktop) network(c *ui.Context) {
	workspace(c, func() {
		controls := func() {
			if button(c, "临时直连…", action.Default, a.busy || a.running != nil) {
				a.openServerForm(serverDirect, launcher.Server{})
			}
			if button(c, "添加服务器…", action.Primary, a.busy) {
				a.openServerForm(serverAdd, launcher.Server{})
			}
		}
		if len(a.state.Servers) == 0 {
			heading(c, "联机")
		} else {
			heading(c, "联机", controls)
		}
		note := "离线账号仅能加入允许离线登录的服务器。"
		if a.state.StationKey != "" {
			note = "皮肤站账号：" + a.accountName() + "。请加入同一认证体系的服务器。"
		} else if a.state.MicrosoftProfile != "" {
			note = "Microsoft 账号：" + a.accountName() + "。"
		}
		muted(c, note).FontSize(12)
		if len(a.state.Servers) == 0 {
			ui.Column(c).Grow(1).Center().Children(func() {
				display.Empty(c, display.EmptyProps{Icon: lucide.Server(), Title: "暂无收藏的服务器", Action: func() { ui.Row(c).Gap(8).Children(controls) }})
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
			muted(c, "收藏服务器").Padding(3, 5)
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
						action.CopyButton(c, server.Address, action.CopyButtonProps{}).Label("复制服务器地址")
					})
					check := a.serverStatus[server.Address]
					if check != nil && check.pending {
						flightLoading(c, check.message)
					} else if check != nil && check.err != "" {
						feedback.Alert(c, "无法查询服务器状态", feedback.AlertProps{Type: feedback.Warning, Description: check.err})
					} else if check != nil {
						muted(c, check.message).FontSize(13)
					} else {
						muted(c, "尚未查询服务器状态").FontSize(12)
					}
					i, selected := a.serverInstancePicker(c)
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						if button(c, "启动并加入", action.Primary, !selected || a.busy || a.running != nil) {
							a.playOnServer(i, false, server.Address)
						}
						if button(c, "查询状态", action.Default, check != nil && check.pending) {
							a.checkServer(server)
						}
						ui.Spacer(c).Grow(1)
						if icon(c, lucide.Pencil(), "编辑服务器 "+server.Name, a.busy) {
							a.openServerForm(serverEdit, server)
						}
						if icon(c, lucide.Trash2(), "移除服务器 "+server.Name, a.busy) {
							a.openServerForm(serverRemove, server)
						}
					})
				})
			})
		})
	})
}

func (a *desktop) serverInstance() (launcher.Instance, bool) {
	if i, ok := a.state.Find(a.serverInstanceID); ok {
		return i, true
	}
	return a.state.Find(a.state.Selected)
}

func (a *desktop) serverInstancePicker(c *ui.Context) (launcher.Instance, bool) {
	if len(a.state.Instances) == 0 {
		if button(c, "先安装游戏实例…", action.Default, a.busy) {
			a.serverForm = nil
			a.create()
		}
		return launcher.Instance{}, false
	}
	i, _ := a.serverInstance()
	value := strconv.Itoa(i.ID)
	options := make([]field.SelectOption, 0, len(a.state.Instances))
	for _, entry := range a.state.Instances {
		options = append(options, field.SelectOption{Value: strconv.Itoa(entry.ID), Label: entry.Name + " · " + entry.Version + " · " + loaderName(entry)})
	}
	settingsRow(c, "加入时使用", func() {
		catalogPicker(c, &value, options, "选择联机实例", "", a.busy || a.running != nil).Grow(1).MinWidth(0)
	})
	if value != strconv.Itoa(i.ID) {
		a.serverInstanceID, _ = strconv.Atoi(value)
	}
	return a.serverInstance()
}
