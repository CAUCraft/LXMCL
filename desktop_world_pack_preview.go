package main

import (
	"context"
	"errors"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/packmeta"
	"github.com/CAUCraft/LXMCL/internal/saves"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

type worldPackPreview struct {
	pack     saves.DataPack
	metadata packmeta.Metadata
	err      error
	cancel   context.CancelFunc
	loading  bool
}

func (p *worldPacksView) closePreview() {
	if p.preview != nil {
		p.preview.cancel()
		p.preview = nil
	}
}

func (a *desktop) openWorldPackPreview(browser *worldBrowser, i launcher.Instance, p *worldPacksView, pack saves.DataPack) {
	if a.ctx == nil || browser.packs != p || p.tab != worldPacksLocal {
		return
	}
	p.closePreview()
	ctx, cancel := context.WithCancel(a.ctx)
	preview := &worldPackPreview{pack: pack, loading: true, cancel: cancel}
	p.preview = preview
	data := p.data
	go func() {
		defer cancel()
		metadata, err := data.Preview(ctx, i.GameDir, p.world, pack)
		a.post(func() {
			if !a.form.open || a.form.worlds != browser || a.form.target != i || browser.packs != p || p.preview != preview {
				return
			}
			if current, ok := a.state.Find(i.ID); !ok || current != i {
				return
			}
			preview.metadata, preview.err, preview.loading = metadata, err, false
		})
	}()
}

func (a *desktop) showWorldPackPreview(c *ui.Context, browser *worldBrowser, i launcher.Instance, p *worldPacksView) {
	preview := p.preview
	back, refresh := false, false
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		back = button(c, "返回数据包列表", action.Default, false)
		display.Text(c, preview.pack.Name, display.TextProps{Strong: true, Rows: 1}).
			Grow(1).MinWidth(0).Ellipsis("…").Tooltip(preview.pack.Name)
		refresh = icon(c.Key(preview), lucide.RefreshCw(), "刷新预览", a.ctx == nil)
	})
	if back {
		p.closePreview()
		return
	}
	if refresh {
		a.openWorldPackPreview(browser, i, p, preview.pack)
		return
	}
	switch {
	case preview.loading:
		flightLoading(c, "正在读取 pack.mcmeta…")
	case errors.Is(preview.err, saves.ErrDataPackMetadataMissing):
		muted(c, "缺少 pack.mcmeta")
	case preview.err != nil:
		feedback.Alert(c, "无法读取 pack.mcmeta", feedback.AlertProps{Type: feedback.Warning, Description: preview.err.Error()})
	default:
		layout.ScrollArea(c.Key(preview), layout.ScrollAreaProps{}, func() {
			ui.Column(c).Gap(6).Children(func() {
				meta := preview.metadata
				hasFormat := false
				for _, field := range []struct{ label, value string }{
					{"格式编号", meta.Format}, {"声明范围", meta.Supported}, {"最低格式", meta.Min}, {"最高格式", meta.Max},
				} {
					if field.value != "" {
						muted(c, field.label+" · "+field.value).Selectable()
						hasFormat = true
					}
				}
				if !hasFormat {
					muted(c, "未声明格式")
				}
				display.Text(c, "说明", display.TextProps{Strong: true})
				if meta.Description == "" {
					muted(c, "未填写说明")
				} else {
					display.Text(c, meta.Description, display.TextProps{}).Selectable()
				}
			})
		}).MaxHeight(220)
	}
}
