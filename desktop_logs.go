package main

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	launcherlog "github.com/CAUCraft/LXMCL/internal/log"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

type instanceLogView struct {
	instance launcher.Instance
	entries  []launcherlog.Entry
	reading  bool
	err      string
}

func (a *desktop) showInstanceLogs(i launcher.Instance) {
	a.logLibrary = &instanceLogView{instance: i}
	a.stopLogRead()
	a.logOpen = true
	a.logPath, a.logText, a.logError, a.logExportErr = "", "", "", ""
	a.logQuery, a.logFilter, a.logScroll.Y, a.logExported = "", 0, 0, false
	a.refreshLogFiles()
}

func (a *desktop) refreshLogFiles() {
	p := a.logLibrary
	if p == nil || p.reading || a.ctx == nil {
		return
	}
	p.reading, p.err = true, ""
	go func() {
		entries, err := launcherlog.Recent(a.ctx, p.instance.GameDir)
		a.post(func() {
			if a.logLibrary != p {
				return
			}
			p.entries, p.reading = entries, false
			if err != nil {
				p.err = err.Error()
			}
			if !a.logOpen {
				return
			}
			path := a.logPath
			if !slices.ContainsFunc(entries, func(e launcherlog.Entry) bool { return e.Path == path }) {
				path = ""
				if len(entries) > 0 {
					path = entries[0].Path
				}
			}
			if path == "" {
				a.stopLogRead()
				a.logPath, a.logText, a.logError = "", "", ""
			} else {
				a.selectLog(path)
			}
		})
	}()
}

func (a *desktop) logFilePicker(c *ui.Context) {
	p := a.logLibrary
	options := make([]field.SelectOption, 0, len(p.entries))
	for _, entry := range p.entries {
		kind := "启动日志"
		switch filepath.Base(filepath.Dir(entry.Path)) {
		case "logs":
			kind = "游戏日志"
			if strings.HasSuffix(entry.Path, ".log.gz") {
				kind = "归档日志"
			}
		case "crash-reports":
			kind = "崩溃报告"
		}
		label := entry.Modified.Local().Format("01-02 15:04") + " · " + kind + " · " + filepath.Base(entry.Path)
		options = append(options, field.SelectOption{Value: entry.Path, Label: label})
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		path := a.logPath
		placeholder := "暂无日志，运行游戏后可刷新"
		if p.reading {
			placeholder = "正在读取日志列表…"
		}
		if len(options) > 0 {
			placeholder = ""
		}
		catalogPicker(c, &path, options, "选择实例日志", placeholder, len(options) == 0).Grow(1).MinWidth(0)
		if path != a.logPath {
			a.selectLog(path)
		}
		if icon(c, lucide.RefreshCw(), "刷新日志列表", p.reading) {
			a.refreshLogFiles()
		}
	})
}
