package main

import (
	"context"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/saves"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

type localBackupView struct {
	target  launcher.Instance
	entries []saves.BackupEntry
	query   string
	list    ui.ListState
	cancel  context.CancelFunc
	loading bool
	err     error

	exportCancel context.CancelFunc
	exportErr    string
	exported     string
}

func (p *localBackupView) visibleEntries() []saves.BackupEntry {
	query := strings.ToLower(strings.TrimSpace(p.query))
	if query == "" {
		return p.entries
	}
	var visible []saves.BackupEntry
	for _, entry := range p.entries {
		if strings.Contains(strings.ToLower(entry.Name), query) {
			visible = append(visible, entry)
		}
	}
	return visible
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
		if p.exportCancel != nil {
			p.exportCancel()
		}
		a.localBackups = nil
	}
}

func (a *desktop) refreshLocalBackups(i launcher.Instance) {
	query := ""
	if previous := a.localBackups; previous != nil && previous.target == i {
		query = previous.query
	}
	a.closeLocalBackups()
	ctx, cancel := context.WithCancel(a.ctx)
	p := &localBackupView{target: i, query: query, cancel: cancel, loading: true}
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
	exporting := p.exportCancel != nil
	locked := a.busy || a.pickerBusy || a.running != nil || exporting
	open := true
	feedback.Modal(c, &open, feedback.ModalProps{Title: "本地备份", Width: 540, NoFooter: true}, func() {
		ui.Column(c).Gap(10).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				display.Text(c, current.Name, display.TextProps{Strong: true, Rows: 1}).Grow(1).MinWidth(0).Ellipsis("…")
				ui.Column(c).Width(236).Children(func() {
					previous := p.query
					field.SearchBar(c, &p.query, field.SearchBarProps{Placeholder: "搜索备份文件名"}).Label("搜索备份文件名")
					if p.query != previous {
						p.list = ui.ListState{}
					}
				})
				if icon(c, lucide.RefreshCw(), "刷新本地备份", p.loading || a.pickerBusy || exporting) {
					a.refreshLocalBackups(current)
				}
			})
			entries := p.visibleEntries()
			switch {
			case p.loading:
				flightLoading(c, "正在读取本地备份…")
			case p.err != nil:
				feedback.Alert(c, "无法读取本地备份", feedback.AlertProps{Type: feedback.Warning, Description: p.err.Error()})
			case len(p.entries) == 0:
				muted(c, "此实例还没有本地备份。")
			case len(entries) == 0:
				muted(c, "没有匹配的备份文件。")
			default:
				const rowHeight = 45
				rows := 5
				if p.exportErr != "" {
					rows = 4
				}
				p.list.Key = func(index int) any { return entries[index].Name }
				ui.List(c, &p.list, len(entries)).Height(float32(min(rows, len(entries)) * rowHeight)).FillWidth().Rows(func(r ui.ListRow) {
					entry := entries[r.Index]
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
						if entry.Missing {
							return
						}
						if action.Button(r.Context, "预览…", action.ButtonProps{Variant: action.Default}).
							Label("预览本地备份 " + entry.Name).Disabled(locked).Clicked() {
							a.previewLocalSaveBackup(current, entry.Name)
						}
						if action.Button(r.Context, "导出…", action.ButtonProps{Variant: action.Default}).
							Label("导出备份 " + entry.Name).Disabled(locked).Clicked() {
							a.chooseLocalBackupExport(p, entry)
						}
					})
				})
			}
			if p.exportErr != "" {
				feedback.Alert(c, "无法导出备份", feedback.AlertProps{Type: feedback.Error, Variant: feedback.Plain, Description: p.exportErr})
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				switch {
				case exporting:
					muted(c, "正在导出备份…").FontSize(12)
				case p.exported != "":
					display.Text(c, "已导出备份", display.TextProps{Rows: 1})
					muted(c, p.exported).FontSize(12).MinWidth(0).SingleLine().Ellipsis("…")
				}
				ui.Spacer(c).Grow(1)
				if exporting && button(c, "取消导出", action.Default, false) {
					p.exportCancel()
				}
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
