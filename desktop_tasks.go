package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
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

func failedOrCanceled(row taskRecord) bool {
	return row.Status == taskFailed || row.Status == taskCanceled
}

func (a *desktop) noticeTaskFiltered() bool {
	if a.taskFilter == 1 && a.noticeTaskKey != "" {
		for _, row := range a.history {
			if taskKey(row) == a.noticeTaskKey {
				return !failedOrCanceled(row)
			}
		}
	}
	return false
}

func (a *desktop) openNoticeTask() {
	if a.noticeTaskKey != "" {
		a.browsedTask = a.noticeTaskKey
		if a.noticeTaskFiltered() {
			a.taskFilter = 0
		}
	}
	a.openTasks()
}

func (a *desktop) tasks(c *ui.Context) {
	ui.Column(c).Grow(1).Basis(0).MinHeight(0).Gap(8).Children(func() {
		a.queuePanel(c)
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
		if a.historyReadErr != "" {
			if a.noticeHistory {
				a.notice, a.noticeHistory = "", false
			}
			title := "无法读取任务记录"
			if a.historyReloading {
				title = "正在重新读取任务记录…"
			} else if a.historyRetried {
				title = "重新读取任务记录失败"
			}
			feedback.Alert(c, title, feedback.AlertProps{Type: feedback.Error,
				Description: "修复 tasks.json 的内容或读取权限后，可在这里重新读取。"})
			muted(c, a.historyReadErr).FontSize(12).MaxLines(2).Ellipsis("…")
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if button(c, "重新读取任务记录", action.Primary, a.historyReloading) {
					a.reloadTaskHistory()
				}
				if button(c, "打开配置目录", action.Default, false) {
					a.openPath(a.store.Root, false)
				}
				action.CopyButton(c, a.historyReadErr, action.CopyButtonProps{}).Label("复制任务记录读取错误")
			})
			return
		}
		if len(a.history) > 0 {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				display.Text(c, "历史记录", display.TextProps{Heading: 4})
				ui.Spacer(c).Grow(1)
				action.Segmented(c, &a.taskFilter, []string{"全部任务", "失败 / 取消"},
					action.SegmentedProps{Size: theme.Small}).Label("历史任务筛选")
			})
			visible := a.history
			if a.taskFilter == 1 {
				visible = nil
				for _, row := range a.history {
					if failedOrCanceled(row) {
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
		} else if a.task.Stage == "" && a.running == nil && len(a.queue.pending) == 0 && a.queue.loadError == nil && a.queue.saveError == nil {
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
				Label: row.Name + " · " + row.Action + " · " + row.Status + " · " + row.At.Local().Format("01-02 15:04")})
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
	failed := failedOrCanceled(row)
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
			label := "资源"
			if row.Resource.Source == launcher.CurseForgeMods {
				label = "CurseForge"
			}
			muted(c, label+" · "+row.Resource.Title).FontSize(13).Selectable()
		}
		if row.Detail != "" {
			if row.Status == taskFailed {
				feedback.Alert(c, "任务未完成", feedback.AlertProps{Type: feedback.Error, Description: row.Detail})
			} else {
				muted(c, row.Detail).FontSize(13).Selectable()
			}
		}
		if failed && !retry && !folderRetry && row.Pack.Archive == "" {
			reason := "此任务不支持直接重试，请从原入口重新发起。"
			if row.InstanceID != 0 {
				if i, exists := a.state.Find(row.InstanceID); !exists {
					reason = "原实例已移除，请在实例页重新选择。"
				} else if row.Resource != nil && row.Resource.Project == "" && row.Resource.Kind != "" {
					if kind := launcher.ContentKind(row.Resource.Kind); kind.Supports(i) {
						reason = "查看实例中的" + contentLabel(kind) + "，检查已完成的项目后可继续操作。"
					} else {
						reason = "当前实例配置已不支持该资源类型，请检查实例的加载器。"
					}
				} else if row.Action == taskBackup {
					reason = "原实例配置已变化，请从游戏文件页重新备份。"
				} else if row.Action == taskLaunch || row.Action == taskVerify || row.Resource != nil {
					reason = "实例配置、登录账号或资源信息已变化，请检查后重新发起。"
				}
			}
			muted(c, reason).FontSize(12)
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if row.Pack.Archive != "" {
				if launcher.CanOpenPackBackup(row.Pack) && button(c, "打开回滚备份", action.Default, false) {
					a.openPackBackup(row)
				}
				if _, ok := launcher.PackRetryTarget(row, a.state); ok {
					if button(c, "重新预览更新", action.Primary, a.busy || a.pickerBusy || a.running != nil) {
						a.openPackRetry(row)
					}
				} else {
					muted(c, "原实例已变更，请重新选择更新目标。").FontSize(12)
				}
			}
			needsKey := row.Resource != nil && a.needsResourceKey(*row.Resource)
			if (retry || folderRetry) && needsKey && button(c, "设置 API Key…", action.Primary, false) {
				a.modpacks.keyOpen = true
			}
			if (retry || folderRetry) && !needsKey && button(c, "重试", action.Primary, a.busy || a.running != nil) {
				if folderRetry {
					a.saveResourceFile(row.Folder, *row.Resource, launcher.PackVersion{})
				} else if row.Action == taskBackup {
					a.backupSaves(target)
				} else if row.Resource != nil {
					a.downloadResource(target, *row.Resource, launcher.PackVersion{})
				} else {
					a.playOnServer(target, row.Action == taskVerify, row.Server)
				}
			}
			if i, exists := a.state.Find(row.InstanceID); exists && button(c, "查看实例", action.Default, false) {
				a.query, a.instanceFilter = "", 0
				a.inspect(i)
				switch {
				case row.Action == taskBackup || row.Action == "恢复存档" || strings.HasPrefix(row.Action, "恢复存档 · "):
					a.detailPage = detailFiles
				case row.Resource != nil:
					if kind := launcher.ContentKind(row.Resource.Kind); kind.Supports(i) {
						a.inspectContent(i, kind)
					}
				case strings.HasSuffix(row.Action, "资源"):
					a.detailPage = detailContent
				case failed && (row.Action == taskVerify || row.Action == taskLaunch):
					a.detailPage = detailLaunch
				}
			}
			if launcherlog.IsGame(row.Log) {
				if button(c, "查看日志", action.Default, false) {
					a.showLog(row.Log)
				}
				if button(c, "打开日志目录", action.Default, false) {
					a.openPath(filepath.Dir(row.Log), false)
				}
			}
			if row.Action == taskBackup && row.Status == taskCompleted && row.InstanceID > 0 && button(c, "打开备份目录", action.Default, false) {
				a.openPath(launcher.BackupsDir(a.store.Root, row.InstanceID), false)
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
		body := ui.Column(c).Gap(10)
		preview := ui.Local(body, "log-preview", func() logPreview { return logPreview{} })
		body.Children(func() {
			if a.logLibrary != nil {
				a.logFilePicker(c)
			}
			location := a.logPath
			if p := a.logLibrary; p != nil {
				if location == "" {
					location = p.instance.GameDir
				}
				location = p.instance.Name + " · " + location
			}
			muted(c, location).FontSize(12).SingleLine().Ellipsis("…").Selectable()
			var search ui.Element
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				query, filter := a.logQuery, a.logFilter
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					search = field.SearchBar(c, &a.logQuery, field.SearchBarProps{Placeholder: "搜索日志与堆栈", ShortKey: "mod+f"}).Label("搜索游戏日志")
				})
				action.Segmented(c, &a.logFilter, []string{"全部日志", "警告 / 错误"}, action.SegmentedProps{Size: theme.Small}).Label("日志筛选")
				if query != a.logQuery || filter != a.logFilter {
					a.logScroll.Y = 0
					a.logExported, a.logExportErr = false, ""
				}
				if icon(c, lucide.RefreshCw(), "刷新游戏日志", a.logReading || a.logPath == "") {
					a.reloadLog()
				}
			})
			problem, detail := a.logProblem()
			if detail != "" {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					layout.ScrollArea(c.Key(problem), layout.ScrollAreaProps{}, func() {
						feedback.Alert(c, problem, feedback.AlertProps{Type: feedback.Error, Description: detail})
					}).Grow(1).MinWidth(0).MaxHeight(90)
					action.CopyButton(c, detail, action.CopyButtonProps{}).Label("复制日志错误详情")
				})
			}
			preview.update(a.logText, a.logQuery, a.logFilter == 1)
			content := preview.text
			if content == "" {
				content = "没有匹配的日志。"
				if a.logReading {
					content = "正在读取游戏日志…"
				} else if a.logLibrary != nil && a.logPath == "" {
					content = "暂无日志或崩溃报告，运行游戏后可刷新列表。"
					if a.logLibrary.reading {
						content = "正在查找实例日志…"
					} else if a.logLibrary.err != "" {
						content = "检查实例目录和读取权限后，刷新日志列表重试。"
					}
				} else if a.logError != "" {
					content = "请刷新重试，或打开原文件查看。"
				} else if a.logText == "" && a.logPath != "" {
					content = i18n.Text("log.game_empty")
				}
			}
			_, height := c.Size()
			reserved := float32(340)
			if a.logDiagnosis.Cause != "" && detail == "" {
				layout.ScrollArea(c.Key("log-diagnosis"), layout.ScrollAreaProps{}, func() { a.diagnosisPanel(c) }).MaxHeight(92)
				reserved += 102
			}
			if a.logLibrary != nil {
				reserved += 42
			}
			if detail != "" {
				reserved += 90
			}
			layout.ScrollArea(c.Key(a.logPath), layout.ScrollAreaProps{State: &a.logScroll}, func() {
				if preview.text == "" && a.logText != "" && !a.logReading && (a.logQuery != "" || a.logFilter != 0) {
					ui.Row(c).Padding(12).Gap(8).AlignItems(ui.Center).Children(func() {
						display.Text(c, content, display.TextProps{})
						if button(c, "清除筛选", action.Default, false) {
							a.logQuery, a.logFilter, a.logScroll.Y = "", 0, 0
							search.Focus()
							a.logExported, a.logExportErr = false, ""
						}
					})
					return
				}
				display.Text(c, content, display.TextProps{Code: true, FontSize: 12}).Padding(12).Selectable()
			}).Height(max(80, min(340, height-reserved))).Radius(8).Background(theme.TokensOf(c).FillTertiary)
			hint := "最近 64 KiB · 按日志条目筛选，保留堆栈"
			if a.logLibrary != nil {
				hint = "最近 30 个文件 · 预览末尾 64 KiB · 保留堆栈"
				if filepath.Base(filepath.Dir(a.logPath)) == "crash-reports" {
					hint = "最近 30 个文件 · 报告开头 64 KiB · 按章节筛选"
				}
			}
			muted(c, hint).FontSize(12)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() {
				caption := fmt.Sprintf("显示 %d 行", preview.lines)
				if a.logReading {
					caption = "正在刷新…"
				} else if a.logExporting {
					caption = "正在导出…"
				} else if a.logExported {
					caption = "已导出预览"
				}
				muted(c, caption).FontSize(12).Grow(1)
				action.CopyButton(c, preview.text, action.CopyButtonProps{}).Label("复制当前日志").Disabled(preview.text == "" || a.logReading)
				if button(c, "导出预览…", action.Default, preview.text == "" || a.logReading || a.logExporting || a.pickerBusy) {
					a.exportLog(preview.text)
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
	if !a.logOpen && a.logCancel != nil {
		a.stopLogRead()
	}
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

func (a *desktop) diagnosisPanel(c *ui.Context) {
	d := a.logDiagnosis
	var title, label string
	page := detailLaunch
	switch d.Cause {
	case "java":
		title, label = "当前 Java 无法读取这个版本的类文件。请核对实例使用的 Java 版本。", "查看 Java 设置"
	case "heap":
		title, label = "Java 堆内存已耗尽。请核对实例的内存上限和模组需求。", "查看内存设置"
	case "dependency":
		title, label, page = "加载器报告缺少必需的模组。下方原文列出了依赖要求。", "管理实例模组", detailContent
	case "mod-version":
		title, label, page = "加载器报告模组版本不兼容。请核对游戏、加载器和模组版本。", "管理实例模组", detailContent
	default:
		return
	}
	feedback.Alert(c, title, feedback.AlertProps{Type: feedback.Warning})
	display.Text(c, d.Evidence, display.TextProps{Code: true, FontSize: 12, Rows: 2}).Selectable()
	for _, i := range a.state.Instances {
		if filepath.Join(i.GameDir, launcherlog.GameDirectory) != filepath.Dir(a.logPath) && (a.logLibrary == nil || a.logLibrary.instance.ID != i.ID) {
			continue
		}
		if page == detailContent && !launcher.Mods.Supports(i) {
			return
		}
		ui.Row(c).Children(func() {
			if button(c, label, action.Default, false) {
				a.logOpen = false
				a.inspect(i)
				a.detailPage = page
				if page == detailContent {
					a.content = contentView{id: i.ID, revision: i.Revision, tab: 1}
					a.refreshContent(i)
				}
			}
		})
		return
	}
}
