package main

import (
	"fmt"

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
	a.logRequest++
	request := a.logRequest
	a.logPath, a.logText, a.logOpen = path, "正在读取游戏日志…", true
	go func() {
		log, err := launcherlog.ReadGame(path)
		a.post(func() {
			if a.logRequest != request {
				return
			}
			if err != nil {
				a.logText = fmt.Sprintf("无法读取日志：%s", err)
			} else {
				a.logText = log
			}
		})
	}()
}

func (a *desktop) openLog(path string) {
	if launcherlog.IsGame(path) {
		a.openPath(path, false)
	}
}
