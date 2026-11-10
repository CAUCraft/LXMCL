package main

import (
	"context"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/overlay"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

type microsoftLoginView struct {
	open, saving bool
	code         launcher.MicrosoftDeviceCode
	err          string
	cancel       context.CancelFunc
}

func (a *desktop) accountName() string {
	if a.state.StationKey != "" {
		if p, ok := a.state.Station(a.state.StationKey); ok {
			return p.Name
		}
		return "皮肤站账号"
	}
	if id := a.state.MicrosoftProfile; id != "" {
		if a.microsoftAccount.Profile.ID == id {
			return a.microsoftAccount.Profile.Name
		}
		return "Microsoft"
	}
	if a.state.Settings.Player != "" {
		return a.state.Settings.Player
	}
	return "离线账号"
}

func (a *desktop) openAccounts(microsoft bool) {
	if a.page != pageAccount {
		a.accountReturn = a.page
	}
	a.page = pageAccount
	a.browsedAccount = a.state.Settings.Player
	if microsoft {
		a.browsedAccount = ""
	} else if _, ok := a.state.Station(a.state.StationKey); ok {
		a.browsedAccount = stationAccountPrefix + a.state.StationKey
	} else if a.state.MicrosoftProfile != "" {
		a.browsedAccount = ""
	}
}

func (a *desktop) accounts(c *ui.Context) {
	workspace(c, func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Shrink(0).Children(func() {
			if icon(c, lucide.ArrowLeft(), "返回上一页", false) {
				a.page = a.accountReturn
			}
			display.Text(c, "账号", display.TextProps{Strong: true, FontSize: 19})
		})
		layout.Divider(c, layout.DividerProps{})
		w, _ := c.Size()
		splitWorkspace(c, min(214, max(158, w*0.19)), func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				muted(c, "账号列表").FontSize(12).Grow(1)
				a.addAccountMenu(c)
			})
			a.accountList(c)
		}, func() {
			key := a.browsedAccount
			if key == "" {
				key = ":microsoft/" + a.microsoftAccount.Profile.ID
			}
			detailPane(c.Key(key), func() {
				disabled := a.busy || a.pickerBusy
				if a.browsedAccount == "" {
					a.startAppearance()
					a.microsoftPage(c, disabled || a.busy)
				} else if key, station := strings.CutPrefix(a.browsedAccount, stationAccountPrefix); station {
					if p, ok := a.state.Station(key); ok {
						a.stationPage(c, p, disabled)
					}
				} else {
					a.offlineAccount(c, disabled)
				}
			})
		})
	})
}

func (a *desktop) microsoftPage(c *ui.Context, disabled bool) {
	profile := a.microsoftAccount.Profile
	selected := a.state.MicrosoftProfile
	syncing := a.appearance != nil && a.appearance.syncing
	if profile.ID == "" {
		sectionPane(c).Grow(1).Basis(0).MinHeight(0).Center().Padding(18).Children(func() {
			display.Empty(c, display.EmptyProps{Icon: lucide.ShieldCheck(), Title: "连接 Microsoft 账号",
				Description: "登录 Minecraft Java 版账号，管理皮肤和披风。",
				Action: func() {
					if button(c, "登录 Microsoft", action.Primary, disabled) {
						a.beginMicrosoftLogin()
					}
					if selected != "" && button(c, "移除过期登录", action.Text, disabled) {
						a.forgetMicrosoft()
					}
				}})
		})
		return
	}
	accountHeader(c, func() { a.accountAvatar(c, profile.Name, true, 42) },
		profile.Name, "Microsoft · Minecraft: Java 版", func() {
			if selected == profile.ID {
				activeAccountTag(c)
			} else if button(c, "设为启动账号", action.Primary, disabled) {
				a.selectMicrosoft(profile.ID)
			}
			a.microsoftMenu(c, disabled || syncing)
		})
	if p := a.appearance; p != nil && p.id == profile.ID {
		a.appearancePanel(c, p, disabled)
	}
}

func activeAccountTag(c *ui.Context) {
	display.Tag(c, "启动账号", display.TagProps{Color: display.TagBlue, Size: theme.Small})
}

func accountHeader(c *ui.Context, avatar func(), name, detail string, controls func()) {
	sectionPane(c).Padding(4, 0).Shrink(0).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			avatar()
			ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
				display.Text(c, name, display.TextProps{Strong: true, FontSize: 18, Rows: 1}).Ellipsis("…")
				muted(c, detail).FontSize(12).SingleLine().Ellipsis("…")
			})
			controls()
		})
	})
	layout.Divider(c, layout.DividerProps{})
}

func (a *desktop) microsoftMenu(c *ui.Context, disabled bool) {
	box := ui.Row(c).Shrink(0)
	open := ui.Local(box, "open", func() bool { return false })
	box.Children(func() {
		items := []overlay.MenuItem{
			{Key: "login", Label: "重新登录", Icon: lucide.RefreshCw(), Disabled: disabled},
			{Key: "logout", Label: "退出登录", Icon: lucide.LogOut(), Danger: true, Disabled: disabled},
		}
		switch overlay.DropdownMenu(c, open, overlay.DropdownMenuProps{Items: items}, func(trigger ui.Element) {
			action.StyleActionIcon(c, trigger, lucide.Ellipsis(), action.ActionIconProps{Title: "Microsoft 账号操作"})
		}) {
		case "login":
			a.beginMicrosoftLogin()
		case "logout":
			a.forgetMicrosoft()
		}
	})
}

const addOfflineAccount = ":add"

func (a *desktop) addAccountMenu(c *ui.Context) {
	root := ui.Row(c).Shrink(0)
	open := ui.Local(root, "open", func() bool { return false })
	root.Children(func() {
		switch overlay.DropdownMenu(c, open, overlay.DropdownMenuProps{Items: []overlay.MenuItem{
			{Key: "microsoft", Label: "Microsoft 登录", Icon: lucide.ShieldCheck()},
			{Key: "station", Label: "第三方皮肤站", Icon: lucide.Globe(), Disabled: a.busy},
			{Key: "offline", Label: "添加离线账号", Icon: lucide.UserRound(), Disabled: a.busy},
		}}, func(t ui.Element) {
			action.StyleActionIcon(c, t, lucide.Plus(), action.ActionIconProps{Title: "添加账号"})
		}) {
		case "microsoft":
			a.browsedAccount = ""
			if a.microsoftAccount.Profile.ID == "" {
				a.beginMicrosoftLogin()
			}
		case "station":
			a.beginStationLogin(launcher.StationProfile{})
		case "offline":
			a.browsedAccount = addOfflineAccount
		}
	})
}

func (a *desktop) accountList(c *ui.Context) {
	profile := a.microsoftAccount.Profile
	name, description := "Microsoft 账号", "Java 版 · 未登录"
	if profile.ID != "" {
		name, description = profile.Name, "Microsoft"
		if a.state.MicrosoftProfile == profile.ID {
			description = "当前启动账号"
		}
	}
	microsoft := display.ListItem{Key: ":microsoft", Label: name, Description: description, Icon: lucide.UserRound()}
	if profile.ID != "" {
		microsoft.Avatar = func() { a.accountAvatar(c, name, true, 28) }
	}
	items := []display.ListItem{microsoft}
	for _, p := range a.state.StationProfiles {
		detail := stationHost(p)
		if a.state.StationKey == p.Key {
			detail += " · 启动"
		}
		items = append(items, display.ListItem{Key: stationAccountPrefix + p.Key, Label: p.Name, Description: detail,
			Avatar: func() { a.stationAvatar(c, p, 28) }})
	}
	for _, player := range a.state.Players() {
		description := "离线"
		if a.state.MicrosoftProfile == "" && a.state.StationKey == "" && a.state.Settings.Player == player {
			description = "当前启动账号"
		}
		items = append(items, display.ListItem{Key: player, Label: player, Description: description,
			Avatar: func() { a.accountAvatar(c, player, false, 28) }})
	}
	active := a.browsedAccount
	if active == "" {
		active = ":microsoft"
	}
	layout.ScrollArea(c, layout.ScrollAreaProps{}, func() {
		if clicked := display.List(c, display.ListProps{Items: items, Active: active, Compact: true}); clicked != "" {
			if clicked == ":microsoft" {
				clicked = ""
			}
			a.browsedAccount = clicked
			c.Invalidate()
		}
	}).Grow(1).MinHeight(0)
}

func (a *desktop) offlineAccount(c *ui.Context, disabled bool) {
	name := a.browsedAccount
	if name == addOfflineAccount {
		sectionPane(c).Padding(8, 0).Gap(10).Children(func() {
			heading(c, "添加离线账号")
			settingsRow(c, "离线玩家名", func() {
				settingsInput(c, "离线玩家名", &a.newOfflinePlayer, "3–16 位字母、数字或下划线", 0, disabled)
				if button(c, "添加并切换", action.Primary, disabled || a.newOfflinePlayer == "") {
					a.selectOfflinePlayer(a.newOfflinePlayer)
				}
			})
			muted(c, "离线账号仅能加入允许离线登录的服务器。").FontSize(13)
		})
		return
	}
	accountHeader(c, func() { a.accountAvatar(c, name, false, 42) },
		name, "离线账号", func() {
			if name == a.state.Settings.Player && a.state.MicrosoftProfile == "" && a.state.StationKey == "" {
				activeAccountTag(c)
			} else if button(c, "设为启动账号", action.Primary, disabled) {
				a.selectOfflinePlayer(name)
			}
			if name != a.state.Settings.Player && icon(c, lucide.Trash2(), "移除离线账号", disabled) {
				a.forgetOfflinePlayer(name)
			}
		})
	root := ui.Row(c).Gap(10).AlignItems(ui.Stretch).Grow(1).Basis(0).MinHeight(0)
	orbit := ui.Local(root, "orbit", frontOrbit)
	root.Children(func() {
		modelPreview(c, clubSkin, nil, false, orbit, "孪枭 · 离线预览")
	})
}

func (a *desktop) selectMicrosoft(id string) {
	if id == "" || id != a.microsoftAccount.Profile.ID {
		return
	}
	next := a.state.Clone()
	next.MicrosoftProfile = id
	next.StationKey = ""
	a.save(next, 0, func(err error) { a.message("切换 Microsoft 账号", err) })
}

func (a *desktop) forgetMicrosoft() {
	if a.busy || (a.appearance != nil && a.appearance.syncing) {
		return
	}
	next := a.state.Clone()
	next.MicrosoftProfile = ""
	a.busy = true
	a.writes.Go(func() {
		err := a.store.DeleteMicrosoftAccount()
		removed := err == nil
		if removed {
			err = a.store.Save(next)
		}
		a.post(func() {
			a.busy = false
			if removed {
				a.microsoftAccount, a.minecraftSession = launcher.MicrosoftAccount{}, launcher.MinecraftSession{}
				a.appearance = nil
			}
			if err == nil {
				a.state = next
			}
			a.message("退出 Microsoft 登录", err)
		})
	})
}

func (a *desktop) beginMicrosoftLogin() {
	if a.busy || (a.appearance != nil && a.appearance.syncing) {
		return
	}
	if a.microsoft != nil && a.microsoft.cancel != nil {
		a.microsoft.cancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	p := &microsoftLoginView{open: true, cancel: cancel}
	a.microsoft = p
	a.writes.Go(func() {
		defer cancel()
		code, err := launcher.BeginMicrosoftLogin(ctx)
		if err != nil {
			a.loginError(p, err)
			return
		}
		a.post(func() {
			if a.microsoft == p && p.open {
				p.code = code
			}
		})
		account, session, err := launcher.CompleteMicrosoftLogin(ctx, code)
		if err != nil {
			a.loginError(p, err)
			return
		}
		a.post(func() {
			if a.microsoft == p && p.open {
				a.saveMicrosoftLogin(p, account, session)
			}
		})
	})
}

func (a *desktop) loginError(p *microsoftLoginView, err error) {
	a.post(func() {
		if a.microsoft == p && p.open {
			p.err, p.code = err.Error(), launcher.MicrosoftDeviceCode{}
		}
	})
}

func (a *desktop) saveMicrosoftLogin(p *microsoftLoginView, account launcher.MicrosoftAccount, session launcher.MinecraftSession) {
	next := a.state.Clone()
	next.MicrosoftProfile = account.Profile.ID
	next.StationKey = ""
	p.code, p.saving, a.busy = launcher.MicrosoftDeviceCode{}, true, true
	a.writes.Go(func() {
		err := a.store.SaveMicrosoftAccount(account)
		stored := err == nil
		if stored {
			err = a.store.Save(next)
		}
		a.post(func() {
			a.busy, p.saving = false, false
			if stored {
				a.microsoftAccount, a.minecraftSession = account, session
				a.appearance = nil
			}
			if err != nil {
				if p.open {
					p.err = "无法保存登录：" + err.Error()
				} else {
					a.message("无法保存 Microsoft 登录", err)
					a.noticeAccount = true
				}
				return
			}
			a.state, p.open = next, false
			a.message("已登录 Microsoft · "+account.Profile.Name, nil)
			a.startAppearance()
		})
	})
}

func (a *desktop) microsoftLoginDialog(c *ui.Context) {
	p := a.microsoft
	if p == nil {
		return
	}
	feedback.Modal(c, &p.open, feedback.ModalProps{Title: "登录 Microsoft", Width: 480, NoFooter: true}, func() {
		ui.Column(c).Gap(10).Children(func() {
			switch {
			case p.saving:
				flightLoading(c, "正在保存登录…")
			case p.err != "":
				feedback.Alert(c, p.err, feedback.AlertProps{Type: feedback.Error})
				if button(c, "重新获取登录代码", action.Default, a.busy) {
					a.beginMicrosoftLogin()
				}
			case p.code.Code == "":
				flightLoading(c, "正在向 Microsoft 获取登录代码…")
			default:
				panelHeading(c, lucide.ShieldCheck(), "在浏览器完成登录", "输入以下设备代码，回到启动器等待确认。")
				surface(c).Padding(10, 12).Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						display.Text(c, p.code.Code, display.TextProps{Code: true, Strong: true, FontSize: 19}).Grow(1).Selectable()
						action.CopyButton(c, p.code.Code, action.CopyButtonProps{}).Label("复制登录代码")
					})
				})
				layout.Divider(c, layout.DividerProps{})
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if button(c, "取消", action.Text, false) {
						p.open = false
					}
					if button(c, "打开 Microsoft 登录页", action.Primary, false) {
						c.OpenURL(p.code.VerificationURL)
					}
				})
			}
		})
	})
	if !p.open && !p.saving && a.microsoft == p {
		if p.cancel != nil {
			p.cancel()
		}
		a.microsoft = nil
	}
}
