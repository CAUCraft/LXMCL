package main

import (
	"context"

	launcherlog "github.com/CAUCraft/LXMCL/internal/log"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

type taskRecord = tasks.Record
type resourceTask = tasks.Resource

const (
	historyLimit  = tasks.Limit
	taskCompleted = tasks.Completed
	taskStarted   = tasks.Started
	taskExited    = tasks.Exited
	taskFailed    = tasks.Failed
	taskCanceled  = tasks.Canceled
	taskLaunch    = tasks.Launch
	taskVerify    = tasks.Verify
)

var (
	loadTaskHistory   = tasks.Load
	saveTaskHistory   = tasks.Save
	updateTaskHistory = tasks.Update
)

func (a *desktop) restoreTaskHistory(records []taskRecord) {
	a.history = records
	for _, r := range records {
		if launcherlog.IsGame(r.Log) {
			a.lastLog = r.Log
			break
		}
	}
}

func (a *desktop) saveTask(record taskRecord, render func()) {
	_, err := a.journal.Record(a.store.Root, record)
	a.post(func() {
		a.history = a.journal.Snapshot()
		render()
		if err != nil {
			// Preserve the game outcome and report the separate disk error.
			a.notice += " · 任务记录保存失败：" + err.Error()
			a.noticeError = true
		}
	})
}

func (a *desktop) showLog(path string) {
	if !launcherlog.IsGame(path) {
		return
	}
	a.logLibrary = nil
	a.selectLog(path)
}

func (a *desktop) selectLog(path string) {
	if !launcherlog.IsPreview(path) {
		return
	}
	if a.logPath != path || !a.logOpen {
		a.logQuery, a.logFilter, a.logScroll.Y, a.logText = "", 0, 0, ""
		a.logExported, a.logExportErr = false, ""
	}
	a.logPath, a.logOpen = path, true
	a.reloadLog()
}

func (a *desktop) stopLogRead() {
	if a.logCancel != nil {
		a.logCancel()
		a.logCancel = nil
	}
	a.logRequest++
	a.logReading = false
}

func (a *desktop) reloadLog() {
	a.stopLogRead()
	if a.ctx == nil || !launcherlog.IsPreview(a.logPath) {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.logCancel = cancel
	request := a.logRequest
	path := a.logPath
	a.logReading, a.logError = true, ""
	a.logExported, a.logExportErr = false, ""
	go func() {
		defer cancel()
		log, err := launcherlog.ReadPreview(ctx, path)
		a.post(func() {
			if a.logRequest != request {
				return
			}
			a.logReading, a.logCancel = false, nil
			if err != nil {
				a.logText, a.logError = "", err.Error()
			} else {
				a.logText = log
			}
		})
	}()
}

func (a *desktop) openLog(path string) {
	if launcherlog.IsPreview(path) {
		a.openPath(path, false)
	}
}
