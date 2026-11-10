package main

import (
	"context"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

type microsoftLoginView struct {
	open, saving bool
	code         launcher.MicrosoftDeviceCode
	err          string
	cancel       context.CancelFunc
}

func (a *desktop) accountName() string {
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

func (a *desktop) microsoftSettings(c *ui.Context, disabled bool) {
	profile := a.microsoftAccount.Profile
	selected := a.state.MicrosoftProfile
	syncing := a.appearance != nil && a.appearance.syncing
	surface(c).Padding(12, 14).Gap(8).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			name := profile.Name
			if name == "" {
				name = "Microsoft 账号"
			}
			a.accountAvatar(c, name, true, 40)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				display.Text(c, name, display.TextProps{Strong: true, FontSize: 17, Rows: 1}).Ellipsis("…")
				muted(c, "Minecraft: Java 版").FontSize(12)
			})
			if profile.ID != "" {
				if selected == profile.ID {
					display.Tag(c, "使用中", display.TagProps{Color: display.TagGreen, Size: theme.Small})
				} else if button(c, "切换", action.Default, disabled) {
					a.selectMicrosoft(profile.ID)
				}
			}
			label := "登录 Microsoft"
			if profile.ID != "" {
				label = "重新登录"
			}
			if button(c, label, action.Primary, disabled || syncing) {
				a.beginMicrosoftLogin()
			}
			if (profile.ID != "" || selected != "") && button(c, "退出登录", action.Text, disabled || syncing) {
				a.forgetMicrosoft()
			}
		})
		if selected != "" && selected != profile.ID {
			feedback.Alert(c, "需要重新登录 Microsoft，或切换到离线账号。", feedback.AlertProps{Type: feedback.Warning})
		}
		if !launcher.MicrosoftCredentialsPersist() {
			muted(c, "此平台的 Microsoft 登录仅在本次运行中保留。").FontSize(12)
		}
		if profile.ID == "" {
			muted(c, "登录后可预览、更换 Java 版皮肤与披风。").FontSize(13)
		}
	})
	if p := a.appearance; p != nil && p.id == profile.ID {
		a.appearancePanel(c, p, disabled)
	}
}

const addOfflineAccount = ":add"

func (a *desktop) accountList(c *ui.Context) {
	profile := a.microsoftAccount.Profile
	name, description := "Microsoft 账号", "Java 版 · 未登录"
	if profile.ID != "" {
		name, description = profile.Name, "Microsoft"
		if a.state.MicrosoftProfile == profile.ID {
			description += " · 使用中"
		}
	}
	items := []display.ListItem{{Key: ":microsoft", Label: name, Description: description,
		Avatar: func() { a.accountAvatar(c, name, true, 25) }}}
	for _, player := range a.state.Players() {
		description := "离线"
		if a.state.MicrosoftProfile == "" && a.state.Settings.Player == player {
			description += " · 使用中"
		}
		items = append(items, display.ListItem{Key: player, Label: player, Description: description,
			Avatar: func() { a.accountAvatar(c, player, false, 25) }})
	}
	items = append(items, display.ListItem{Key: addOfflineAccount, Label: "添加离线账号", Icon: lucide.Plus()})
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
		surface(c).Padding(12, 14).Gap(10).Children(func() {
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
	surface(c).Padding(12, 14).Gap(10).Children(func() {
		ui.Row(c).Gap(9).AlignItems(ui.Center).Children(func() {
			a.accountAvatar(c, name, false, 40)
			display.Text(c, name, display.TextProps{Strong: true, FontSize: 17}).Grow(1)
			if name == a.state.Settings.Player && a.state.MicrosoftProfile == "" {
				display.Tag(c, "使用中", display.TagProps{Color: display.TagGreen, Size: theme.Small})
			} else if button(c, "切换", action.Primary, disabled) {
				a.selectOfflinePlayer(name)
			}
			if name != a.state.Settings.Player && button(c, "移除离线账号", action.Text, disabled) {
				a.forgetOfflinePlayer(name)
			}
		})
		muted(c, "启动器使用孪枭作为离线账号头像。要在 Minecraft 中更换皮肤或披风，请登录 Microsoft。").FontSize(13)
		if a.microsoftAccount.Profile.ID == "" && button(c, "登录 Microsoft", action.Default, disabled) {
			a.browsedAccount = ""
			a.beginMicrosoftLogin()
		}
	})
	surface(c).Padding(12, 14).Gap(8).Children(func() {
		heading(c, "孪枭 · 皮肤预览")
		ui.Row(c).Gap(8).Children(func() {
			appearanceTile(c, "正面", clubSkin.front, lucide.UserRound())
			appearanceTile(c, "背面", clubSkin.back, lucide.UserRound())
		})
	})
}

func (a *desktop) selectMicrosoft(id string) {
	if id == "" || id != a.microsoftAccount.Profile.ID {
		return
	}
	next := a.state.Clone()
	next.MicrosoftProfile = id
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
