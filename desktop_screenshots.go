package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

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
	query, appliedQuery                 string
	selected, viewerFrom                string
	loading, folder, exporting, copying bool
	err, previewErr                     string
	picture                             ui.ImageSource
	original                            *screenshots.Original
	scroll                              ui.ScrollState
}

func (p *screenshotGallery) visibleRows() []display.ListItem {
	if p.query == "" {
		return p.rows
	}
	if p.appliedQuery != p.query {
		p.appliedQuery, p.matches = p.query, nil
		query := strings.ToLower(strings.TrimSpace(p.query))
		for _, row := range p.rows {
			if strings.Contains(strings.ToLower(row.Key), query) {
				p.matches = append(p.matches, row)
			}
		}
	}
	return p.matches
}

func (a *desktop) closeScreenshots() {
	if p := a.screenshots; p != nil {
		if p.cancel != nil {
			p.cancel()
		}
		a.screenshots = nil
	}
}

func (a *desktop) loadScreenshots(i launcher.Instance, selected, query string) {
	a.closeScreenshots()
	p := &screenshotGallery{instance: i, loading: true, query: query}
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
			if !slices.ContainsFunc(visible, func(row display.ListItem) bool { return row.Key == selected }) {
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
	rows := p.visibleRows()
	index := slices.IndexFunc(rows, func(row display.ListItem) bool { return row.Key == p.selected })
	target := index
	var back bool
	var toCopy, toIcon *screenshots.Original
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
	}
	screenshotPreview(c, p, false)
}

func (a *desktop) instanceScreenshots(c *ui.Context, i launcher.Instance) {
	p := a.screenshots
	if p == nil || p.instance != i {
		a.loadScreenshots(i, "", "")
		p = a.screenshots
	}
	var toCopy *screenshots.Original
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
	if !p.loading && p.err == "" && (len(p.rows) > 0 || p.query != "") {
		old := p.query
		key := "mod+f"
		if a.logOpen {
			key = ""
		}
		search = field.SearchBar(c.Key("screenshot-search"), &p.query, field.SearchBarProps{
			Placeholder: "搜索截图文件名", ShortKey: key,
		}).Label("搜索截图文件名")
		rows = p.visibleRows()
		if old != p.query {
			p.restrictBatchSelection(rows)
			p.scroll = ui.ScrollState{}
			if p.selected != "" && !slices.ContainsFunc(rows, func(row display.ListItem) bool { return row.Key == p.selected }) {
				a.selectScreenshot(p, "")
			}
			c.Invalidate()
		}
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
				if button(c, "清除截图搜索", action.Default, false) {
					p.query, p.scroll = "", ui.ScrollState{}
					search.Focus()
					c.Invalidate()
				}
			}})
		})
	default:
		w, _ := c.Size()
		splitWorkspace(c, catalogListWidth(w), func() {
			muted(c, "修改时间 · 最近在前").FontSize(12)
			layout.ScrollArea(c, layout.ScrollAreaProps{State: &p.scroll}, func() {
				a.screenshotList(c, p, rows)
			}).Grow(1).MinHeight(0)
		}, func() {
			if p.selected == "" {
				muted(c, "选择截图查看预览")
				return
			}
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
	switch {
	case refresh:
		a.loadScreenshots(i, p.selected, p.query)
		c.Invalidate()
	case toCopy != nil && p.original == toCopy:
		a.copyScreenshot(p, toCopy)
	case toExport != "" && p.selected == toExport:
		a.exportScreenshot(p)
	}
}
