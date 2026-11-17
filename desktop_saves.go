package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

type restoreView struct {
	target   launcher.Instance
	filename string
	preview  launcher.SavePreview
	worlds   []string
	selected int // 0 restores all worlds, other rows use worlds[selected-1].
	list     ui.ListState
	loading  bool
	err      string
	cancel   context.CancelFunc
}

func (a *desktop) pickSaveArchive(i launcher.Instance) {
	previous := a.restore
	a.pickPath(mygo.OpenDialogOptions{Title: "选择存档 ZIP", DefaultPath: launcher.BackupsDir(a.store.Root, i.ID),
		Filters: []mygo.FileFilter{{Name: "存档归档", Extensions: []string{"zip"}}}}, func(path string) {
		current, ok := a.state.Find(i.ID)
		if !ok || current.Revision != i.Revision || a.inspected != i.ID || a.page != pageLibrary || a.detailPage != detailFiles || a.restore != previous {
			return
		}
		a.previewSaveArchive(i, path)
	})
}

func (a *desktop) previewSaveArchive(i launcher.Instance, path string) {
	if a.restore != nil && a.restore.cancel != nil {
		a.restore.cancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	p := &restoreView{target: i, filename: filepath.Base(path), loading: true, cancel: cancel}
	a.restore = p
	go func() {
		defer cancel()
		preview, err := launcher.InspectSaveArchive(ctx, path)
		a.post(func() {
			if a.restore != p {
				return
			}
			p.loading = false
			if err != nil {
				p.err = err.Error()
				return
			}
			p.preview, p.worlds = preview, preview.Worlds()
		})
	}()
}

func (a *desktop) restoreDialog(c *ui.Context) {
	p := a.restore
	if p == nil {
		return
	}
	current, exists := a.state.Find(p.target.ID)
	stale := !exists || current.Revision != p.target.Revision
	open := true
	feedback.Modal(c, &open, feedback.ModalProps{Title: "恢复存档", Width: 480, NoFooter: true}, func() {
		ui.Column(c).Gap(10).Children(func() {
			panelHeading(c, lucide.Archive(), p.filename, "目标实例 · "+p.target.Name)
			switch {
			case stale:
				feedback.Alert(c, "实例已更改", feedback.AlertProps{Type: feedback.Warning, Description: "请返回游戏文件页，重新选择恢复目标。"})
			case p.loading:
				flightLoading(c, "正在检查 ZIP 中的世界…")
			case p.err != "":
				feedback.Alert(c, "无法恢复此归档", feedback.AlertProps{Type: feedback.Error, Description: p.err})
			case len(p.worlds) == 1:
				display.Text(c, "世界 · "+p.worlds[0], display.TextProps{Strong: true}).Selectable()
			default:
				display.Text(c, "恢复范围", display.TextProps{Strong: true})
				p.worldPicker(c)
			}
			if !stale && len(p.worlds) > 0 {
				display.Text(c, "同名世界会另存为新副本，例如「生存 (2)」。", display.TextProps{FontSize: 13})
			}
			layout.Divider(c, layout.DividerProps{})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if !stale && !p.loading && button(c, "重新选择 ZIP…", action.Text, a.pickerBusy || a.busy) {
					a.pickSaveArchive(p.target)
				}
				ui.Spacer(c).Grow(1)
				if stale {
					if button(c, "返回实例", action.Default, false) {
						if exists {
							a.inspect(current)
							a.detailPage = detailFiles
						} else {
							a.page, a.inspected = pageLibrary, 0
						}
						open = false
					}
					return
				}
				if button(c, "取消", action.Default, false) {
					open = false
				}
				if len(p.worlds) > 0 {
					label := "恢复世界"
					if len(p.worlds) > 1 && p.selected == 0 {
						label = "恢复全部世界"
					}
					if button(c, label, action.Primary, a.busy || a.pickerBusy || a.running != nil || p.selected < 0) {
						open = !a.startRestore(p)
					}
				}
			})
		})
	})
	if !open {
		if p.cancel != nil {
			p.cancel()
		}
		a.restore = nil
	}
}

func (a *desktop) startRestore(p *restoreView) bool {
	if a.ctx == nil || a.busy || a.pickerBusy || a.running != nil || len(p.worlds) == 0 || p.selected < 0 || p.selected > len(p.worlds) {
		return false
	}
	world := ""
	if p.selected > 0 {
		world = p.worlds[p.selected-1]
	}
	count := len(p.worlds)
	if world != "" {
		count = 1
	}
	label := fmt.Sprintf("恢复存档 · %d 个世界", count)
	a.runInstanceTask(p.target, label, func(ctx context.Context) error {
		_, err := p.preview.Restore(ctx, p.target.GameDir, world)
		return err
	})
	return true
}

func (p *restoreView) worldPicker(c *ui.Context) {
	label := func(row int) string {
		if row == 0 {
			return fmt.Sprintf("全部世界（%d 个）", len(p.worlds))
		}
		return p.worlds[row-1]
	}
	p.list.Label, p.list.Selected = label, &p.selected
	ui.List(c, &p.list, len(p.worlds)+1).Height(float32(min(4, len(p.worlds)+1)) * 34).FillWidth().Rows(func(row ui.ListRow) {
		rc := row.Context
		ui.Row(rc).Height(34).Padding(0, 10).Gap(8).AlignItems(ui.Center).Children(func() {
			glyph := lucide.Folder()
			if row.Index == 0 {
				glyph = lucide.Layers()
			}
			ui.Icon(rc, glyph).FontSize(15)
			ui.Text(rc, label(row.Index)).SingleLine().Grow(1).MinWidth(0).Ellipsis("…")
			if row.Selected() {
				ui.Icon(rc, lucide.Check()).FontSize(15)
			}
		})
	})
}
