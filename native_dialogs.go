package main

import (
	"runtime"
	"strings"

	"github.com/egoist/mygo"
)

func (a *desktop) pickPath(options mygo.OpenDialogOptions, accept func(string)) {
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
				accept(paths[0])
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
