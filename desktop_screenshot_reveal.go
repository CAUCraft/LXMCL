package main

import (
	"errors"

	"github.com/CAUCraft/LXMCL/internal/screenshots"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

type screenshotRevealRequest struct {
	original   *screenshots.Original
	name, view string
	filter     screenshotFilterKey
}

func screenshotRevealButton(c *ui.Context, p *screenshotGallery) *screenshots.Original {
	if icon(c.Key(&p.reveal), lucide.FolderOpen(), "在文件夹中显示", p.original == nil || p.reveal != nil) {
		return p.original
	}
	return nil
}

func (a *desktop) revealScreenshot(p *screenshotGallery, original *screenshots.Original) {
	if a.ctx == nil || original == nil || p.reveal != nil || !a.currentScreenshotGallery(p) ||
		p.original != original || !screenshotIsVisible(p.visibleRows(), p.selected) {
		return
	}
	request := &screenshotRevealRequest{original: original, name: p.selected, view: p.viewerFrom, filter: p.appliedFilter}
	p.reveal = request
	go func() {
		path, err := original.RevealPath(a.ctx)
		a.post(func() {
			if p.reveal != request {
				return
			}
			p.reveal = nil
			if !a.currentScreenshotGallery(p) || a.form.open || a.logOpen ||
				p.original != request.original || p.selected != request.name || p.viewerFrom != request.view ||
				!screenshotIsVisible(p.visibleRows(), request.name) || p.appliedFilter != request.filter {
				return
			}
			if err != nil {
				a.message("无法定位截图", errors.New("请刷新截图后重试。"))
				return
			}
			mygo.Shell.ShowItemInFolder(path)
		})
	}()
}
