package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/screenshots"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

type screenshotGallery struct {
	instance                            launcher.Instance
	cancel                              context.CancelFunc
	rows                                []display.ListItem
	selected, viewerFrom                string
	loading, folder, exporting, copying bool
	err, previewErr                     string
	picture                             ui.ImageSource
	original                            *screenshots.Original
	scroll                              ui.ScrollState
}

func (a *desktop) closeScreenshots() {
	if p := a.screenshots; p != nil {
		if p.cancel != nil {
			p.cancel()
		}
		a.screenshots = nil
	}
}

func (a *desktop) loadScreenshots(i launcher.Instance, selected string) {
	a.closeScreenshots()
	p := &screenshotGallery{instance: i, loading: true}
	a.screenshots = p
	if a.ctx == nil {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	p.cancel = cancel
	go func() {
		entries, err := screenshots.List(ctx, i.GameDir)
		rows := make([]display.ListItem, 0, len(entries))
		for _, e := range entries {
			rows = append(rows, display.ListItem{Key: e.Name, Label: e.Name, Description: e.Modified.Local().Format("2006-01-02 15:04")})
		}
		a.post(func() {
			if ctx.Err() != nil || a.screenshots != p {
				return
			}
			p.loading, p.folder, p.rows = false, err == nil, rows
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				p.err = err.Error()
			}
			if len(rows) == 0 {
				return
			}
			if !slices.ContainsFunc(rows, func(row display.ListItem) bool { return row.Key == selected }) {
				selected = rows[0].Key
			}
			a.selectScreenshot(p, selected)
		})
	}()
}

func (a *desktop) selectScreenshot(p *screenshotGallery, name string) {
	if p.cancel != nil {
		p.cancel()
	}
	p.selected, p.picture, p.original, p.previewErr, p.copying = name, nil, nil, "", false
	if a.ctx == nil {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	p.cancel = cancel
	go func() {
		img, original, err := screenshots.Preview(ctx, p.instance.GameDir, name)
		var picture ui.ImageSource
		if err == nil && ctx.Err() == nil {
			picture = ui.NewBitmap(img)
		}
		a.post(func() {
			if ctx.Err() != nil || a.screenshots != p {
				return
			}
			if err != nil {
				p.previewErr = err.Error()
				return
			}
			p.picture, p.original = picture, original
		})
	}()
}

func (a *desktop) copyScreenshot(p *screenshotGallery, original *screenshots.Original) {
	ctx, cancel := context.WithCancel(a.ctx)
	if p.cancel != nil {
		p.cancel()
	}
	p.cancel, p.copying = cancel, true
	go func() {
		defer cancel()
		png, err := original.PNG(ctx)
		var data transfer.Data
		if err == nil {
			data = transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, png)))
		}
		a.post(func() {
			if a.screenshots != p || p.original != original {
				return
			}
			p.copying = false
			if a.page != pageLibrary || a.detailPage != detailScreenshots || a.inspected != p.instance.ID {
				return
			}
			if err != nil {
				a.message("无法读取原图，请刷新截图后重试", err)
				return
			}
			if err := mygo.Clipboard.Write(data); err != nil {
				a.message("无法复制图片，请重试", err)
				return
			}
			a.message("已复制图片", nil)
		})
	}()
}

func (a *desktop) exportScreenshot(p *screenshotGallery) {
	name := p.selected
	a.pickPath(mygo.OpenDialogOptions{Title: "选择截图导出目录", ButtonLabel: "导出到此处", Directory: true, CreateDirectories: true}, func(folder string) {
		if a.screenshots != p || p.selected != name || p.exporting {
			return
		}
		p.exporting = true
		go func() {
			err := screenshots.Export(a.ctx, p.instance.GameDir, name, folder)
			a.post(func() {
				p.exporting = false
				if err != nil {
					a.message("无法导出截图", err)
					return
				}
				a.message("已导出截图 · "+filepath.Join(folder, name), nil)
			})
		}()
	})
}

func screenshotPreview(c *ui.Context, p *screenshotGallery, expandable bool) bool {
	switch {
	case p.previewErr != "":
		feedback.Alert(c, "无法预览这张截图", feedback.AlertProps{Type: feedback.Warning, Description: "可选择另一张，或在文件夹中查看原图。"})
	case p.picture == nil:
		flightLoading(c, "正在读取预览…")
	default:
		frame := layout.Block(c, layout.BlockProps{Variant: layout.BlockBorderless, Clickable: expandable}).
			Grow(1).Basis(0).MinHeight(0).Clip()
		if expandable {
			frame.Label("查看大图")
		}
		frame.Children(func() {
			ui.Image(c, p.picture).Absolute().Top(0).Left(0).Right(0).Bottom(0).Fit(ui.Contain).Label("截图预览")
			if expandable {
				ui.Box(c).Absolute().Top(6).Right(6).Padding(5).Radius(5).Background(theme.TokensOf(c).BgContainer).Children(func() {
					ui.Icon(c, lucide.Maximize2()).FontSize(16)
				})
			}
		})
		return expandable && frame.Clicked()
	}
	return false
}

func screenshotCopyButton(c *ui.Context, p *screenshotGallery) *screenshots.Original {
	if button(c.Key(p.original), "复制图片", action.Default, p.original == nil || p.copying) {
		return p.original
	}
	return nil
}

func (a *desktop) leaveScreenshotViewer(p *screenshotGallery) {
	from := p.viewerFrom
	p.viewerFrom = ""
	if p.selected != from {
		a.selectScreenshot(p, from)
	}
}

func (a *desktop) screenshotViewer(c *ui.Context, p *screenshotGallery) {
	index := slices.IndexFunc(p.rows, func(row display.ListItem) bool { return row.Key == p.selected })
	target := index
	var back bool
	var toCopy *screenshots.Original
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		back = button(c, "返回图库", action.Default, false)
		display.Text(c, p.selected, display.TextProps{Strong: true, Rows: 1}).Grow(1).MinWidth(0).Ellipsis("…")
		if p.original != nil {
			muted(c, fmt.Sprintf("%d × %d", p.original.Size.X, p.original.Size.Y)).Shrink(0)
		}
		if icon(c, lucide.ChevronLeft(), "上一张", index <= 0) || c.Shortcut(0, ui.KeyLeft) {
			target = index - 1
		}
		muted(c, fmt.Sprintf("%d / %d", index+1, len(p.rows)))
		if icon(c, lucide.ChevronRight(), "下一张", index+1 >= len(p.rows)) || c.Shortcut(0, ui.KeyRight) {
			target = index + 1
		}
		toCopy = screenshotCopyButton(c, p)
	})
	switch {
	case back:
		a.leaveScreenshotViewer(p)
		c.Invalidate()
		return
	case target != index && target >= 0 && target < len(p.rows):
		a.selectScreenshot(p, p.rows[target].Key)
		c.Invalidate()
	case toCopy != nil:
		a.copyScreenshot(p, toCopy)
	}
	screenshotPreview(c, p, false)
}

func (a *desktop) instanceScreenshots(c *ui.Context, i launcher.Instance) {
	p := a.screenshots
	if p == nil || p.instance != i {
		a.loadScreenshots(i, "")
		p = a.screenshots
	}
	var toCopy *screenshots.Original
	ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
		display.Text(c, "游戏截图", display.TextProps{Strong: true, FontSize: 16}).Grow(1)
		if icon(c, lucide.RefreshCw(), "刷新截图", p.loading) {
			a.loadScreenshots(i, p.selected)
		}
		toCopy = screenshotCopyButton(c, p)
		if button(c, "导出原图…", action.Default, p.picture == nil || p.exporting || a.pickerBusy || a.busy) {
			a.exportScreenshot(p)
		}
		if button(c, "打开截图文件夹", action.Default, !p.folder) {
			a.openPath(filepath.Join(i.GameDir, screenshots.Directory), false)
		}
	})
	switch {
	case p.loading:
		flightLoading(c, "正在读取截图…")
	case p.err != "":
		feedback.Alert(c, "无法读取截图目录", feedback.AlertProps{Type: feedback.Error, Description: p.err})
	case len(p.rows) == 0:
		ui.Column(c).Grow(1).Center().Children(func() {
			display.Empty(c, display.EmptyProps{Icon: lucide.Image(), Title: "还没有游戏截图", Description: "在此实例中按 F2 截图，然后刷新此页。"})
		})
	default:
		w, _ := c.Size()
		splitWorkspace(c, catalogListWidth(w), func() {
			muted(c, "修改时间 · 最近在前").FontSize(12)
			layout.ScrollArea(c, layout.ScrollAreaProps{State: &p.scroll}, func() {
				if name := display.List(c, display.ListProps{Items: p.rows, Active: p.selected, Compact: true}); name != "" && name != p.selected {
					a.selectScreenshot(p, name)
				}
			}).Grow(1).MinHeight(0)
		}, func() {
			if screenshotPreview(c, p, true) {
				p.viewerFrom = p.selected
				c.Invalidate()
			}
			if p.original != nil {
				display.Text(c, p.selected, display.TextProps{Strong: true, Rows: 1}).MinWidth(0).Ellipsis("…")
				muted(c, fmt.Sprintf("%d × %d", p.original.Size.X, p.original.Size.Y)).FontSize(12)
			}
		})
	}
	if toCopy != nil && a.screenshots == p && p.original == toCopy {
		a.copyScreenshot(p, toCopy)
	}
}
