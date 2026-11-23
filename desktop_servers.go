package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

type serverCheck struct {
	pending                   bool
	updated                   time.Time
	summary, description, err string
	icon                      ui.ImageSource
}

func (a *desktop) checkServer(server launcher.Server) {
	if a.serverStatus == nil {
		a.serverStatus = make(map[string]*serverCheck)
	}
	status := a.serverStatus[server.Address]
	if status == nil {
		status = &serverCheck{}
		a.serverStatus[server.Address] = status
	}
	if status.pending {
		return
	}
	status.pending, status.err = true, ""
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
				status.updated, status.description = time.Now(), reply.Description
				players := "未公开人数"
				if reply.PlayersReported {
					players = fmt.Sprintf("%d/%d 人", reply.Online, reply.Max)
				}
				status.summary = fmt.Sprintf("%s · %d ms", players, reply.Latency.Milliseconds())
				if reply.Version != "" {
					status.summary = reply.Version + " · " + status.summary
				}
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
		heading(c, "联机", func() {
			action.Segmented(c, &a.serverSource, []string{"启动器收藏", "游戏内收藏"}, action.SegmentedProps{Size: theme.Small}).Label("收藏来源")
			if a.serverSource == 0 && len(a.state.Servers) != 0 {
				controls()
			}
		})
		if a.serverSource == 1 {
			a.gameServerLibrary(c)
			return
		}
		a.closeGameServers()
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
		index := 0
		items := make([]display.ListItem, 0, len(a.state.Servers))
		for n, entry := range a.state.Servers {
			description := entry.Address
			if entry.Name == entry.Address {
				description = ""
			}
			items = append(items, display.ListItem{
				Key: entry.Address, Label: entry.Name, Description: description,
				Avatar: func() { a.serverBadge(c, entry.Address, 28) },
			})
			if entry.Address == a.browsedServer {
				server, index = entry, n
			}
		}
		w, _ := c.Size()
		splitWorkspace(c, listWidth(w), func() {
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				muted(c, "收藏服务器").Padding(3, 5).Grow(1)
				if len(a.state.Servers) > 1 {
					if icon(c, lucide.ArrowUp(), "上移收藏服务器", a.busy || index == 0) {
						a.moveServer(server, -1)
					}
					if icon(c, lucide.ArrowDown(), "下移收藏服务器", a.busy || index == len(a.state.Servers)-1) {
						a.moveServer(server, 1)
					}
				}
			})
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
			layout.ScrollArea(c.Key(server.Address), layout.ScrollAreaProps{}, func() {
				sectionPane(c).Padding(14).Gap(10).Children(func() {
					ui.Row(c).Gap(9).AlignItems(ui.Center).Children(func() {
						a.serverBadge(c, server.Address, 36)
						ui.Column(c).Gap(3).Grow(1).MinWidth(0).Children(func() {
							display.Text(c, server.Name, display.TextProps{Strong: true, FontSize: 15, Rows: 1}).Ellipsis("…")
							if server.Name != server.Address {
								muted(c, server.Address).SingleLine().Ellipsis("…")
							}
						})
						action.CopyButton(c, server.Address, action.CopyButtonProps{}).Label("复制服务器地址")
					})
					check := a.serverStatus[server.Address]
					serverStatusDetails(c, check)
					i, selected := a.serverInstancePicker(c, "加入时使用")
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
			}).Grow(1).MinHeight(0)
		})
	})
}

func (a *desktop) moveServer(server launcher.Server, step int) {
	next := a.state.Clone()
	if !next.MoveServer(server, step) {
		return
	}
	previous := a.browsedServer
	a.save(next, 0, func(err error) {
		if err != nil {
			a.message("保存服务器收藏顺序失败", err)
			return
		}
		if a.browsedServer == previous {
			a.browsedServer = server.Address
		}
	})
}

func (a *desktop) serverInstance() (launcher.Instance, bool) {
	if a.serverInstanceID != 0 {
		return a.state.Find(a.serverInstanceID)
	}
	return a.state.Find(a.state.Selected)
}

func (a *desktop) serverInstancePicker(c *ui.Context, label string) (launcher.Instance, bool) {
	if len(a.state.Instances) == 0 {
		if button(c, "先安装游戏实例…", action.Default, a.busy) {
			a.serverForm = nil
			a.create()
		}
		return launcher.Instance{}, false
	}
	value := strconv.Itoa(a.serverInstanceID)
	options := make([]field.SelectOption, 1, len(a.state.Instances)+1)
	options[0] = field.SelectOption{Value: "0", Label: "跟随主页", Disabled: true}
	for _, entry := range a.state.Instances {
		id := strconv.Itoa(entry.ID)
		label := entry.Name + " · " + entry.Version + " · " + loaderName(entry)
		options = append(options, field.SelectOption{Value: id, Label: label})
		if entry.ID == a.state.Selected {
			options[0] = field.SelectOption{Value: "0", Label: "跟随主页 · " + label}
		}
	}
	placeholder := ""
	if _, ok := a.serverInstance(); !ok && a.serverInstanceID != 0 {
		placeholder = "原实例已移除，请重新选择"
	}
	settingsRow(c, label, func() {
		catalogPicker(c, &value, options, "选择联机实例", placeholder, a.busy || a.running != nil).Grow(1).MinWidth(0)
	})
	if value != strconv.Itoa(a.serverInstanceID) {
		a.serverInstanceID, _ = strconv.Atoi(value)
	}
	return a.serverInstance()
}

func serverStatusDetails(c *ui.Context, status *serverCheck) {
	if status == nil {
		muted(c, "尚未查询服务器状态").FontSize(12)
		return
	}
	if status.pending {
		flightLoading(c, "正在查询服务器…")
	} else if status.err != "" {
		feedback.Alert(c, "无法查询服务器状态", feedback.AlertProps{Type: feedback.Warning, Description: status.err})
	}
	if status.updated.IsZero() {
		return
	}
	if status.description != "" {
		display.Text(c, status.description, display.TextProps{FontSize: 14, Rows: 3}).Ellipsis("…").Selectable()
	}
	display.Text(c, status.summary, display.TextProps{FontSize: 13, Rows: 1}).Ellipsis("…")
	muted(c, "上次成功查询 · "+status.updated.Local().Format("01-02 15:04:05")).FontSize(12)
}
