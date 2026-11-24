package main

import (
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

type installExistingView struct {
	version string
	scroll  ui.ScrollState
}

func (a *desktop) browseExistingVersion(version string) {
	a.install.existing = &installExistingView{version: version}
	a.page, a.inspected = pageLibrary, 0
}

func (a *desktop) returnToInstallDraft() {
	a.install.existing = nil
	a.page, a.downloadPage, a.inspected = pageDownloads, downloadGame, 0
}

func (a *desktop) existingVersionLibrary(c *ui.Context, view *installExistingView) {
	heading(c, "Minecraft "+view.version+" · 已有实例", func() {
		if button(c, "返回安装", action.Default, false) {
			a.returnToInstallDraft()
			c.Invalidate()
		}
	})
	var rows []launcher.Instance
	for _, i := range a.state.Instances {
		if matchesInstanceVersion(i, view.version) {
			rows = append(rows, i)
		}
	}
	if len(rows) == 0 {
		ui.Column(c).Grow(1).Center().Children(func() {
			display.Empty(c, display.EmptyProps{Icon: lucide.PackageOpen(), Title: "此版本暂无实例"})
		})
		return
	}
	a.libraryWorkspace(c, a.orderLibraryInstances(rows), view)
}
