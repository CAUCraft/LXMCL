package main

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/saves"
	"github.com/egoist/mygo"
)

func (a *desktop) chooseLocalBackupExport(p *localBackupView, entry saves.BackupEntry) {
	if a.ctx == nil || a.busy || a.pickerBusy || a.running != nil || p.exportCancel != nil || entry.Missing {
		return
	}
	a.pickerBusy = true
	p.exportErr, p.exported = "", ""
	options := mygo.SaveDialogOptions{
		Parent: a.window, Title: "导出存档备份", ButtonLabel: "导出", DefaultPath: entry.Name,
		Filters: []mygo.FileFilter{{Name: "存档备份 ZIP", Extensions: []string{"zip"}}},
	}
	go func() {
		destination, err := mygo.Dialog.Save(options)
		a.post(func() {
			a.pickerBusy = false
			if a.localBackups != p {
				return
			}
			if err != nil {
				p.exportErr = err.Error()
				return
			}
			if destination == "" {
				return
			}
			i, ok := a.state.Find(p.target.ID)
			if !ok || i != p.target || a.page != pageLibrary || a.detailPage != detailFiles || a.inspected != i.ID {
				return
			}
			if a.busy || a.running != nil {
				p.exportErr = "请在当前任务结束后重试。"
				return
			}
			ctx, cancel := context.WithCancel(a.ctx)
			p.exportCancel = cancel
			root := a.store.Root
			a.writes.Go(func() {
				defer cancel()
				err := saves.ExportLocalBackup(ctx, root, i.ID, entry, destination)
				a.post(func() {
					if a.localBackups != p {
						return
					}
					p.exportCancel = nil
					switch {
					case err == nil:
						p.exported = filepath.Base(destination)
					case errors.Is(err, context.Canceled):
					case errors.Is(err, fs.ErrExist):
						p.exportErr = "目标文件已存在，请选择新的文件名或保存位置。"
					case errors.Is(err, saves.ErrLocalBackupMissing):
						p.exportErr = "原备份文件已丢失，请刷新本地备份列表。"
					case errors.Is(err, saves.ErrLocalBackupChanged):
						p.exportErr = "原备份文件已更改，请刷新列表后重新选择。"
					default:
						p.exportErr = err.Error()
					}
				})
			})
		})
	}()
}
