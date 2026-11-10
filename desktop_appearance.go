package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

type appearanceView struct {
	id                       string
	loaded, syncing, reading bool
	data                     launcher.Appearance
	skin                     skinPreview
	capes                    map[string]*ui.Bitmap
	file, variant            string
	draft                    []byte
	classic, slim            skinPreview
	legacy                   bool
	cape                     string
	capeLoading, capeError   string
	capeRequest              uint64
	status, failure, warning string
}

type appearanceChange struct {
	skin    []byte
	variant string
	setCape bool
	cape    string
}

func (a *desktop) accountAvatar(c *ui.Context, name string, online bool, size float32) {
	_, wash := brand(c)
	var img ui.ImageSource = clubSkin.head
	if online {
		if p := a.appearance; p != nil && p.id == a.microsoftAccount.Profile.ID && p.skin.head != nil {
			img = p.skin.head
		}
	}
	display.Avatar(c, name, display.AvatarProps{
		Size: size, Image: img, Shape: display.AvatarSquare, Fallback: display.AvatarText, Background: wash,
	})
}

func (a *desktop) startAppearance() {
	id := a.microsoftAccount.Profile.ID
	if a.ctx == nil || id == "" || a.busy || a.appearance != nil || (a.microsoft != nil && a.microsoft.open) {
		return
	}
	a.appearance = &appearanceView{id: id, capes: make(map[string]*ui.Bitmap)}
	a.syncAppearance(a.appearance, appearanceChange{})
}

func (a *desktop) syncAppearance(p *appearanceView, change appearanceChange) {
	if a.busy || p.syncing || a.appearance != p || a.microsoftAccount.Profile.ID != p.id {
		return
	}
	account, session := a.microsoftAccount, a.minecraftSession
	refresh := !session.ValidFor(p.id)
	p.syncing, a.busy = true, refresh
	p.status, p.failure, p.warning = "", "", ""
	a.writes.Go(func() {
		ctx, stop := context.WithTimeout(a.ctx, 65*time.Second)
		defer stop()
		var data launcher.Appearance
		var err error
		var saved, applied bool
		if refresh {
			var next launcher.MicrosoftAccount
			next, session, err = launcher.RefreshMicrosoftLogin(ctx, account)
			if err == nil {
				account, saved = next, true
				if saveErr := a.store.SaveMicrosoftAccount(next); saveErr != nil {
					err = fmt.Errorf("无法保存刷新后的登录：%w", saveErr)
				}
			}
		}
		if err == nil && len(change.skin) != 0 {
			err = session.UploadSkin(ctx, change.skin, change.variant)
			applied = err == nil
		}
		if err == nil && change.setCape {
			err = session.SetCape(ctx, change.cape)
			applied = err == nil
		}
		if err == nil {
			data, err = session.Appearance(ctx)
		}
		a.post(func() {
			if refresh {
				a.busy = false
			}
			if a.appearance != p {
				return
			}
			if saved {
				a.microsoftAccount, a.minecraftSession = account, session
			}
			if errors.Is(err, launcher.ErrMicrosoftRequired) && a.minecraftSession == session {
				a.minecraftSession = launcher.MinecraftSession{}
			}
			if applied {
				if len(change.skin) != 0 {
					p.draft, p.file = nil, ""
					p.skin = p.classic
					if change.variant == "slim" {
						p.skin = p.slim
					}
				}
				p.status = "外观已应用到 Minecraft"
			}
			if err != nil {
				p.syncing = false
				p.failure = err.Error()
				if applied {
					p.loaded = false
					p.failure = "更改已提交，但无法刷新档案：" + p.failure
				}
				return
			}
			if data.Skin.URL != p.data.Skin.URL && !(applied && len(change.skin) != 0) {
				p.skin = skinPreview{}
			}
			cape := data.ActiveCape().ID
			if !change.setCape && p.cape != p.data.ActiveCape().ID {
				if p.cape == "" {
					cape = ""
				}
				for _, entry := range data.Capes {
					if entry.ID == p.cape {
						cape = entry.ID
						break
					}
				}
			}
			p.loaded, p.data, p.cape = true, data, cape
			p.capes = make(map[string]*ui.Bitmap)
			p.capeRequest++
			p.capeLoading, p.capeError = "", ""
			if p.draft == nil {
				p.variant = strings.ToLower(data.Skin.Variant)
				if p.variant != "slim" {
					p.variant = "classic"
				}
			}
			a.loadProfilePreviews(p, data)
		})
	})
}

func (a *desktop) loadProfilePreviews(p *appearanceView, data launcher.Appearance) {
	go func() {
		ctx, stop := context.WithTimeout(a.ctx, 16*time.Second)
		defer stop()
		var skin skinPreview
		var cape *ui.Bitmap
		var warnings []string
		if data.Skin.URL != "" {
			img, err := launcher.FetchProfileTexture(ctx, data.Skin.URL, true)
			if err == nil {
				skin = renderSkin(img, strings.EqualFold(data.Skin.Variant, "slim"))
			} else {
				warnings = append(warnings, "皮肤预览加载失败")
			}
		}
		active := data.ActiveCape()
		if active.URL != "" {
			img, err := launcher.FetchProfileTexture(ctx, active.URL, false)
			if err == nil {
				cape = renderCape(img)
			} else {
				warnings = append(warnings, "披风预览加载失败")
			}
		}
		a.post(func() {
			if a.appearance != p {
				return
			}
			p.syncing, p.warning = false, strings.Join(warnings, " · ")
			if skin.head != nil || data.Skin.URL == "" {
				p.skin = skin
			}
			if cape != nil {
				p.capes[active.ID] = cape
			}
			if p.cape != active.ID {
				a.previewCape(p)
			}
		})
	}()
}

func (a *desktop) chooseSkin(p *appearanceView) {
	a.pickPath(mygo.OpenDialogOptions{
		Title: "选择 Minecraft 皮肤", ButtonLabel: "预览皮肤",
		Filters: []mygo.FileFilter{{Name: "PNG 皮肤", Extensions: []string{"png"}}},
	}, func(path string) {
		if a.appearance != p {
			return
		}
		p.reading, p.status, p.failure = true, "", ""
		go func() {
			data, img, err := launcher.ReadSkinFile(path)
			var classic, slim skinPreview
			legacy := false
			if err == nil {
				legacy = img.Bounds().Dy() == 32
				classic = renderSkin(img, false)
				if !legacy {
					slim = renderSkin(img, true)
				}
			}
			a.post(func() {
				if a.appearance != p {
					return
				}
				p.reading = false
				if err != nil {
					p.draft, p.file, p.failure = nil, "", err.Error()
					return
				}
				p.file, p.draft, p.classic, p.slim, p.legacy = filepath.Base(path), data, classic, slim, legacy
				if legacy {
					p.variant = "classic"
				}
			})
		}()
	})
}

func (a *desktop) previewCape(p *appearanceView) {
	p.capeError = ""
	id := p.cape
	p.capeLoading = ""
	p.capeRequest++
	request := p.capeRequest
	if id == "" || p.capes[id] != nil {
		return
	}
	for _, cape := range p.data.Capes {
		if cape.ID != id {
			continue
		}
		p.capeLoading = id
		go func() {
			ctx, stop := context.WithTimeout(a.ctx, 15*time.Second)
			defer stop()
			img, err := launcher.FetchProfileTexture(ctx, cape.URL, false)
			var preview *ui.Bitmap
			if err == nil {
				preview = renderCape(img)
			}
			a.post(func() {
				if a.appearance != p || p.capeRequest != request {
					return
				}
				p.capeLoading = ""
				if err == nil {
					p.capes[id] = preview
				} else {
					p.capeError = "无法加载披风预览"
				}
			})
		}()
		break
	}
}

func (a *desktop) appearancePanel(c *ui.Context, p *appearanceView, disabled bool) {
	surface(c).Padding(12, 14).Gap(9).Children(func() {
		heading(c, "皮肤与披风", func() {
			if button(c, "刷新外观", action.Text, disabled || p.syncing || p.reading) {
				a.syncAppearance(p, appearanceChange{})
			}
		})
		if p.syncing || p.reading {
			label := "正在同步 Minecraft 外观…"
			if p.reading {
				label = "正在读取皮肤 PNG…"
			}
			flightLoading(c, label)
		}
		if p.status != "" {
			feedback.Alert(c, p.status, feedback.AlertProps{Type: feedback.Success})
		}
		if p.failure != "" {
			feedback.Alert(c, p.failure, feedback.AlertProps{Type: feedback.Error})
		}
		if p.warning != "" {
			feedback.Alert(c, p.warning, feedback.AlertProps{Type: feedback.Warning})
		}
		if !p.loaded {
			return
		}
		disabled = disabled || p.syncing || p.reading
		ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
			a.skinEditor(c, p, disabled)
			a.capeEditor(c, p, disabled)
		})
	})
}

func (a *desktop) skinEditor(c *ui.Context, p *appearanceView, disabled bool) {
	ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(8).Children(func() {
		display.Text(c, "皮肤", display.TextProps{Strong: true})
		preview := p.skin
		if p.draft != nil {
			preview = p.classic
			if p.variant == "slim" {
				preview = p.slim
			}
		}
		ui.Row(c).Gap(6).Children(func() {
			appearanceTile(c, "正面", preview.front, lucide.UserRound())
			appearanceTile(c, "背面", preview.back, lucide.UserRound())
		})
		if p.draft == nil {
			label := "PNG · 64 × 64 或 64 × 32"
			if p.data.Skin.URL == "" {
				label = "当前使用游戏默认皮肤"
			}
			muted(c, label).FontSize(12).SingleLine()
		} else {
			muted(c, p.file).FontSize(12).SingleLine().Ellipsis("…")
			field.Select(c, &p.variant, field.SelectProps{Disabled: disabled || p.legacy, Options: []field.SelectOption{
				{Value: "classic", Label: "经典模型"}, {Value: "slim", Label: "纤细模型"},
			}}).Label("皮肤模型")
		}
		ui.Row(c).Gap(6).Children(func() {
			if button(c, "选择 PNG…", action.Default, disabled) {
				a.chooseSkin(p)
			}
			if p.draft != nil && button(c, "取消预览", action.Text, disabled) {
				p.draft, p.file = nil, ""
			}
		})
		if p.draft != nil && button(c, "应用皮肤", action.Primary, disabled) {
			a.syncAppearance(p, appearanceChange{skin: p.draft, variant: p.variant})
		}
	})
}

func (a *desktop) capeEditor(c *ui.Context, p *appearanceView, disabled bool) {
	ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(8).Children(func() {
		display.Text(c, "披风", display.TextProps{Strong: true})
		label := "不佩戴"
		if p.cape != "" {
			label = "披风预览"
		}
		ui.Row(c).Children(func() {
			appearanceTile(c, label, p.capes[p.cape], lucide.Shirt())
		})
		if p.cape != "" && p.capeLoading == p.cape {
			flightLoading(c, "正在加载披风…")
		} else if p.capeError != "" && p.cape != "" && p.capes[p.cape] == nil {
			if button(c, "重试披风预览", action.Text, disabled) {
				a.previewCape(p)
			}
		}
		if len(p.data.Capes) == 0 {
			muted(c, "此账号暂无已拥有的披风").FontSize(12)
			return
		}
		options := []field.SelectOption{{Value: "none", Label: "不佩戴"}}
		active := p.data.ActiveCape().ID
		for i, cape := range p.data.Capes {
			name := cape.Alias
			if name == "" {
				name = fmt.Sprintf("披风 %d", i+1)
			}
			if cape.ID == active {
				name += " · 使用中"
			}
			options = append(options, field.SelectOption{Value: cape.ID, Label: name})
		}
		selected := p.cape
		if selected == "" {
			selected = "none"
		}
		field.Select(c, &selected, field.SelectProps{Options: options, Disabled: disabled}).Label("选择披风")
		if selected == "none" {
			selected = ""
		}
		if p.cape != selected {
			p.cape, p.status = selected, ""
			a.previewCape(p)
		}
		if button(c, "应用披风", action.Primary, disabled || p.cape == active) {
			a.syncAppearance(p, appearanceChange{setCape: true, cape: p.cape})
		}
	})
}

func appearanceTile(c *ui.Context, label string, img *ui.Bitmap, glyph *ui.SVG) {
	ink, wash := brand(c)
	ui.Column(c).Grow(1).Basis(0).MinWidth(0).Height(113).Radius(8).Background(wash).
		AlignItems(ui.Center).Justify(ui.Center).Gap(5).Children(func() {
		if img != nil {
			ui.Image(c, img).Size(56, 80).Fit(ui.Contain)
		} else {
			ui.Box(c).Size(56, 80).Center().Children(func() { ui.Icon(c, glyph).FontSize(25).TextColor(ink) })
		}
		muted(c, label).FontSize(12)
	})
}
