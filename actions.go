package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo"
)

func (a *desktop) post(fn func()) {
	if a.ctx.Err() != nil {
		return
	}
	mygo.RunOnMain(func() {
		if a.ctx.Err() == nil {
			fn()
			a.window.Invalidate()
		}
	})
}

func (a *desktop) load() {
	go func() {
		s, err := a.store.Load()
		history, historyErr := loadTaskHistory(a.store.Root)
		installed := make(map[int]bool, len(s.Instances))
		for _, i := range s.Instances {
			installed[i.ID] = a.game.Installed(i)
		}
		a.post(func() {
			if err != nil {
				a.loadErr = err.Error()
				return
			}
			a.state, a.draft, a.installed, a.ready = s, s.Settings, installed, true
			a.memory = strconv.Itoa(s.Settings.Memory)
			a.restoreTaskHistory(history)
			if historyErr != nil {
				a.message("无法读取任务记录", historyErr)
			}
		})
	}()
}

func (a *desktop) message(s string, err error) {
	a.notice, a.noticeError = s, err != nil
	a.noticeJava, a.noticeTask = false, false
	if err != nil {
		a.notice = fmt.Sprintf("%s：%s", s, err)
	}
}

func (a *desktop) save(next launcher.State, done func(error)) {
	if !a.ready || a.busy {
		return
	}
	a.busy = true
	a.writes.Go(func() {
		err := a.store.Save(next)
		a.post(func() {
			a.busy = false
			if err == nil {
				a.state = next
			}
			done(err)
		})
	})
}

func (a *desktop) choose(id int) {
	if _, ok := a.state.Find(id); !ok || id == a.state.Selected {
		return
	}
	next := a.state.Clone()
	next.Selected = id
	a.save(next, func(err error) {
		if err != nil {
			a.message("无法保存启动实例", err)
		}
	})
}

func (a *desktop) create() {
	version := "1.21.1"
	if len(a.releases) > 0 {
		version = a.releases[0].ID
	}
	a.form = editor{open: true, mode: "create", version: version, focus: true}
	if len(a.releases) == 0 {
		a.refreshCatalog()
	}
}

func (a *desktop) edit(i launcher.Instance) {
	a.form = editor{open: true, mode: "rename", target: i, name: i.Name, focus: true}
}

func (a *desktop) inspect(i launcher.Instance) {
	a.page, a.libraryPage, a.inspected = pageLibrary, 0, i.ID
}

func (a *desktop) submitEditor() {
	if a.busy {
		return
	}
	form := a.form
	next := a.state.Clone()
	var err error
	switch form.mode {
	case "create":
		err = next.Create(form.name, strings.TrimSpace(form.version))
	case "rename":
		err = next.Rename(form.target.ID, form.target.Revision, form.name)
	case "remove":
		next.Remove(form.target.ID)
	}
	if err != nil {
		a.form.err = err.Error()
		return
	}
	a.form.err = ""
	a.save(next, func(err error) {
		same := a.form.mode == form.mode && a.form.target.ID == form.target.ID
		if err != nil {
			if same && a.form.open {
				a.form.err = err.Error()
			} else {
				a.message("保存实例失败", err)
			}
			return
		}
		visible := same && a.form.open
		if visible {
			a.form.open = false
		}
		switch form.mode {
		case "create":
			i, _ := a.state.Find(next.Selected)
			for _, other := range a.state.Instances {
				if other.Version == i.Version && a.installed[other.ID] {
					a.installed[i.ID] = true
					break
				}
			}
			if visible {
				a.inspect(i)
			} else {
				a.message("已创建实例 · "+i.Name, nil)
			}
		case "remove":
			if a.inspected == form.target.ID {
				a.inspected = 0
			}
		}
	})
}

func (a *desktop) refreshCatalog() {
	if a.catalogBusy {
		return
	}
	a.catalogBusy, a.catalogErr = true, ""
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
		defer cancel()
		versions, err := launcher.Releases(ctx)
		a.post(func() {
			a.catalogBusy = false
			if err != nil {
				a.catalogErr = "未能取得官方版本列表，可手动输入版本号。"
			} else {
				a.releases = versions
			}
		})
	}()
}

func (a *desktop) saveSettings() {
	next := a.state.Clone()
	next.Settings = a.draft
	next.Settings.GameRoot = strings.TrimSpace(a.draft.GameRoot)
	next.Settings.JavaPath = strings.TrimSpace(a.draft.JavaPath)
	next.Settings.Player = strings.TrimSpace(a.draft.Player)
	m, err := strconv.Atoi(strings.TrimSpace(a.memory))
	if err != nil {
		a.message("最大内存须填写整数 MiB", err)
		return
	}
	next.Settings.Memory = m
	if err := launcher.ValidateSettings(next.Settings); err != nil {
		a.message("无法保存设置", err)
		return
	}
	a.save(next, func(err error) {
		if err == nil {
			a.draft = next.Settings
			a.memory = strconv.Itoa(next.Settings.Memory)
			a.message("设置已保存", nil)
		} else {
			a.message("无法保存设置", err)
		}
	})
}

func (a *desktop) detectJava() {
	if a.busy {
		return
	}
	path := a.draft.JavaPath
	a.busy, a.javaInfo = true, "正在运行 Java 检测…"
	go func() {
		j, err := launcher.FindJava(a.ctx, path, 21)
		a.post(func() {
			a.busy = false
			if err != nil {
				a.javaInfo = ""
				a.message("Java 检测失败", err)
			} else {
				if strings.TrimSpace(path) != "" {
					a.draft.JavaPath = j.Path
				}
				a.javaInfo = fmt.Sprintf("Java %d · %s", j.Major, j.Path)
			}
		})
	}()
}

func (a *desktop) openPath(path string, create bool) {
	if !filepath.IsAbs(path) {
		a.message("无法打开路径", errors.New("需要绝对路径"))
		return
	}
	go func() {
		var err error
		if create {
			err = os.MkdirAll(path, 0755)
		}
		if err == nil {
			err = mygo.Shell.OpenPath(path)
		}
		if err != nil {
			a.post(func() { a.message("无法打开路径", err) })
		}
	}()
}

func (a *desktop) play(i launcher.Instance, installOnly bool) {
	if a.busy || a.running != nil {
		return
	}
	a.busy, a.taskName = true, i.Name
	a.notice = ""
	a.task = launcher.Progress{Stage: "检查游戏文件"}
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelTask = cancel
	settings, g := a.state.Settings, a.game
	record := taskRecord{Name: i.Name, Version: i.Version, Action: "启动游戏", At: time.Now()}
	if installOnly {
		record.Action = "下载 / 校验"
	}
	records := a.taskSnapshot()
	a.writes.Go(func() {
		defer cancel()
		var mu sync.Mutex
		var last time.Time
		var stage string
		report := func(p launcher.Progress) {
			mu.Lock()
			defer mu.Unlock()
			if p.Stage != stage || p.Done == p.Total || time.Since(last) > 140*time.Millisecond {
				last, stage = time.Now(), p.Stage
				a.post(func() {
					if a.task.Stage != p.Stage || a.task.Done <= p.Done {
						a.task = p
					}
				})
			}
		}
		needsInstall := installOnly || !g.Installed(i)
		var err error
		if needsInstall {
			err = g.Install(ctx, i, report)
		}
		var run launcher.Running
		if err == nil && !installOnly {
			report(launcher.Progress{Stage: "检查 Java 并启动游戏"})
			run, err = g.Start(ctx, i, settings)
			if !needsInstall && errors.Is(err, launcher.ErrRepairRequired) && ctx.Err() == nil {
				report(launcher.Progress{Stage: "修复游戏文件"})
				err = g.Install(ctx, i, report)
				if err == nil {
					report(launcher.Progress{Stage: "检查 Java 并启动游戏"})
					run, err = g.Start(ctx, i, settings)
				}
			}
		}
		installed := g.Installed(i)
		switch {
		case errors.Is(err, context.Canceled):
			record.Status, record.Detail = taskCanceled, "已校验的文件会在下次复用"
		case err != nil:
			record.Status, record.Detail = taskFailed, err.Error()
		case installOnly:
			record.Status, record.Detail = taskCompleted, "游戏文件已校验"
		default:
			record.Status, record.Log, record.PID = taskStarted, run.Log, run.PID
		}
		records = a.saveTask(records, record, func() {
			a.busy, a.cancelTask = false, nil
			a.task = launcher.Progress{}
			for _, inst := range a.state.Instances {
				if inst.Version == i.Version {
					a.installed[inst.ID] = installed && (inst.Loader == "" || strings.EqualFold(inst.Loader, "vanilla"))
				}
			}
			switch {
			case errors.Is(err, context.Canceled):
				a.message("任务已取消，可继续下载", nil)
			case err != nil:
				a.message("任务失败 · "+i.Name, err)
				a.noticeJava = errors.Is(err, launcher.ErrJavaRequired)
				a.noticeTask = true
			case installOnly:
				a.message("已校验游戏文件 · "+i.Name, nil)
			default:
				a.running, a.runningID, a.lastLog = &run, i.ID, run.Log
				a.message(fmt.Sprintf("已启动 Java 进程 · PID %d", run.PID), nil)
			}
		})
		if run.Done == nil {
			return
		}
		select {
		case err := <-run.Done:
			record.Status = taskExited
			if err != nil {
				record.Status, record.Detail = taskFailed, err.Error()
			}
			a.saveTask(records, record, func() {
				a.running, a.runningID = nil, 0
				a.message("游戏已退出 · "+i.Name, err)
				a.noticeTask = err != nil
			})
		case <-a.ctx.Done():
			// Closing the launcher does not stop an already launched game.
		}
	})
}

func (a *desktop) openLogFolder() {
	if isGameLog(a.lastLog) {
		a.openPath(filepath.Dir(a.lastLog), false)
	}
}
