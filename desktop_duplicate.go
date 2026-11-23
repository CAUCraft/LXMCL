package main

import (
	"context"
	"errors"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
)

func (a *desktop) copyInstanceContent(form editor, next launcher.State, target launcher.Instance) {
	if a.ctx == nil || !a.ready || a.busy || a.running != nil {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.form.open = false
	a.busy, a.taskName, a.cancelTask, a.taskCanceling = true, target.Name, cancel, false
	a.task, a.notice = launcher.Progress{Stage: "正在复制资源和游戏设置"}, ""
	store, game := a.store, a.game
	fromInstance, fromTab, fromQuery, fromFilter := a.inspected, a.detailPage, a.query, a.instanceFilter
	record := (taskRecord{InstanceID: form.target.ID, Revision: form.target.Revision, Name: form.target.Name,
		Version: form.target.Version, Action: "复制实例", At: time.Now()}).Submitted()
	a.writes.Go(func() {
		defer cancel()
		var last time.Time
		var err error
		if !form.copyContent {
			err = launcher.DuplicateFiles(ctx, form.target, target, form.files, store, next)
		} else {
			err = launcher.DuplicateWithContent(ctx, store, next, form.target, target, form.files.Screenshots, func(progress launcher.Progress) {
				if time.Since(last) >= 140*time.Millisecond {
					last = time.Now()
					a.post(func() { a.task = progress })
				}
			})
		}
		cached := false
		switch {
		case errors.Is(err, context.Canceled):
			record.Status, record.Detail = taskCanceled, "复制已取消，原实例保留"
		case err != nil:
			record.Status, record.Detail = taskFailed, err.Error()
		default:
			cached = game.Installed(target)
			record.InstanceID, record.Revision, record.Name = target.ID, target.Revision, target.Name
			record.Status, record.Detail = taskCompleted, "已创建实例副本，来自 "+form.target.Name
		}
		a.saveTask(record, func() {
			a.busy, a.cancelTask, a.task, a.taskCanceling = false, nil, launcher.Progress{}, false
			if err == nil {
				a.state = next
				if a.installed == nil {
					a.installed = make(map[int]bool)
				}
				a.installed[target.ID] = cached
				if a.page == pageLibrary && a.inspected == fromInstance && a.detailPage == fromTab && a.query == fromQuery && a.instanceFilter == fromFilter {
					a.query, a.instanceFilter = "", 0
					a.inspect(target)
				}
				a.message("已创建独立副本 · "+target.Name, nil)
			} else if record.Status == taskCanceled {
				a.message("实例复制已取消", nil)
			} else {
				a.message("实例复制失败 · "+target.Name, err)
			}
		})
	})
}
