package main

import (
	"context"
	"image"
	"net/url"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/overlay"
)

const stationAccountPrefix = ":station/"

type stationLoginView struct {
	open, busy, saving                     bool
	api, username, password, role, failure string
	target                                 launcher.StationProfile
	pending                                launcher.StationLogin
	cancel                                 context.CancelFunc
}

type stationPreview struct {
	busy bool
	skin skinPreview
	cape *image.NRGBA
	slim bool
	err  string
}

func stationHost(p launcher.StationProfile) string {
	if u, err := url.Parse(p.API); err == nil && u.Host != "" {
		return u.Host
	}
	return "皮肤站"
}

func (a *desktop) rememberStation(s launcher.StationSession) {
	if a.stationSessions == nil {
		a.stationSessions = make(map[string]launcher.StationSession)
	}
	a.stationSessions[s.Profile.Key] = s
}

func (a *desktop) beginStationLogin(target launcher.StationProfile) {
	if a.busy {
		return
	}
	a.stationLogin = &stationLoginView{open: true, api: target.API, target: target}
}

func (a *desktop) stationLoginDialog(c *ui.Context) {
	p := a.stationLogin
	if p == nil {
		return
	}
	feedback.Modal(c, &p.open, feedback.ModalProps{Title: "登录第三方皮肤站", Width: 440, NoFooter: true}, func() {
		ui.Column(c).Gap(8).Children(func() {
			roles := p.pending.Roles()
			if len(roles) == 0 {
				muted(c, "使用皮肤站自己的账号。登录信息会发给下方填写的站点。").FontSize(13)
				stationField(c, "皮肤站或 API 地址", &p.api, "https://skin.example.org", p.busy || p.target.Key != "", false)
				stationField(c, "皮肤站账号", &p.username, "邮箱或用户名", p.busy, false)
				stationField(c, "皮肤站密码", &p.password, "", p.busy, true)
			} else {
				options := make([]field.SelectOption, 0, len(roles))
				for _, r := range roles {
					options = append(options, field.SelectOption{Value: r.ID, Label: r.Name})
				}
				muted(c, "选择要加入启动器的角色。").FontSize(13)
				catalogPicker(c, &p.role, options, "皮肤站角色", "", p.busy)
			}
			if p.busy {
				flightLoading(c, "正在验证皮肤站登录…")
			}
			if p.failure != "" {
				feedback.Alert(c, p.failure, feedback.AlertProps{Type: feedback.Error})
			}
			ui.Row(c).Gap(6).Justify(ui.End).Children(func() {
				if button(c, "取消", action.Default, p.saving) {
					p.open = false
				}
				if len(roles) == 0 {
					if button(c, "登录皮肤站", action.Primary, a.busy || p.busy || p.api == "" || p.username == "" || p.password == "") {
						a.authenticateStation(p)
					}
				} else if button(c, "使用此角色", action.Primary, a.busy || p.busy || p.role == "") {
					a.bindStation(p)
				}
			})
		})
	})
	if !p.open && !p.saving {
		if p.cancel != nil {
			p.cancel()
		}
		p.password = ""
		a.stationLogin = nil
	}
}

func stationField(c *ui.Context, label string, value *string, placeholder string, disabled, password bool) {
	muted(c, label).FontSize(12)
	props := field.InputProps{Placeholder: placeholder}
	if password {
		field.InputPassword(c, value, props).Label(label).Disabled(disabled)
	} else {
		field.Input(c, value, props).Label(label).Disabled(disabled)
	}
}

func (a *desktop) authenticateStation(p *stationLoginView) {
	if a.ctx == nil || p.busy {
		return
	}
	api, user, password := p.api, p.username, p.password
	p.busy, p.password, p.failure = true, "", ""
	ctx, cancel := context.WithTimeout(a.ctx, 45*time.Second)
	p.cancel = cancel
	go func() {
		defer cancel()
		pending, err := launcher.BeginStationLogin(ctx, api, user, password)
		a.post(func() {
			if a.stationLogin != p || !p.open {
				return
			}
			p.busy = false
			if err != nil {
				p.failure = err.Error()
				return
			}
			p.pending = pending
			roles := pending.Roles()
			p.role = roles[0].ID
			if p.target.ID != "" {
				for _, role := range roles {
					if strings.EqualFold(role.ID, p.target.ID) {
						p.role = role.ID
						a.bindStation(p)
						return
					}
				}
				p.pending = launcher.StationLogin{}
				p.failure = "此登录没有原来的角色"
			} else if len(roles) == 1 {
				a.bindStation(p)
			}
		})
	}()
}

func (a *desktop) bindStation(p *stationLoginView) {
	if a.busy || p.busy || p.role == "" {
		return
	}
	pending, role, target, next := p.pending, p.role, p.target, a.state.Clone()
	p.busy, p.saving, p.failure, a.busy = true, true, "", true
	a.writes.Go(func() {
		ctx, stop := context.WithTimeout(a.ctx, 30*time.Second)
		defer stop()
		session, err := pending.Bind(ctx, role)
		if err == nil && target.Key != "" && session.Profile.Key != target.Key {
			err = launcher.ErrSkinStationLogin
		}
		if err == nil {
			err = a.store.SaveStationSession(session)
		}
		if err == nil {
			next.UseStation(session.Profile)
			err = a.store.Save(next)
		}
		a.post(func() {
			p.busy, p.saving, a.busy = false, false, false
			if err != nil {
				p.pending = launcher.StationLogin{}
				if p.open {
					p.failure = err.Error()
				} else {
					a.message("无法添加皮肤站账号", err)
				}
				return
			}
			a.rememberStation(session)
			a.state, p.open = next, false
			a.browsedAccount = stationAccountPrefix + session.Profile.Key
			a.loadStationPreview(session.Profile, true)
			a.message("已登录皮肤站 · "+session.Profile.Name, nil)
		})
	})
}

func (a *desktop) selectStation(p launcher.StationProfile) {
	next := a.state.Clone()
	next.UseStation(p)
	a.save(next, 0, func(err error) { a.message("切换皮肤站账号", err) })
}

func (a *desktop) forgetStation(p launcher.StationProfile) {
	if a.busy {
		return
	}
	next := a.state.Clone()
	next.ForgetStation(p.Key)
	a.busy = true
	a.writes.Go(func() {
		err := a.store.Save(next)
		removed := err == nil
		if removed {
			err = a.store.DeleteStationSession(p.Key)
		}
		a.post(func() {
			a.busy = false
			if removed {
				a.state = next
				delete(a.stationSessions, p.Key)
				delete(a.stationPreviews, p.Key)
				if a.browsedAccount == stationAccountPrefix+p.Key {
					a.browsedAccount = a.state.Settings.Player
				}
			}
			a.message("移除皮肤站账号", err)
		})
	})
}

func (a *desktop) loadStationPreview(profile launcher.StationProfile, force bool) {
	if a.ctx == nil || profile.Key == "" {
		return
	}
	if old := a.stationPreviews[profile.Key]; old != nil && (old.busy || !force) {
		return
	}
	if a.stationPreviews == nil {
		a.stationPreviews = make(map[string]*stationPreview)
	}
	p := &stationPreview{busy: true}
	a.stationPreviews[profile.Key] = p
	go func() {
		ctx, stop := context.WithTimeout(a.ctx, 20*time.Second)
		defer stop()
		t, err := profile.Textures(ctx)
		var skin skinPreview
		var cape *image.NRGBA
		if t.Skin != nil {
			skin = renderSkin(t.Skin)
		}
		if t.Cape != nil {
			cape = textureAtlas(t.Cape)
		}
		a.post(func() {
			p.busy, p.skin, p.cape, p.slim = false, skin, cape, t.Slim
			if err != nil {
				p.err = err.Error()
			}
		})
	}()
}

func (a *desktop) stationAvatar(c *ui.Context, profile launcher.StationProfile, size float32) {
	img := placeholderSkin.head
	if p := a.stationPreviews[profile.Key]; p != nil && p.skin.head != nil {
		img = p.skin.head
	}
	skinAvatar(c, profile.Name, img, size)
}

func (a *desktop) stationPage(c *ui.Context, profile launcher.StationProfile, disabled bool) {
	a.loadStationPreview(profile, false)
	accountHeader(c, func() { a.stationAvatar(c, profile, 38) }, profile.Name, stationHost(profile), func() {
		if a.state.StationKey == profile.Key {
			activeAccountTag(c)
		} else if button(c, "设为启动账号", action.Primary, disabled) {
			a.selectStation(profile)
		}
		box := ui.Row(c).Shrink(0)
		open := ui.Local(box, "open", func() bool { return false })
		box.Children(func() {
			switch overlay.DropdownMenu(c, open, overlay.DropdownMenuProps{Items: []overlay.MenuItem{
				{Key: "login", Label: "重新登录", Icon: lucide.RefreshCw(), Disabled: disabled},
				{Key: "remove", Label: "移除皮肤站账号", Icon: lucide.Trash2(), Danger: true, Disabled: disabled},
			}}, func(t ui.Element) {
				action.StyleActionIcon(c, t, lucide.Ellipsis(), action.ActionIconProps{Title: "皮肤站账号操作"})
			}) {
			case "login":
				a.beginStationLogin(profile)
			case "remove":
				a.forgetStation(profile)
			}
		})
	})
	preview := a.stationPreviews[profile.Key]
	skin, cape, slim, caption := placeholderSkin, (*image.NRGBA)(nil), false, "示意模型"
	if preview != nil {
		if preview.skin.atlas != nil {
			skin, slim, caption = preview.skin, preview.slim, ""
		}
		cape = preview.cape
	}
	body := ui.Row(c).Gap(10).AlignItems(ui.Stretch).Grow(1).Basis(0).MinHeight(0)
	orbit := ui.Local(body, "orbit", frontOrbit)
	body.Children(func() {
		modelPreview(c, skin, cape, slim, orbit, caption)
		accountInspector(c, func() {
			heading(c, "皮肤站")
			if preview != nil && preview.busy {
				flightLoading(c, "读取外观…")
			} else if preview != nil && preview.err != "" {
				feedback.Alert(c, preview.err, feedback.AlertProps{Type: feedback.Warning})
			}
			muted(c, "在皮肤站修改皮肤和披风，保存后回来刷新。").FontSize(13)
			if site := profile.Homepage(); site != "" && button(c, "打开皮肤站", action.Default, false) {
				c.OpenURL(site)
			}
			if button(c, "刷新外观", action.Text, preview != nil && preview.busy) {
				a.loadStationPreview(profile, true)
			}
		})
	})
}
