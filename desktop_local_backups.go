package main

import (
	"context"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/saves"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

type localBackupView struct {
	target  launcher.Instance
	entries []saves.BackupEntry
	list    ui.ListState
	cancel  context.CancelFunc
	loading bool
	err     error
}

func (a *desktop) backupRecords(id int) []saves.BackupRecord {
	var records []saves.BackupRecord
	for _, row := range a.history {
		if row.InstanceID == id && row.Action == taskBackup && row.Status == taskCompleted && row.SaveArchive != "" {
			records = append(records, saves.BackupRecord{Name: row.SaveArchive, At: row.At})
		}
	}
	return records
}

func (a *desktop) closeLocalBackups() {
	if p := a.localBackups; p != nil {
		p.cancel()
		a.localBackups = nil
	}
}

func (a *desktop) refreshLocalBackups(i launcher.Instance) {
	a.closeLocalBackups()
	ctx, cancel := context.WithCancel(a.ctx)
	p := &localBackupView{target: i, cancel: cancel, loading: true}
	a.localBackups = p
	root, known := a.store.Root, a.backupRecords(i.ID)
	go func() {
		defer cancel()
		entries, err := saves.LocalBackups(ctx, root, i.ID, known)
		a.post(func() {
			if a.localBackups == p {
				p.entries, p.err, p.loading = entries, err, false
			}
		})
	}()
}

func (a *desktop) previewLocalSaveBackup(i launcher.Instance, name string) {
	a.previewManagedSaveBackup(&restoreView{target: i, filename: name})
}

func (a *desktop) previewBackupTask(row taskRecord) {
	source := launcher.Instance{ID: row.InstanceID, Name: row.Name}
	a.previewManagedSaveBackup(&restoreView{target: source, filename: row.SaveArchive, readOnly: true})
}

func (a *desktop) previewManagedSaveBackup(p *restoreView) {
	root, id, name := a.store.Root, p.target.ID, p.filename
	a.readSavePreview(p, func(ctx context.Context) (launcher.SavePreview, error) {
		path, err := saves.CheckLocalBackup(root, id, name)
		if err != nil {
			return launcher.SavePreview{}, err
		}
		return launcher.InspectSaveArchive(ctx, path)
	})
}

func (a *desktop) localBackupDialog(c *ui.Context) {
	p := a.localBackups
	if p == nil {
		return
	}
	current, exists := a.state.Find(p.target.ID)
	if !exists || current != p.target || a.page != pageLibrary || a.inspected != current.ID || a.detailPage != detailFiles {
		a.closeLocalBackups()
		return
	}
	if a.restore != nil {
		return
	}
	open := true
	feedback.Modal(c, &open, feedback.ModalProps{Title: "本地备份", Width: 540, NoFooter: true}, func() {
		ui.Column(c).Gap(10).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				display.Text(c, current.Name, display.TextProps{Strong: true, Rows: 1}).Grow(1).MinWidth(0).Ellipsis("…")
				if icon(c, lucide.RefreshCw(), "刷新本地备份", p.loading) {
					a.refreshLocalBackups(current)
				}
			})
			switch {
			case p.loading:
				flightLoading(c, "正在读取本地备份…")
			case p.err != nil:
				feedback.Alert(c, "无法读取本地备份", feedback.AlertProps{Type: feedback.Warning, Description: p.err.Error()})
			case len(p.entries) == 0:
				muted(c, "此实例还没有本地备份。")
			default:
				const rowHeight = 45
				ui.List(c, &p.list, len(p.entries)).Height(float32(min(5, len(p.entries)) * rowHeight)).FillWidth().Rows(func(r ui.ListRow) {
					entry := p.entries[r.Index]
					ui.Row(r.Context).Height(rowHeight).AlignItems(ui.Center).Gap(10).Padding(0, 6).Children(func() {
						ui.Icon(r.Context, lucide.Archive()).FontSize(16)
						ui.Column(r.Context).Grow(1).MinWidth(0).Gap(2).Children(func() {
							display.Text(r.Context, entry.Name, display.TextProps{Rows: 1}).Ellipsis("…")
							detail := "文件已丢失"
							if !entry.Missing {
								detail = fileSizeLabel(entry.Size)
							}
							if !entry.Modified.IsZero() {
								detail = entry.Modified.Local().Format("2006-01-02 15:04") + " · " + detail
							}
							muted(r.Context, detail).FontSize(12)
						})
						if !entry.Missing && action.Button(r.Context, "预览…", action.ButtonProps{Variant: action.Default}).
							Label("预览本地备份 "+entry.Name).Disabled(a.busy || a.pickerBusy || a.running != nil).Clicked() {
							a.previewLocalSaveBackup(current, entry.Name)
						}
					})
				})
			}
			ui.Row(c).Children(func() {
				ui.Spacer(c).Grow(1)
				if button(c, "关闭", action.Default, false) {
					open = false
				}
			})
		})
	})
	if !open {
		a.closeLocalBackups()
	}
}
