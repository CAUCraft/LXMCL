package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"unicode"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

type savedPacksView struct {
	target   launcher.Instance
	rows     []content.SavedPack
	copyText string
	copyErr  error
	err      error
	loading  bool
	copied   bool
	scroll   ui.ListState
}

func (a *desktop) closeSavedPacks() {
	if a.contentCancel != nil {
		a.contentCancel()
	}
	a.content.savedPacks = nil
}

func (a *desktop) openSavedPacks(i launcher.Instance) {
	if a.ctx == nil {
		return
	}
	a.closeSavedPacks()
	ctx, cancel := context.WithCancel(a.ctx)
	a.contentCancel = cancel
	p := &savedPacksView{target: i, loading: true}
	a.content.savedPacks = p
	go func() {
		defer cancel()
		rows, err := content.SavedResourcePacks(ctx, i)
		var copyText string
		if err == nil {
			copyText = savedPackOrderClipboard(rows)
		}
		a.post(func() {
			if a.content.savedPacks == p {
				p.rows, p.copyText, p.err, p.loading = rows, copyText, err, false
			}
		})
	}()
}

func (a *desktop) savedPackOrder(c *ui.Context, i launcher.Instance, p *savedPacksView) {
	if p.target != i {
		a.closeSavedPacks()
		return
	}
	back, refresh, copyOrder := false, false, false
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		back = icon(c, lucide.ArrowLeft(), "返回本地资源包", false)
		display.Text(c, "游戏上次选择", display.TextProps{Strong: true, FontSize: 16}).Grow(1)
		copyOrder = button(c.Key(p), "复制资源包顺序", action.Default, p.copyText == "")
		refresh = icon(c, lucide.RefreshCw(), "重新读取游戏选择", p.loading)
	})
	switch {
	case back:
		a.closeSavedPacks()
		return
	case refresh:
		a.openSavedPacks(i)
		return
	case copyOrder:
		a.copySavedPackOrder(c, p)
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		muted(c, "按 options.txt 中上次保存的顺序排列").FontSize(13).Grow(1).MinWidth(0)
		if p.copied {
			muted(c, "已复制资源包顺序").FontSize(12)
		}
	})
	if p.copyErr != nil {
		feedback.Alert(c, "复制资源包顺序失败，请重试", feedback.AlertProps{Type: feedback.Error, Description: p.copyErr.Error()})
	}
	switch {
	case p.loading:
		flightLoading(c, "正在读取游戏选择…")
	case errors.Is(p.err, fs.ErrNotExist):
		muted(c, "此实例还没有 options.txt。")
	case p.err != nil:
		feedback.Alert(c, "无法读取资源包选择", feedback.AlertProps{Type: feedback.Error, Description: p.err.Error()})
	case p.rows == nil:
		muted(c, "options.txt 中没有资源包选择记录。")
	case len(p.rows) == 0:
		muted(c, "上次保存的资源包列表为空。")
	default:
		ui.List(c, &p.scroll, len(p.rows)).Grow(1).MinHeight(0).Rows(func(row ui.ListRow) {
			c, pack := row.Context, p.rows[row.Index]
			ui.Row(c).Padding(4, 8).MinHeight(44).Gap(12).AlignItems(ui.Center).Children(func() {
				muted(c, fmt.Sprint(row.Index+1)).MinWidth(24)
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					display.Text(c, pack.ID, display.TextProps{Strong: true, Rows: 1}).Ellipsis("…").Selectable()
					label := "来源未确认"
					switch pack.Kind {
					case content.BuiltinPack:
						label = "游戏内置标识"
					case content.LoaderPackID:
						label = "加载器标识"
					case content.MissingPack:
						label = "本地缺失"
					case content.LocalPack:
						label = "已安装 · ZIP"
						if pack.File.Directory {
							label = "已安装 · 目录"
						} else if !pack.File.Enabled {
							label = "本地文件已停用"
						}
					}
					muted(c, label).FontSize(12)
				})
				if pack.Kind == content.LocalPack && action.Button(c, "预览", action.ButtonProps{Variant: action.Default}).Label(fmt.Sprintf("预览第 %d 项", row.Index+1)).Clicked() {
					a.openPackPreview(i, *pack.File)
				}
			})
		})
	}
}

func savedPackOrderClipboard(rows []content.SavedPack) string {
	if len(rows) == 0 {
		return ""
	}
	var text strings.Builder
	text.WriteString("游戏上次保存的资源包顺序")
	for n, pack := range rows {
		fmt.Fprintf(&text, "\n%d. %s", n+1, savedPackClipboardLabel(pack.ID))
	}
	return text.String()
}

func savedPackClipboardLabel(id string) string {
	name, file := strings.CutPrefix(id, "file/")
	if !storage.ValidFilename(name) || strings.ContainsAny(name, "\u2028\u2029") ||
		strings.ContainsFunc(name, func(r rune) bool { return unicode.Is(unicode.Bidi_Control, r) }) {
		return "标识已隐藏"
	}
	if file {
		return name + "（文件）"
	}
	return name
}

func (a *desktop) currentSavedPackOrder(p *savedPacksView) bool {
	if a.page != pageLibrary || a.detailPage != detailContent || a.inspected != p.target.ID ||
		a.content.kind() != launcher.ResourcePacks || a.content.savedPacks != p || a.content.packPreview != nil {
		return false
	}
	current, ok := a.state.Find(p.target.ID)
	return ok && current == p.target
}

func (a *desktop) copySavedPackOrder(c *ui.Context, p *savedPacksView) {
	if p.copyText == "" || !a.currentSavedPackOrder(p) {
		return
	}
	p.copied, p.copyErr = false, nil
	c.WriteClipboardThen(p.copyText, func(err error) {
		if !a.currentSavedPackOrder(p) {
			return
		}
		p.copied, p.copyErr = err == nil, err
	})
}
