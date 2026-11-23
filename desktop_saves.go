package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/saves"
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
	readOnly bool // Task history previews do not offer restore or archive selection.
	err      string
	cancel   context.CancelFunc
}

func (a *desktop) backupSaves(i launcher.Instance) {
	a.runSaveBackup(i, "", func(ctx context.Context, root string) (string, error) {
		return launcher.BackupSaves(ctx, i, root)
	})
}

func (a *desktop) runSaveBackup(i launcher.Instance, world string, backup func(context.Context, string) (string, error)) {
	current, ok := a.state.Find(i.ID)
	if !ok || current.Revision != i.Revision || a.busy || a.running != nil || a.ctx == nil {
		return
	}
	root := a.store.Root
	row := taskRecord{Action: taskBackup, InstanceID: i.ID, Revision: i.Revision, Name: i.Name, Version: i.Version, World: world}
	var archive string
	a.runFileTaskWithResult(row, func(ctx context.Context, _ func(launcher.Progress)) error {
		var err error
		archive, err = backup(ctx, root)
		return err
	}, func(record *taskRecord) {
		record.SaveArchive = filepath.Base(archive)
	}, func(err error) {
		if err == nil && a.backups != nil && a.backups.instanceID == i.ID {
			a.refreshBackups(i.ID)
		}
	})
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
	a.readSavePreview(&restoreView{target: i, filename: filepath.Base(path)}, func(ctx context.Context) (launcher.SavePreview, error) {
		return launcher.InspectSaveArchive(ctx, path)
	})
}

func (a *desktop) readSavePreview(p *restoreView, inspect func(context.Context) (launcher.SavePreview, error)) {
	if a.restore != nil && a.restore.cancel != nil {
		a.restore.cancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	p.loading, p.cancel = true, cancel
	a.restore = p
	go func() {
		defer cancel()
		preview, err := inspect(ctx)
		a.post(func() {
			if a.restore != p {
				return
			}
			p.loading = false
			if err != nil {
				p.err = err.Error()
				if p.readOnly && errors.Is(err, fs.ErrNotExist) {
					p.err = "备份文件已丢失，请检查原实例的备份目录。"
				}
				return
			}
			p.worlds = preview.Worlds()
			if !p.readOnly {
				p.preview = preview
			}
		})
	}()
}

func (a *desktop) restoreDialog(c *ui.Context) {
	p := a.restore
	if p == nil {
		return
	}
	current, exists := a.state.Find(p.target.ID)
	stale := !p.readOnly && (!exists || current.Revision != p.target.Revision)
	title, source, errorTitle := "恢复存档", "目标实例 · "+p.target.Name, "无法恢复此归档"
	if p.readOnly {
		title, source, errorTitle = "备份预览", fmt.Sprintf("原实例 · %s · #%d", p.target.Name, p.target.ID), "无法预览此归档"
	}
	open := true
	feedback.Modal(c, &open, feedback.ModalProps{Title: title, Width: 480, NoFooter: true}, func() {
		ui.Column(c).Gap(10).Children(func() {
			panelHeading(c, lucide.Archive(), p.filename, source)
			switch {
			case stale:
				feedback.Alert(c, "实例已更改", feedback.AlertProps{Type: feedback.Warning, Description: "请返回游戏文件页，重新选择恢复目标。"})
			case p.loading:
				flightLoading(c, "正在检查 ZIP 中的世界…")
			case p.err != "":
				feedback.Alert(c, errorTitle, feedback.AlertProps{Type: feedback.Error, Description: p.err})
			case len(p.worlds) == 1:
				display.Text(c, "世界 · "+p.worlds[0], display.TextProps{Strong: true}).Selectable()
			default:
				label := "恢复范围"
				if p.readOnly {
					label = "归档内的世界"
				}
				display.Text(c, label, display.TextProps{Strong: true})
				p.worldList(c)
			}
			if !p.readOnly && !stale && len(p.worlds) > 0 {
				display.Text(c, "同名世界会另存为新副本，例如「生存 (2)」。", display.TextProps{FontSize: 13})
			}
			layout.Divider(c, layout.DividerProps{})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if p.readOnly {
					ui.Spacer(c).Grow(1)
					if button(c, "关闭", action.Default, false) {
						open = false
					}
					return
				}
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
	if p.readOnly || a.ctx == nil || a.busy || a.pickerBusy || a.running != nil || len(p.worlds) == 0 || p.selected < 0 || p.selected > len(p.worlds) {
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
	a.runInstanceTask(p.target, label, "", func(ctx context.Context) error {
		_, err := p.preview.Restore(ctx, p.target.GameDir, world)
		return err
	})
	return true
}

func (p *restoreView) worldList(c *ui.Context) {
	count := len(p.worlds)
	label := func(row int) string { return p.worlds[row] }
	if !p.readOnly {
		count++
		label = func(row int) string {
			if row == 0 {
				return fmt.Sprintf("全部世界（%d 个）", len(p.worlds))
			}
			return p.worlds[row-1]
		}
		p.list.Selected = &p.selected
	}
	p.list.Label = label
	ui.List(c, &p.list, count).Height(float32(min(4, count)) * 34).FillWidth().Rows(func(row ui.ListRow) {
		rc := row.Context
		ui.Row(rc).Height(34).Padding(0, 10).Gap(8).AlignItems(ui.Center).Children(func() {
			glyph := lucide.Folder()
			if row.Index == 0 && !p.readOnly {
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

type backupView struct {
	instanceID int
	entries    []saves.BackupEntry
	cancel     context.CancelFunc
	loading    bool
	err        string
}

func (a *desktop) refreshBackups(id int) {
	if a.backups != nil && a.backups.cancel != nil {
		a.backups.cancel()
	}
	p := &backupView{instanceID: id}
	a.backups = p
	if a.ctx == nil {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	p.loading, p.cancel = true, cancel
	root, known := a.store.Root, a.backupRecords(id)
	go func() {
		defer cancel()
		entries, err := saves.LocalBackups(ctx, root, id, known)
		entries = slices.DeleteFunc(entries, func(entry saves.BackupEntry) bool { return entry.Missing })
		entries = entries[:min(3, len(entries))]
		a.post(func() {
			if a.backups != p {
				return
			}
			p.entries, p.loading = entries, false
			if err != nil {
				p.err = err.Error()
			}
		})
	}()
}

func (a *desktop) recentBackupList(c *ui.Context, i launcher.Instance, disabled bool) {
	if a.backups == nil || a.backups.instanceID != i.ID {
		a.refreshBackups(i.ID)
	}
	p := a.backups
	sectionPane(c).Padding(8, 14).Gap(4).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			display.Text(c, "最近备份", display.TextProps{Strong: true}).Grow(1)
			if button(c, "查看全部备份…", action.Default, a.ctx == nil) {
				a.refreshLocalBackups(i)
			}
			if icon(c, lucide.RefreshCw(), "刷新最近备份", p.loading) {
				a.refreshBackups(i.ID)
			}
		})
		if p.err != "" {
			feedback.Alert(c, "无法读取备份目录", feedback.AlertProps{Type: feedback.Warning, Description: p.err})
		} else if p.loading {
			muted(c, "正在读取备份列表…").FontSize(12)
		} else if len(p.entries) == 0 {
			muted(c, "此实例还没有本地备份。").FontSize(12)
		}
		for _, entry := range p.entries {
			ui.Row(c.Key(entry.Name)).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, lucide.Archive()).FontSize(16)
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					display.Text(c, entry.Name, display.TextProps{FontSize: 13, Rows: 1}).Ellipsis("…")
					muted(c, entry.Modified.Local().Format("2006-01-02 15:04")+" · "+fileSizeLabel(entry.Size)).FontSize(12)
				})
				if action.Button(c, "预览…", action.ButtonProps{Variant: action.Default}).Label("预览备份 " + entry.Name).Disabled(disabled).Clicked() {
					a.previewLocalSaveBackup(i, entry.Name)
				}
			})
		}
	})
}
