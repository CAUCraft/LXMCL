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
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

type screenshotGallery struct {
	instance                            launcher.Instance
	cancel                              context.CancelFunc
	rows, matches                       []display.ListItem
	entries                             map[string]screenshots.Entry
	batch                               map[string]bool
	query, dateFilter, order            string
	selected, viewerFrom                string
	loading, folder, exporting, copying bool
	err, previewErr                     string
	picture                             ui.ImageSource
	original                            *screenshots.Original
	reveal                              *screenshotRevealRequest
	scroll                              ui.ScrollState

	appliedFilter, selectionFilter screenshotFilterKey
}

func (a *desktop) closeScreenshots() {
	if p := a.screenshots; p != nil {
		if p.cancel != nil {
			p.cancel()
		}
		a.screenshots = nil
	}
}

func (a *desktop) loadScreenshots(i launcher.Instance, selected, query, dateFilter, order string) {
	a.closeScreenshots()
	p := &screenshotGallery{instance: i, loading: true, query: query, dateFilter: dateFilter, order: order}
	a.screenshots = p
	if a.ctx == nil {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	p.cancel = cancel
	go func() {
		entries, err := screenshots.List(ctx, i.GameDir)
		rows := make([]display.ListItem, 0, len(entries))
		byName := make(map[string]screenshots.Entry, len(entries))
		for _, e := range entries {
			rows = append(rows, display.ListItem{Key: e.Name, Label: e.Name, Description: e.Modified.Local().Format("2006-01-02 15:04")})
			byName[e.Name] = e
		}
		a.post(func() {
			if ctx.Err() != nil || a.screenshots != p {
				return
			}
			p.loading, p.folder, p.rows, p.entries = false, err == nil, rows, byName
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				p.err = err.Error()
			}
			visible := p.visibleRows()
			if len(visible) == 0 {
				return
			}
			if !screenshotIsVisible(visible, selected) {
				if dateFilter != "" {
					return
				}
				selected = visible[0].Key
			}
			a.selectScreenshot(p, selected)
		})
	}()
}

func (a *desktop) selectScreenshot(p *screenshotGallery, name string) {
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.selected, p.picture, p.original, p.previewErr, p.copying = name, nil, nil, "", false
	p.reveal = nil
	if name == "" || a.ctx == nil {
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
			if ctx.Err() != nil || a.screenshots != p || !screenshotIsVisible(p.visibleRows(), name) {
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

func (a *desktop) currentScreenshotGallery(p *screenshotGallery) bool {
	if a.screenshots != p || a.page != pageLibrary || a.detailPage != detailScreenshots || a.inspected != p.instance.ID {
		return false
	}
	current, ok := a.state.Find(p.instance.ID)
	return ok && current == p.instance
}

func (a *desktop) copyScreenshot(p *screenshotGallery, original *screenshots.Original) {
	if a.ctx == nil || !a.currentScreenshotGallery(p) || !screenshotIsVisible(p.visibleRows(), p.selected) {
		return
	}
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
			if !a.currentScreenshotGallery(p) || !screenshotIsVisible(p.visibleRows(), p.selected) {
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
	name, original := p.selected, p.original
	if !a.currentScreenshotGallery(p) || !screenshotIsVisible(p.visibleRows(), name) {
		return
	}
	a.pickPath(mygo.OpenDialogOptions{Title: "选择截图导出目录", ButtonLabel: "导出到此处", Directory: true, CreateDirectories: true}, func(folder string) {
		if !a.currentScreenshotGallery(p) || p.selected != name || p.original != original ||
			!screenshotIsVisible(p.visibleRows(), name) || p.exporting || a.busy {
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
	p.viewerFrom, p.reveal = "", nil
	if p.selected != from && screenshotIsVisible(p.visibleRows(), from) {
		a.selectScreenshot(p, from)
	}
}

func (a *desktop) screenshotViewer(c *ui.Context, p *screenshotGallery) {
	rows := a.currentScreenshotRows(c, p)
	index := slices.IndexFunc(rows, func(row display.ListItem) bool { return row.Key == p.selected })
	if index < 0 {
		p.viewerFrom, p.reveal = "", nil
		c.Invalidate()
		return
	}
	target := index
	var back bool
	var toCopy, toIcon, toReveal *screenshots.Original
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		back = button(c, "返回图库", action.Default, false)
		display.Text(c, p.selected, display.TextProps{Strong: true, Rows: 1}).Grow(1).MinWidth(0).Ellipsis("…")
		if p.original != nil {
			muted(c, fmt.Sprintf("%d × %d", p.original.Size.X, p.original.Size.Y)).Shrink(0)
		}
		if icon(c, lucide.ChevronLeft(), "上一张", index <= 0) || !a.form.open && c.Shortcut(0, ui.KeyLeft) {
			target = index - 1
		}
		muted(c, fmt.Sprintf("%d / %d", index+1, len(rows)))
		if icon(c, lucide.ChevronRight(), "下一张", index+1 >= len(rows)) || !a.form.open && c.Shortcut(0, ui.KeyRight) {
			target = index + 1
		}
		toCopy = screenshotCopyButton(c, p)
		toReveal = screenshotRevealButton(c, p)
		if button(c.Key(p.original).Key("icon"), "用作实例图标…", action.Default, p.original == nil || a.busy || a.pickerBusy) {
			toIcon = p.original
		}
	})
	switch {
	case back:
		a.leaveScreenshotViewer(p)
		c.Invalidate()
		return
	case target != index && target >= 0 && target < len(rows):
		a.selectScreenshot(p, rows[target].Key)
		c.Invalidate()
	case toCopy != nil:
		a.copyScreenshot(p, toCopy)
	case toIcon != nil:
		a.editScreenshotIcon(p, toIcon)
	case toReveal != nil:
		a.revealScreenshot(p, toReveal)
	}
	screenshotPreview(c, p, false)
}

func (a *desktop) instanceScreenshots(c *ui.Context, i launcher.Instance) {
	p := a.screenshots
	if p == nil || p.instance != i {
		a.loadScreenshots(i, "", "", "", "")
		p = a.screenshots
	}
	var toCopy, toReveal *screenshots.Original
	var toExport string
	var refresh bool
	ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
		display.Text(c, "游戏截图", display.TextProps{Strong: true, FontSize: 16}).Grow(1)
		refresh = icon(c, lucide.RefreshCw(), "刷新截图", p.loading)
		toCopy = screenshotCopyButton(c, p)
		if button(c, "导出原图…", action.Default, p.picture == nil || p.exporting || a.pickerBusy || a.busy) {
			toExport = p.selected
		}
		if p.batch == nil {
			if button(c.Key("batch"), "批量导出…", action.Default, len(p.rows) == 0 || p.exporting || a.pickerBusy || a.busy) {
				p.batch = make(map[string]bool)
			}
		} else if button(c.Key("batch"), "退出批量", action.Default, a.busy || a.pickerBusy) {
			p.batch = nil
		}
		if button(c, "打开截图文件夹", action.Default, !p.folder) {
			a.openPath(filepath.Join(i.GameDir, screenshots.Directory), false)
		}
	})
	var rows []display.ListItem
	var search ui.Element
	if !p.loading && p.err == "" && (len(p.rows) > 0 || p.query != "" || p.dateFilter != "") {
		key := "mod+f"
		if a.logOpen {
			key = ""
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			search = field.SearchBar(c.Key("screenshot-search"), &p.query, field.SearchBarProps{
				Placeholder: "搜索截图文件名", ShortKey: key,
			}).Label("搜索截图文件名").Grow(1).MinWidth(0)
			muted(c, "按修改日期").FontSize(12).Shrink(0)
			field.Select(c.Key("screenshot-date-filter"), &p.dateFilter, field.SelectProps{Options: screenshotDateOptions}).
				Label("修改日期").Width(116).Shrink(0)
			muted(c, "按修改时间").FontSize(12).Shrink(0)
			field.Select(c.Key("screenshot-order"), &p.order, field.SelectProps{Options: screenshotOrderOptions}).
				Label("截图排序").Width(104).Shrink(0)
		})
		rows = a.currentScreenshotRows(c, p)
	}
	if p.batch != nil {
		a.screenshotBatchActions(c, p, rows)
	}
	switch {
	case p.loading:
		flightLoading(c, "正在读取截图…")
	case p.err != "":
		feedback.Alert(c, "无法读取截图目录", feedback.AlertProps{Type: feedback.Error, Description: p.err})
	case len(p.rows) == 0:
		ui.Column(c).Grow(1).Center().Children(func() {
			display.Empty(c, display.EmptyProps{Icon: lucide.Image(), Title: "还没有游戏截图", Description: "在此实例中按 F2 截图，然后刷新此页。"})
		})
	case len(rows) == 0:
		ui.Column(c).Grow(1).Center().Children(func() {
			display.Empty(c, display.EmptyProps{Icon: lucide.Search(), Title: "没有匹配的截图", Action: func() {
				label := "清除截图搜索"
				if p.dateFilter != "" {
					label = "清除截图筛选"
				}
				if button(c, label, action.Default, false) {
					p.query, p.dateFilter = "", ""
					search.Focus()
					c.Invalidate()
				}
			}})
		})
	default:
		w, _ := c.Size()
		splitWorkspace(c, catalogListWidth(w), func() {
			layout.ScrollArea(c, layout.ScrollAreaProps{State: &p.scroll}, func() {
				a.screenshotList(c, p, rows)
			}).Grow(1).MinHeight(0)
		}, func() {
			if p.selected == "" {
				muted(c, "选择截图查看预览")
				return
			}
			expanded := screenshotPreview(c, p, true)
			if expanded {
				p.viewerFrom, p.reveal = p.selected, nil
				c.Invalidate()
			}
			if p.original != nil {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Children(func() {
						display.Text(c, p.selected, display.TextProps{Strong: true, Rows: 1}).Ellipsis("…")
						muted(c, fmt.Sprintf("%d × %d", p.original.Size.X, p.original.Size.Y)).FontSize(12)
					})
					if original := screenshotRevealButton(c, p); !expanded {
						toReveal = original
					}
				})
			}
		})
	}
	switch {
	case refresh:
		a.loadScreenshots(i, p.selected, p.query, p.dateFilter, p.order)
		c.Invalidate()
	case toCopy != nil && p.original == toCopy:
		a.copyScreenshot(p, toCopy)
	case toExport != "" && p.selected == toExport:
		a.exportScreenshot(p)
	case toReveal != nil:
		a.revealScreenshot(p, toReveal)
	}
}
