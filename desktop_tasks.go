package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	launcherlog "github.com/CAUCraft/LXMCL/internal/log"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func (a *desktop) openTasks() {
	a.page, a.downloadPage = pageDownloads, downloadTasks
}

func (a *desktop) tasks(c *ui.Context) {
	ui.Column(c).Grow(1).Basis(0).MinHeight(0).Gap(8).Children(func() {
		if a.task.Stage != "" {
			sectionPane(c).Padding(10, 14).Gap(6).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Children(func() {
						display.Text(c, a.taskName, display.TextProps{Strong: true, FontSize: 15})
						flightLoading(c, a.taskStage())
					})
					a.cancelTaskButton(c)
				})
				a.taskProgress(c)
			})
		}
		if a.running != nil {
			sectionPane(c).Padding(10, 14).Gap(6).Children(func() {
				name := "Minecraft"
				if i, ok := a.state.Find(a.runningID); ok {
					name = i.Name
				}
				panelHeading(c, lucide.Gamepad2(), name, fmt.Sprintf("游戏进程运行中 · PID %d", a.running.PID))
				if a.lastLog != "" && button(c, "查看当前日志", action.Default, false) {
					a.showLog(a.lastLog)
				}
			})
		}
		if len(a.history) > 0 {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				display.Text(c, "历史记录", display.TextProps{Heading: 4})
				ui.Spacer(c).Grow(1)
				action.Segmented(c, &a.taskFilter, []string{"全部任务", "失败 / 取消"},
					action.SegmentedProps{Size: theme.Small}).Label("历史任务筛选")
				if a.lastLog != "" && button(c, "打开日志目录", action.Text, false) {
					a.openLogFolder()
				}
			})
			visible := a.history
			if a.taskFilter == 1 {
				visible = nil
				for _, row := range a.history {
					if row.Status == taskFailed || row.Status == taskCanceled {
						visible = append(visible, row)
					}
				}
			}
			if len(visible) == 0 {
				emptyPanel(c, lucide.CircleCheck(), "没有失败或取消的任务")
				return
			}
			a.taskHistoryPane(c, visible)
		} else if a.lastLog != "" && a.running == nil {
			sectionPane(c).Padding(16).Gap(8).Children(func() {
				property(c, "最近日志", a.lastLog)
				if button(c, "查看日志", action.Default, false) {
					a.showLog(a.lastLog)
				}
			})
		} else if a.task.Stage == "" && a.running == nil {
			emptyPanel(c, lucide.Activity(), "暂无任务")
		}
	})
}

func (a *desktop) taskHistoryPane(c *ui.Context, rows []taskRecord) {
	selected := rows[0]
	items := make([]display.ListItem, 0, len(rows))
	for _, row := range rows {
		key := taskKey(row)
		items = append(items, display.ListItem{Key: key, Label: row.Name,
			Description: row.Status + " · " + row.Action, Icon: taskIcon(row)})
		if key == a.browsedTask {
			selected = row
		}
	}
	w, _ := c.Size()
	if w < 1000 {
		options := make([]field.SelectOption, 0, len(rows))
		for _, row := range rows {
			options = append(options, field.SelectOption{Value: taskKey(row),
				Label: row.Name + " · " + row.Status + " · " + row.At.Local().Format("01-02 15:04")})
		}
		key := taskKey(selected)
		catalogPicker(c, &key, options, "选择历史任务", "", false).FillWidth()
		if key != taskKey(selected) {
			a.browsedTask = key
			for _, row := range rows {
				if taskKey(row) == key {
					selected = row
					break
				}
			}
		}
		layout.ScrollArea(c.Key(taskKey(selected)), layout.ScrollAreaProps{}, func() {
			sectionPane(c).Padding(8, 14).Children(func() { a.taskRow(c, selected) })
		}).Grow(1).MinHeight(0)
		return
	}
	splitWorkspace(c, listWidth(w), func() {
		layout.ScrollArea(c, layout.ScrollAreaProps{}, func() {
			if key := display.List(c, display.ListProps{Items: items, Active: taskKey(selected), Compact: true}); key != "" {
				a.browsedTask = key
				for _, row := range rows {
					if taskKey(row) == key {
						selected = row
						break
					}
				}
				c.Invalidate()
			}
		}).Grow(1).MinHeight(0)
	}, func() {
		detailPane(c.Key(taskKey(selected)), func() {
			layout.ScrollArea(c, layout.ScrollAreaProps{}, func() {
				sectionPane(c).Padding(8, 14).Children(func() { a.taskRow(c, selected) })
			}).Grow(1).MinHeight(0)
		})
	})
}

func taskKey(row taskRecord) string {
	return row.Key()
}

func taskIcon(row taskRecord) *ui.SVG {
	switch row.Status {
	case taskFailed:
		return lucide.CircleAlert()
	case taskCanceled:
		return lucide.CircleX()
	case taskCompleted, taskExited:
		return lucide.CircleCheck()
	case taskStarted:
		return lucide.Gamepad2()
	default:
		return lucide.Activity()
	}
}

func (a *desktop) taskRow(c *ui.Context, row taskRecord) {
	tone := display.TagGray
	switch row.Status {
	case taskFailed:
		tone = display.TagError
	case taskCompleted, taskExited:
		tone = display.TagSuccess
	case taskStarted:
		tone = display.TagProcessing
	}
	failed := row.Status == taskFailed || row.Status == taskCanceled
	target, retry := launcher.RetryTarget(row, a.state)
	folderRetry := failed && row.Folder != "" && row.Resource != nil &&
		row.Resource.Project != "" && row.Resource.Version != "" &&
		(row.Resource.Kind == string(launcher.Mods) || row.Resource.Kind == string(launcher.ResourcePacks) || row.Resource.Kind == string(launcher.ShaderPacks))
	ui.Column(c).Padding(5, 0).Gap(12).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			leadIcon(c, taskIcon(row), 26)
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				display.Text(c, row.Name, display.TextProps{Strong: true, Rows: 1}).Ellipsis("…")
				detail := row.Action + " · " + row.At.Local().Format("01-02 15:04")
				if row.Version != "" {
					detail = row.Version + " · " + detail
				}
				muted(c, detail).FontSize(12)
			})
			display.Tag(c, row.Status, display.TagProps{Color: tone, Round: true})
		})
		if row.Folder != "" {
			muted(c, row.Folder).FontSize(13).Selectable()
		}
		if row.Server != "" {
			muted(c, "服务器 · "+row.Server).FontSize(13).Selectable()
		}
		if row.Resource != nil && row.Resource.Title != "" {
			muted(c, "资源 · "+row.Resource.Title).FontSize(13).Selectable()
		}
		if row.Detail != "" {
			if row.Status == taskFailed {
				feedback.Alert(c, "任务未完成", feedback.AlertProps{Type: feedback.Error, Description: row.Detail})
			} else {
				muted(c, row.Detail).FontSize(13).Selectable()
			}
		}
		if failed && !retry && !folderRetry {
			reason := "此任务不支持直接重试，请从原入口重新发起。"
			if row.InstanceID != 0 {
				if _, exists := a.state.Find(row.InstanceID); !exists {
					reason = "原实例已移除，请在实例页重新选择。"
				} else if row.Action == taskLaunch || row.Action == taskVerify || row.Resource != nil {
					reason = "实例配置、登录账号或资源信息已变化，请检查后重新发起。"
				}
			}
			muted(c, reason).FontSize(12)
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if (retry || folderRetry) && button(c, "重试", action.Primary, a.busy || a.running != nil) {
				if folderRetry {
					a.saveResourceFile(row.Folder, *row.Resource, launcher.PackVersion{})
				} else if row.Resource != nil {
					a.downloadResource(target, *row.Resource, launcher.PackVersion{})
				} else {
					a.playOnServer(target, row.Action == taskVerify, row.Server)
				}
			}
			if i, exists := a.state.Find(row.InstanceID); exists && button(c, "查看实例", action.Default, false) {
				a.query, a.instanceFilter = "", 0
				a.inspect(i)
				if failed {
					a.detailPage = detailLaunch
				}
			}
			if row.Log != "" && button(c, "查看日志", action.Default, false) {
				a.showLog(row.Log)
			}
			if row.Folder != "" && button(c, "打开文件夹", action.Default, false) {
				a.openPath(row.Folder, false)
			}
			if failed && row.Detail != "" {
				action.CopyButton(c, row.Detail, action.CopyButtonProps{}).Label("复制错误详情")
			}
		})
	})
}

func (a *desktop) taskStage() string {
	if a.taskCanceling {
		return "正在取消，请稍候…"
	}
	return a.task.Stage
}

func (a *desktop) cancelTaskButton(c *ui.Context) {
	if a.cancelTask == nil {
		return
	}
	label := "取消任务"
	if a.taskCanceling {
		label = "正在取消…"
	}
	if button(c, label, action.Default, a.taskCanceling) {
		a.taskCanceling = true
		a.cancelTask()
	}
}

func (a *desktop) taskProgress(c *ui.Context) {
	if a.task.Total > 0 {
		ink, _ := brand(c)
		feedback.Progress(c, 100*float32(a.task.Done)/float32(a.task.Total), feedback.ProgressProps{
			ShowInfo: true, Color: ink, Format: func(float32) string {
				return fmt.Sprintf("%d / %d 个文件", a.task.Done, a.task.Total)
			},
		})
	}
}

func (a *desktop) progress(c *ui.Context) {
	tk := theme.TokensOf(c)
	layout.Divider(c, layout.DividerProps{})
	ui.Row(c).Padding(4, 14).Gap(10).AlignItems(ui.Center).Background(tk.BgContainer).Children(func() {
		flightIndicator(c)
		ui.Column(c).Gap(2).Width(210).MinWidth(120).Children(func() {
			display.Text(c, a.taskName, display.TextProps{Strong: true, Rows: 1}).Ellipsis("…")
			muted(c, a.taskStage()).FontSize(12).SingleLine().Ellipsis("…")
		})
		ui.Column(c).Grow(1).MinWidth(50).Children(func() { a.taskProgress(c) })
		if button(c, "查看任务", action.Text, false) {
			a.openTasks()
		}
		a.cancelTaskButton(c)
	})
}

func (a *desktop) logDialog(c *ui.Context) {
	title := "游戏日志"
	if a.logLibrary != nil {
		title = "日志与崩溃报告"
	}
	feedback.Modal(c, &a.logOpen, feedback.ModalProps{Title: title, NoFooter: true, Width: 760}, func() {
		ui.Column(c).Gap(10).Children(func() {
			if a.logLibrary != nil {
				a.logFilePicker(c)
			}
			muted(c, a.logPath).FontSize(12).SingleLine().Ellipsis("…").Selectable()
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				query, filter := a.logQuery, a.logFilter
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					field.SearchBar(c, &a.logQuery, field.SearchBarProps{Placeholder: "搜索日志与堆栈"}).Label("搜索游戏日志")
				})
				action.Segmented(c, &a.logFilter, []string{"全部日志", "警告 / 错误"}, action.SegmentedProps{Size: theme.Small}).Label("日志筛选")
				if query != a.logQuery || filter != a.logFilter {
					a.logScroll.Y = 0
					a.logExported = false
				}
				if icon(c, lucide.RefreshCw(), "刷新游戏日志", a.logReading) {
					a.reloadLog()
				}
			})
			problem, detail := a.logProblem()
			if detail != "" {
				feedback.Alert(c, problem, feedback.AlertProps{Type: feedback.Error, Description: detail})
			}
			filtered := launcherlog.Filter(a.logText, a.logQuery, a.logFilter == 1)
			preview := filtered
			if preview == "" {
				preview = "没有匹配的日志，请调整筛选或刷新。"
				if a.logReading {
					preview = "正在读取游戏日志…"
				} else if a.logLibrary != nil && a.logPath == "" {
					preview = "暂无日志或崩溃报告，运行游戏后可刷新列表。"
					if a.logLibrary.reading {
						preview = "正在查找实例日志…"
					}
				}
			}
			_, height := c.Size()
			reserved := float32(340)
			if a.logLibrary != nil {
				reserved += 42
			}
			if detail != "" {
				reserved += 90
			}
			layout.ScrollArea(c.Key(a.logPath), layout.ScrollAreaProps{State: &a.logScroll}, func() {
				display.Text(c, preview, display.TextProps{Code: true, FontSize: 12}).Padding(12).Selectable()
			}).Height(max(80, min(340, height-reserved))).Radius(8).Background(theme.TokensOf(c).FillTertiary)
			hint := "最近 64 KiB · 按日志条目筛选，保留堆栈"
			if a.logLibrary != nil {
				hint = "最近 30 个文件 · 预览末尾 64 KiB · 保留堆栈"
				if filepath.Base(filepath.Dir(a.logPath)) == "crash-reports" {
					hint = "最近 30 个文件 · 报告开头 64 KiB · 保留堆栈"
				}
			}
			muted(c, hint).FontSize(12)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() {
				lines := 0
				if filtered != "" {
					lines = strings.Count(strings.TrimSuffix(filtered, "\n"), "\n") + 1
				}
				caption := fmt.Sprintf("显示 %d 行", lines)
				if a.logReading {
					caption = "正在刷新…"
				} else if a.logExporting {
					caption = "正在导出…"
				} else if a.logExported {
					caption = "已导出预览"
				}
				muted(c, caption).FontSize(12).Grow(1)
				action.CopyButton(c, filtered, action.CopyButtonProps{}).Label("复制当前日志").Disabled(filtered == "" || a.logReading)
				if button(c, "导出预览…", action.Default, filtered == "" || a.logReading || a.logExporting || a.pickerBusy) {
					a.exportLog()
				}
				if button(c, "打开文件", action.Default, a.logPath == "") {
					a.openLog(a.logPath)
				}
				if button(c, "关闭", action.Primary, false) {
					a.logOpen = false
				}
			})
		})
	})
}

func (a *desktop) logProblem() (string, string) {
	// Resolve the preview failure first; secondary errors remain available after
	// it clears, without pushing recovery actions outside a small window.
	if a.logError != "" {
		return "无法读取日志", a.logError
	}
	if a.logExportErr != "" {
		return "无法导出日志", a.logExportErr
	}
	if a.logLibrary != nil {
		return "部分日志目录无法读取", a.logLibrary.err
	}
	return "", ""
}
