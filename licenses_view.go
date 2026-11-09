package main

import (
	"embed"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

//go:embed THIRD_PARTY_NOTICES.txt licenses/*.txt
var licenseFiles embed.FS

var licenseOptions = []field.SelectOption{
	{Value: "THIRD_PARTY_NOTICES.txt", Label: "组件清单"},
	{Value: "licenses/mygo-mit.txt", Label: "MyGo · MIT"},
	{Value: "licenses/lobe-mygo-mit.txt", Label: "Lobe MyGo · MIT"},
	{Value: "licenses/lucide-feather.txt", Label: "Lucide / Feather · ISC / MIT"},
	{Value: "licenses/mygo-kage-apache.txt", Label: "Ebitengine Kage · Apache-2.0"},
	{Value: "licenses/purego-apache.txt", Label: "Purego · Apache-2.0"},
	{Value: "licenses/go-text-typesetting.txt", Label: "Go Text Typesetting"},
	{Value: "licenses/go-bsd.txt", Label: "Go 标准库 · BSD-3-Clause"},
	{Value: "licenses/go-x-bsd.txt", Label: "Go 扩展包 · BSD-3-Clause"},
	{Value: "licenses/go-patents.txt", Label: "Go · PATENTS"},
	{Value: "licenses/nsis.txt", Label: "NSIS · Windows 安装器"},
}

func (a *desktop) licensesDialog(c *ui.Context) {
	feedback.Modal(c, &a.licensesOpen, feedback.ModalProps{Title: "第三方许可证", Width: 650, NoFooter: true}, func() {
		if a.licenseFile == "" {
			a.licenseFile = licenseOptions[0].Value
		}
		ui.Column(c).Gap(8).Children(func() {
			field.Select(c, &a.licenseFile, field.SelectProps{Options: licenseOptions}).Label("组件")
			b, err := licenseFiles.ReadFile(a.licenseFile)
			if err != nil {
				feedback.Alert(c, "无法读取许可证", feedback.AlertProps{Type: feedback.Error})
				return
			}
			_, height := c.Size()
			layout.ScrollArea(c.Key(a.licenseFile), layout.ScrollAreaProps{}, func() {
				display.Text(c, string(b), display.TextProps{FontSize: 12}).Padding(10).Selectable()
			}).Height(max(160, min(330, height-220))).Radius(8).Background(theme.TokensOf(c).FillTertiary)
			ui.Row(c).Justify(ui.End).Gap(6).AlignItems(ui.Center).Children(func() {
				action.CopyButton(c, string(b), action.CopyButtonProps{})
				if button(c, "关闭", action.Default, false) {
					a.licensesOpen = false
				}
			})
		})
	})
}
