package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io/fs"
	"path/filepath"
	"slices"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/screenshots"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

type screenshotGallery struct {
	instance        launcher.Instance
	cancel          context.CancelFunc
	rows            []display.ListItem
	selected        string
	loading, folder bool
	err, previewErr string
	picture         ui.ImageSource
	dimensions      image.Point
	scroll          ui.ScrollState
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
	p.selected, p.picture, p.dimensions, p.previewErr = name, nil, image.Point{}, ""
	if a.ctx == nil {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	p.cancel = cancel
	go func() {
		img, size, err := screenshots.Preview(ctx, p.instance.GameDir, name)
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
			p.picture, p.dimensions = picture, size
		})
	}()
}

func (a *desktop) instanceScreenshots(c *ui.Context, i launcher.Instance) {
	p := a.screenshots
	if p == nil || p.instance != i {
		a.loadScreenshots(i, "")
		p = a.screenshots
	}
	ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
		display.Text(c, "游戏截图", display.TextProps{Strong: true, FontSize: 16}).Grow(1)
		if icon(c, lucide.RefreshCw(), "刷新截图", p.loading) {
			a.loadScreenshots(i, p.selected)
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
			switch {
			case p.previewErr != "":
				feedback.Alert(c, "无法预览这张截图", feedback.AlertProps{Type: feedback.Warning, Description: "可选择另一张，或在文件夹中查看原图。"})
			case p.picture == nil:
				flightLoading(c, "正在读取预览…")
			default:
				sectionPane(c).Grow(1).Basis(0).MinHeight(0).Clip().Children(func() {
					ui.Image(c, p.picture).Absolute().Top(0).Left(0).Right(0).Bottom(0).Fit(ui.Contain).Label("截图预览")
				})
				display.Text(c, p.selected, display.TextProps{Strong: true, Rows: 1}).MinWidth(0).Ellipsis("…")
				muted(c, fmt.Sprintf("%d × %d", p.dimensions.X, p.dimensions.Y)).FontSize(12)
			}
		})
	}
}
