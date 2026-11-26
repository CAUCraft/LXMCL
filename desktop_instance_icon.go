package main

import (
	"context"
	"errors"
	"fmt"
	"image"

	"github.com/CAUCraft/LXMCL/internal/iconimage"
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/screenshots"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
)

type instanceIconDraft struct {
	original, encoded string
	picture           ui.ImageSource
	reading           bool
	screenshot        *screenshotIconSource
	cancel            context.CancelFunc
}

type screenshotIconSource struct {
	gallery  *screenshotGallery
	original *screenshots.Original
	name     string
}

func loadInstanceIcons(s launcher.State) map[int]ui.ImageSource {
	icons := make(map[int]ui.ImageSource, len(s.InstanceIcons))
	for _, i := range s.Instances {
		if data := s.InstanceIcons[i.ID]; data != "" {
			if img, err := iconimage.Decode(data); err == nil {
				icons[i.ID] = ui.NewBitmap(img)
			}
		}
	}
	return icons
}

func instanceIcon(c *ui.Context, i launcher.Instance, picture ui.ImageSource, size float32, label string) {
	if picture != nil {
		ui.Image(c, picture).Size(size, size).Fit(ui.Contain).Shrink(0).Label(label)
		return
	}
	ui.Icon(c, loaderIcon(i)).FontSize(size).Shrink(0).Label(label)
}

func (a *desktop) editInstanceIcon(i launcher.Instance) {
	data := a.state.InstanceIcons[i.ID]
	d := &instanceIconDraft{original: data, encoded: data, picture: a.instanceIcons[i.ID]}
	a.form = editor{mode: "icon", open: true, target: i, icon: d}
	if data != "" && d.picture == nil {
		a.form.err = "无法读取已保存的图标，请重新选择 PNG 或恢复默认图标"
	}
}

func (a *desktop) chooseInstanceIcon(d *instanceIconDraft) {
	a.pickPath(mygo.OpenDialogOptions{Title: "选择实例图标", Filters: []mygo.FileFilter{{Name: "PNG 图片", Extensions: []string{"png"}}}}, func(path string) {
		if !a.form.open || a.form.icon != d {
			return
		}
		d.reading = true
		go func() {
			img, data, err := iconimage.ImportPNG(a.ctx, path)
			a.post(func() {
				if !a.form.open || a.form.icon != d {
					return
				}
				d.reading = false
				if err != nil {
					a.form.err = "无法读取 PNG：" + err.Error()
					return
				}
				a.form.err = ""
				d.encoded, d.picture = data, ui.NewBitmap(img)
			})
		}()
	})
}

func (a *desktop) editScreenshotIcon(p *screenshotGallery, original *screenshots.Original) {
	source := &screenshotIconSource{gallery: p, original: original, name: p.selected}
	if a.form.open || a.ctx == nil || !a.screenshotIconCurrent(source) {
		return
	}
	a.editInstanceIcon(p.instance)
	d := a.form.icon
	d.screenshot = source
	a.form.err = ""
	a.readScreenshotIcon(d, false)
}

func (a *desktop) screenshotIconCurrent(source *screenshotIconSource) bool {
	p := source.gallery
	if a.screenshots != p || a.page != pageLibrary || a.detailPage != detailScreenshots ||
		a.inspected != p.instance.ID || p.viewerFrom == "" || p.original != source.original || p.selected != source.name {
		return false
	}
	i, ok := a.state.Find(p.instance.ID)
	return ok && i == p.instance
}

func (a *desktop) readScreenshotIcon(d *instanceIconDraft, apply bool) {
	source := d.screenshot
	if !a.screenshotIconCurrent(source) {
		a.form.open = false
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	d.reading, d.cancel = true, cancel
	go func() {
		png, err := source.original.PNGWithin(ctx, iconimage.MaxImportBytes)
		var problem, data string
		var img image.Image
		switch {
		case errors.Is(err, screenshots.ErrPNGTooLarge):
			problem = "这张截图超过实例图标的 32 MiB 限制，请选择其他截图"
		case err != nil:
			problem = "无法读取原图，请关闭后刷新截图再试"
		case !apply:
			img, data, err = iconimage.ImportPNGBytes(ctx, png)
			if err != nil {
				problem = "无法用这张截图生成图标：" + err.Error()
			}
		}
		a.post(func() {
			defer cancel()
			if ctx.Err() != nil || !a.form.open || a.form.icon != d {
				return
			}
			d.reading, d.cancel = false, nil
			if !a.screenshotIconCurrent(source) {
				a.form.open = false
				return
			}
			if problem != "" {
				a.form.err = problem
				return
			}
			if apply {
				a.commitEditor()
			} else {
				d.encoded, d.picture = data, ui.NewBitmap(img)
			}
		})
	}()
}

func (a *desktop) instanceIconEditor(c *ui.Context) bool {
	d := a.form.icon
	wait := d.reading || a.pickerBusy || a.busy
	if d.screenshot != nil {
		display.Text(c, fmt.Sprintf("实例 #%d · %s", a.form.target.ID, d.screenshot.name), display.TextProps{Rows: 1}).
			MinWidth(0).Ellipsis("…")
		if d.reading {
			flightLoading(c, "正在核对截图…")
		}
	}
	ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
		instanceIcon(c, a.form.target, d.picture, 64, "图标预览")
		ui.Column(c).Gap(8).Children(func() {
			if d.screenshot == nil && button(c, "选择 PNG…", action.Default, wait) {
				a.chooseInstanceIcon(d)
			}
			if button(c, "恢复默认图标", action.Text, wait || d.encoded == "" || d.screenshot != nil && a.form.err != "") {
				d.encoded, d.picture, a.form.err = "", nil, ""
			}
		})
	})
	if a.form.err != "" {
		feedback.Alert(c, a.form.err, feedback.AlertProps{Type: feedback.Error})
	}
	submit := false
	ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
		if button(c, "取消", action.Default, false) {
			a.form.open = false
		}
		submit = button(c, "应用图标", action.Primary, wait || d.original == d.encoded || a.form.err != "")
	})
	return submit
}
