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
}

func (a *desktop) checkServer(server launcher.Server) {
	if a.serverStatus == nil {
		a.serverStatus = make(map[string]*serverCheck)
	}
	status := &serverCheck{pending: true, message: "正在查询…"}
	a.serverStatus[server.Address] = status
	go func() {
		reply, err := launcher.PingServer(a.ctx, server.Address)
		a.post(func() {
			status.pending = false
			if err != nil {
				status.message = "无法查询状态"
			} else {
				status.message = fmt.Sprintf("%s · %d/%d 人 · %d ms", reply.Version, reply.Online, reply.Max, reply.Latency.Milliseconds())
			}
		})
	}()
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
		if a.state.MicrosoftProfile != "" {
			note = "Microsoft 账号：" + a.accountName() + "。"
		}
		if selected {
			note += " 启动实例：" + i.Name + " · " + i.Version
		} else {
			note += " 请先选择启动实例。"
		}
		muted(c, note).FontSize(12)
		if len(a.state.Servers) == 0 {
			emptyPanel(c, lucide.Server(), "暂无收藏")
			return
		}
		layout.ScrollArea(c, layout.ScrollAreaProps{}, func() {
			surface(c).Padding(5, 12).Gap(4).Children(func() {
				for idx, server := range a.state.Servers {
					if idx > 0 {
						layout.Divider(c, layout.DividerProps{})
					}
					ui.Row(c.Key(server.Address)).MinHeight(49).Gap(8).AlignItems(ui.Center).Children(func() {
						leadIcon(c, lucide.Server(), 28)
						check := a.serverStatus[server.Address]
						ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
							display.Text(c, server.Name, display.TextProps{Strong: true, Rows: 1}).Ellipsis("…")
							detail := server.Address
							if check != nil {
								detail += " · " + check.message
							}
							muted(c, detail).FontSize(12).SingleLine().Ellipsis("…")
						})
						if button(c, "查询", action.Default, check != nil && check.pending) {
							a.checkServer(server)
						}
						if button(c, "启动并加入", action.Primary, !selected || a.busy || a.running != nil) {
							a.playOnServer(i, false, server.Address)
						}
						if icon(c, lucide.X(), "移除服务器 "+server.Name, a.busy) {
							a.removeServer(server)
						}
					})
				}
			})
		}).Grow(1).MinHeight(0)
	})
}
