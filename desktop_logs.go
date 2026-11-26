package main

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	launcherlog "github.com/CAUCraft/LXMCL/internal/log"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/field"
)

// Keep filtering and line counting off the repeated scroll and hover frames.
type logPreview struct {
	source, query, text string
	problemsOnly        bool
	lines               int
}

func (p *logPreview) update(source, query string, problemsOnly bool) {
	if p.source == source && p.query == query && p.problemsOnly == problemsOnly {
		return
	}
	p.source, p.query, p.problemsOnly = source, query, problemsOnly
	p.text = launcherlog.Filter(source, query, problemsOnly)
	p.lines = 0
	if p.text != "" {
		p.lines = strings.Count(strings.TrimSuffix(p.text, "\n"), "\n") + 1
	}
}

type instanceLogView struct {
	instance           launcher.Instance
	entries            []launcherlog.Entry
	reading, revealing bool
	revealFailed       bool
	err, fileType      string
}

const (
	launchLogFile  = "启动日志"
	latestLogFile  = "最新游戏日志"
	debugLogFile   = "调试日志"
	archiveLogFile = "归档日志"
	crashLogFile   = "崩溃报告"
)

var logFileTypes = []field.SelectOption{
	{Value: "", Label: "全部类型"},
	{Value: launchLogFile, Label: launchLogFile},
	{Value: latestLogFile, Label: latestLogFile},
	{Value: debugLogFile, Label: debugLogFile},
	{Value: archiveLogFile, Label: archiveLogFile},
	{Value: crashLogFile, Label: crashLogFile},
}

// Entries from Recent already have recognized names and directories.
func logFileType(path string) string {
	switch filepath.Base(filepath.Dir(path)) {
	case "logs":
		switch filepath.Base(path) {
		case "latest.log":
			return latestLogFile
		case "debug.log":
			return debugLogFile
		default:
			return archiveLogFile
		}
	case "crash-reports":
		return crashLogFile
	default:
		return launchLogFile
	}
}

func (p *instanceLogView) visibleEntries() []launcherlog.Entry {
	if p.fileType == "" {
		return p.entries
	}
	var visible []launcherlog.Entry
	for _, entry := range p.entries {
		if logFileType(entry.Path) == p.fileType {
			visible = append(visible, entry)
		}
	}
	return visible
}

func (p *instanceLogView) selectedEntry(path string) (launcherlog.Entry, bool) {
	for _, entry := range p.entries {
		if entry.Path == path && (p.fileType == "" || p.fileType == logFileType(path)) {
			return entry, true
		}
	}
	return launcherlog.Entry{}, false
}

func (a *desktop) revealInstanceLog(p *instanceLogView, entry launcherlog.Entry) {
	if a.ctx == nil || p.reading || p.revealing {
		return
	}
	request, fileType := a.logRequest, p.fileType
	p.revealing, p.revealFailed = true, false
	go func() {
		err := entry.CheckReveal(p.instance.GameDir)
		a.post(func() {
			p.revealing = false
			if !a.logOpen || a.logLibrary != p || p.reading || request != a.logRequest || fileType != p.fileType {
				return
			}
			instance, found := a.state.Find(p.instance.ID)
			current, selected := p.selectedEntry(a.logPath)
			if !found || instance != p.instance || !selected || current != entry {
				return
			}
			if err != nil {
				p.revealFailed = true
				return
			}
			mygo.Shell.ShowItemInFolder(entry.Path)
		})
	}()
}

func (a *desktop) clearHiddenLog(entries []launcherlog.Entry) {
	if a.logPath != "" && !slices.ContainsFunc(entries, func(e launcherlog.Entry) bool { return e.Path == a.logPath }) {
		a.stopLogRead()
		a.resetLogPreview("")
	}
}

func (a *desktop) showInstanceLogs(i launcher.Instance) {
	a.logLibrary = &instanceLogView{instance: i}
	a.stopLogRead()
	a.logOpen = true
	a.resetLogPreview("")
	a.refreshLogFiles()
}

func (a *desktop) resetLogPreview(path string) {
	a.logPath, a.logText, a.logError = path, "", ""
	a.logQuery, a.logFilter, a.logScroll.Y = "", 0, 0
	a.logDiagnosis = launcherlog.Diagnosis{}
	a.logExported, a.logExportErr = false, ""
	if p := a.logLibrary; p != nil {
		p.revealFailed = false
	}
}

func (a *desktop) refreshLogFiles() {
	p := a.logLibrary
	if p == nil || p.reading || a.ctx == nil {
		return
	}
	p.reading, p.revealFailed, p.err = true, false, ""
	request, fileType := a.logRequest, p.fileType
	go func() {
		entries, err := launcherlog.Recent(a.ctx, p.instance.GameDir)
		a.post(func() { a.applyLogFileList(p, entries, err, request, fileType) })
	}()
}

func (a *desktop) applyLogFileList(p *instanceLogView, entries []launcherlog.Entry, err error, request uint64, fileType string) {
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
	visible := p.visibleEntries()
	selectionUnchanged := request == a.logRequest && fileType == p.fileType
	a.clearHiddenLog(visible)
	if !selectionUnchanged {
		return
	}
	if a.logPath != "" {
		a.selectLog(a.logPath)
	} else if len(visible) > 0 {
		a.selectLog(visible[0].Path)
	}
}

func (a *desktop) logFilePicker(c *ui.Context) int {
	p := a.logLibrary
	count := 0
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		oldType := p.fileType
		field.Select(c.Key("log-file-type"), &p.fileType, field.SelectProps{Options: logFileTypes}).
			Label("日志文件类型").Width(136).Shrink(0)
		visible := p.visibleEntries()
		count = len(visible)
		if oldType != p.fileType {
			a.clearHiddenLog(visible)
			c.Invalidate()
		}
		options := make([]field.SelectOption, 0, count)
		for _, entry := range visible {
			label := entry.Modified.Local().Format("01-02 15:04") + " · " + logFileType(entry.Path) + " · " + filepath.Base(entry.Path)
			options = append(options, field.SelectOption{Value: entry.Path, Label: label})
		}
		path := a.logPath
		placeholder := "暂无日志，运行游戏后可刷新"
		switch {
		case count > 0 && path != "":
			placeholder = ""
		case count > 0:
			placeholder = "选择日志文件"
		case p.reading:
			placeholder = "正在读取日志列表…"
		case p.err != "" && len(p.entries) == 0:
			placeholder = "未找到可读取的日志"
		case len(p.entries) > 0:
			placeholder = "此类型暂无文件"
		}
		catalogPicker(c.Key(p.fileType), &path, options, "选择实例日志", placeholder, count == 0).Grow(1).MinWidth(0)
		if path != a.logPath {
			a.selectLog(path)
		}
		if button(c, "刷新日志列表", action.Default, p.reading) {
			a.refreshLogFiles()
		}
	})
	return count
}
