package main

import (
	"path/filepath"
	"runtime"
	"strings"

	launcherlog "github.com/CAUCraft/LXMCL/internal/log"
	"github.com/egoist/mygo"
)

func (a *desktop) exportLog(preview string) {
	if a.ctx == nil || a.pickerBusy || a.logReading || a.logExporting || preview == "" {
		return
	}
	source, request := a.logPath, a.logRequest
	query, filter := a.logQuery, a.logFilter
	a.pickerBusy, a.logExporting, a.logExported, a.logExportErr = true, true, false, ""
	options := mygo.SaveDialogOptions{
		Parent: a.window, Title: "导出当前日志预览", ButtonLabel: "导出",
		DefaultPath: strings.TrimSuffix(filepath.Base(source), filepath.Ext(source)) + "-preview.txt",
		Filters:     []mygo.FileFilter{{Name: "文本文件", Extensions: []string{"txt"}}},
	}
	go func() {
		path, err := mygo.Dialog.Save(options)
		if err == nil && path != "" {
			err = launcherlog.ExportPreview(source, path, preview)
		}
		a.post(func() {
			a.pickerBusy, a.logExporting = false, false
			if !a.logOpen || a.logRequest != request || a.logQuery != query || a.logFilter != filter {
				return
			}
			if err != nil {
				a.logExportErr = err.Error()
			} else {
				a.logExported = path != ""
			}
		})
	}()
}

func (a *desktop) pickPath(options mygo.OpenDialogOptions, accept func(string)) {
	a.pickPaths(options, func(paths []string) { accept(paths[0]) })
}

func (a *desktop) pickPaths(options mygo.OpenDialogOptions, accept func([]string)) {
	if !a.ready || a.busy || a.pickerBusy {
		return
	}
	a.pickerBusy = true
	options.Parent = a.window
	go func() {
		paths, err := mygo.Dialog.Open(options)
		a.post(func() {
			a.pickerBusy = false
			if err != nil {
				a.message("无法打开系统选择窗口", err)
			} else if len(paths) > 0 {
				accept(paths)
			}
		})
	}()
}

func (a *desktop) pickGameRoot() {
	a.pickPath(mygo.OpenDialogOptions{
		Title: "选择新实例的默认目录", DefaultPath: strings.TrimSpace(a.draft.GameRoot),
		Directory: true, CreateDirectories: true, ButtonLabel: "选择目录",
	}, func(path string) { a.draft.GameRoot = path })
}

func (a *desktop) pickDownloadFolder(accept func(string)) {
	a.pickPath(mygo.OpenDialogOptions{
		Title: "保存到文件夹", DefaultPath: a.downloadFolder, Directory: true,
		CreateDirectories: true, ButtonLabel: "保存到此处",
	}, func(path string) {
		a.downloadFolder = path
		accept(path)
	})
}

func (a *desktop) pickJava() {
	a.pickJavaPath(a.draft.JavaPath, func(path string) {
		a.draft.JavaPath = path
		a.javaInfo = ""
	})
}

func (a *desktop) pickJavaPath(current string, accept func(string)) {
	options := mygo.OpenDialogOptions{
		Title: "选择 Java 可执行文件", DefaultPath: strings.TrimSpace(current), ButtonLabel: "选择 Java",
	}
	if runtime.GOOS == "windows" {
		options.Filters = []mygo.FileFilter{{Name: "可执行文件", Extensions: []string{"exe"}}}
	}
	a.pickPath(options, accept)
}
