package main

import (
	"context"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

type resourcePackView struct {
	instance                launcher.Instance
	revealing, revealFailed bool
	file                    launcher.ContentFile
	data                    content.ResourcePackPreview
	picture                 ui.ImageSource
	loading                 bool
	err                     error
}

func (a *desktop) inPackPreview(p *resourcePackView) bool {
	if a.content.packPreview != p || a.page != pageLibrary || a.detailPage != detailContent ||
		a.inspected != p.instance.ID || a.content.kind() != launcher.ResourcePacks {
		return false
	}
	instance, ok := a.state.Find(p.instance.ID)
	return ok && instance == p.instance
}

func (a *desktop) closePackPreview() {
	if a.contentCancel != nil {
		a.contentCancel()
	}
	a.content.packPreview = nil
}

func (a *desktop) openPackPreview(i launcher.Instance, file launcher.ContentFile) {
	if a.ctx == nil {
		return
	}
	a.closePackPreview()
	ctx, cancel := context.WithCancel(a.ctx)
	a.contentCancel = cancel
	p := &resourcePackView{instance: i, file: file, loading: true}
	a.content.packPreview = p
	go func() {
		defer cancel()
		data, err := content.PreviewResourcePack(ctx, i, file)
		a.post(func() {
			if !a.inPackPreview(p) {
				return
			}
			p.data, p.err, p.loading = data, err, false
			if data.Cover != nil {
				p.picture = ui.NewBitmap(data.Cover)
			}
		})
	}()
}

func (a *desktop) revealPackFile(p *resourcePackView) {
	if a.ctx == nil || p.loading || p.revealing || !a.inPackPreview(p) {
		return
	}
	p.revealing, p.revealFailed = true, false
	go func() {
		path, err := p.file.ResourcePackRevealPath(p.instance)
		a.post(func() {
			if !a.inPackPreview(p) {
				return
			}
			p.revealing, p.revealFailed = false, err != nil
			if err == nil {
				mygo.Shell.ShowItemInFolder(path)
			}
		})
	}()
}

func (a *desktop) resourcePackPreview(c *ui.Context, p *resourcePackView) {
	var back, reveal, refresh bool
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		backLabel := "返回资源包列表"
		if a.content.savedPacks != nil {
			backLabel = "返回游戏上次选择"
		}
		back = icon(c, lucide.ArrowLeft(), backLabel, false)
		display.Text(c, p.file.DiskName(), display.TextProps{Strong: true, FontSize: 16, Rows: 1}).Grow(1).MinWidth(0).Ellipsis("…")
		reveal = icon(c, lucide.FolderOpen(), "在文件夹中显示", p.loading || p.revealing)
		refresh = icon(c, lucide.RefreshCw(), "重新读取资源包", p.loading)
	})
	switch {
	case back:
		a.closePackPreview()
		c.Invalidate()
		return
	case refresh:
		a.openPackPreview(p.instance, p.file)
		c.Invalidate()
		return
	case reveal:
		a.revealPackFile(p)
	}
	if p.revealFailed {
		feedback.Alert(c, "无法定位资源包", feedback.AlertProps{Type: feedback.Error, Description: "请返回并刷新资源列表后重试。"})
	}
	if p.loading {
		flightLoading(c, "正在读取资源包…")
		return
	}
	if p.err != nil {
		feedback.Alert(c, "无法预览资源包", feedback.AlertProps{Type: feedback.Error, Description: p.err.Error()})
		return
	}
	layout.ScrollArea(c.Key(p), layout.ScrollAreaProps{}, func() {
		ui.Column(c).Gap(14).Children(func() {
			ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
				sectionPane(c).Size(100, 100).Shrink(0).Center().Children(func() {
					if p.picture != nil {
						ui.Image(c, p.picture).Size(96, 96).Fit(ui.Contain).Label("资源包封面")
						return
					}
					ui.Icon(c, lucide.Image()).FontSize(24)
					label := "无封面"
					if p.data.CoverErr != nil {
						label = "封面不可读"
					}
					muted(c, label).FontSize(12)
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					kind := "ZIP 资源包"
					if p.file.Directory {
						kind = "目录资源包"
					}
					display.Text(c, kind, display.TextProps{Strong: true})
					if p.data.MetadataErr != nil {
						return
					}
					for _, field := range []struct{ label, value string }{
						{"格式编号", p.data.Format}, {"声明范围", p.data.Supported}, {"最低格式", p.data.Min}, {"最高格式", p.data.Max},
					} {
						if field.value != "" {
							muted(c, field.label+" · "+field.value).Selectable()
						}
					}
					if p.data.Format == "" && p.data.Supported == "" && p.data.Min == "" && p.data.Max == "" {
						muted(c, "格式未知")
					}
				})
			})
			if p.data.MetadataErr != nil {
				feedback.Alert(c, "无法读取 pack.mcmeta", feedback.AlertProps{Type: feedback.Warning, Description: p.data.MetadataErr.Error()})
				return
			}
			display.Text(c, "说明", display.TextProps{Strong: true})
			if p.data.Description == "" {
				muted(c, "没有可显示的说明")
				return
			}
			display.Text(c, p.data.Description, display.TextProps{}).Selectable()
		})
	}).Grow(1).MinHeight(0)
}
