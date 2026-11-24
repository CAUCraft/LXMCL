package main

import (
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
)

type installDirectoryCheck struct {
	path     string
	loading  bool
	occupied bool
	err      error
}

func (a *desktop) installDestinationPreview(c *ui.Context, p *installView, loader string) bool {
	target, err := a.state.PreviewNewInstance(a.installDraftName(p, loader), p.version)
	if target.GameDir != "" {
		settingsRow(c, "目标目录（预览）", func() {
			previewPath(c, target.GameDir, "复制新实例目录")
		})
	}
	if err != nil {
		p.directory = nil
		feedback.Alert(c, err.Error(), feedback.AlertProps{Type: feedback.Warning})
		return false
	}

	check := p.directory
	if check == nil || check.path != target.GameDir {
		p.directory = nil
		if a.ctx == nil {
			return true
		}
		a.checkInstallDirectory(p, target.GameDir)
		check = p.directory
	}
	conflict := i18n.Text("instance.directory_occupied")
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if button(c, "重新检查", action.Default, a.busy || check.loading) {
			if p.err == conflict {
				p.err = ""
			}
			a.checkInstallDirectory(p, target.GameDir)
		}
		switch {
		case check.loading:
			muted(c, "正在检查目录…").FontSize(13)
		case check.err != nil:
			muted(c, "无法检查目标目录").FontSize(13).Tooltip(check.err.Error())
		case !check.occupied:
			muted(c, "目录未占用").FontSize(13)
		}
	})
	if check.occupied {
		if p.err == conflict {
			p.err = ""
		}
		feedback.Alert(c, conflict, feedback.AlertProps{Type: feedback.Warning})
	}
	return !check.occupied
}

func (a *desktop) checkInstallDirectory(p *installView, path string) {
	request := &installDirectoryCheck{path: path, loading: true}
	p.directory = request
	go func() {
		occupied, err := launcher.CheckNewInstanceDirectory(path)
		a.post(func() {
			if p.directory == request {
				request.loading, request.occupied, request.err = false, occupied, err
			}
		})
	}()
}
