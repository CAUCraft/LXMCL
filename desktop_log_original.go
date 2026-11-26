package main

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	launcherlog "github.com/CAUCraft/LXMCL/internal/log"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
)

type originalLogExport struct {
	entry    launcherlog.Entry
	request  uint64
	fileType string
	cancel   context.CancelFunc
	writing  bool
}

func (p *instanceLogView) stopOriginalExport() {
	if p == nil {
		return
	}
	if p.original != nil {
		p.original.cancel()
		p.original = nil
	}
	p.originalDone, p.originalErr = false, ""
}

func (a *desktop) currentOriginalExport(p *instanceLogView, export *originalLogExport) bool {
	if !a.logOpen || a.logLibrary != p || p.original != export || p.reading ||
		a.page != pageLibrary || a.detailPage != detailFiles || a.inspected != p.instance.ID ||
		export.request != a.logRequest || export.fileType != p.fileType {
		return false
	}
	instance, found := a.state.Find(p.instance.ID)
	entry, selected := p.selectedEntry(a.logPath)
	return found && instance == p.instance && selected && entry == export.entry
}

func (a *desktop) originalLogAction(c *ui.Context, p *instanceLogView, entry launcherlog.Entry, selected bool) {
	if p.original != nil && p.original.writing {
		if button(c, "取消导出", action.Default, false) {
			p.original.cancel()
		}
		return
	}
	key := struct{ launcherlog.Entry }{entry}
	if button(c.Key(key), "导出原件…", action.Default, a.ctx == nil || !selected || p.reading || a.pickerBusy || a.logExporting || p.original != nil) {
		a.chooseOriginalLog(p, entry)
	}
}

func (a *desktop) chooseOriginalLog(p *instanceLogView, entry launcherlog.Entry) {
	ctx, cancel := context.WithCancel(a.ctx)
	export := &originalLogExport{entry: entry, request: a.logRequest, fileType: p.fileType, cancel: cancel}
	p.original, p.originalDone, p.originalErr = export, false, ""
	a.logExported, a.pickerBusy = false, true
	options := mygo.SaveDialogOptions{
		Parent: a.window, Title: "导出原始日志", ButtonLabel: "导出",
		DefaultPath: filepath.Base(entry.Path),
		Filters:     []mygo.FileFilter{{Name: "原始日志文件", Extensions: []string{strings.TrimPrefix(filepath.Ext(entry.Path), ".")}}},
	}
	go func() {
		path, err := mygo.Dialog.Save(options)
		a.post(func() {
			a.pickerBusy = false
			if !a.currentOriginalExport(p, export) {
				cancel()
				return
			}
			if err != nil || path == "" {
				p.original = nil
				cancel()
				if err != nil {
					p.originalErr = err.Error()
				}
				return
			}
			export.writing = true
			a.writes.Go(func() {
				defer cancel()
				err := entry.ExportOriginal(ctx, p.instance.GameDir, path)
				a.post(func() {
					if !a.currentOriginalExport(p, export) {
						return
					}
					p.original = nil
					switch {
					case err == nil:
						p.originalDone = true
					case errors.Is(err, context.Canceled):
					case errors.Is(err, fs.ErrExist):
						p.originalErr = "目标文件已存在，请选择新的文件名或保存位置。"
					case errors.Is(err, launcherlog.ErrOriginalChanged):
						p.originalErr = "原文件已移走、更换或不可访问。请刷新日志列表后重新选择。"
					default:
						p.originalErr = err.Error()
					}
				})
			})
		})
	}()
}
