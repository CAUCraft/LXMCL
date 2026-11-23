package main

import (
	"github.com/CAUCraft/LXMCL/internal/iconimage"
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
)

type instanceIconDraft struct {
	original, encoded string
	picture           ui.ImageSource
	reading           bool
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

func (a *desktop) instanceIconEditor(c *ui.Context) bool {
	d := a.form.icon
	wait := d.reading || a.pickerBusy || a.busy
	ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
		instanceIcon(c, a.form.target, d.picture, 64, "图标预览")
		ui.Column(c).Gap(8).Children(func() {
			if button(c, "选择 PNG…", action.Default, wait) {
				a.chooseInstanceIcon(d)
			}
			if button(c, "恢复默认图标", action.Text, wait || d.encoded == "") {
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
