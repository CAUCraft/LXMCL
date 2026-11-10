package main

import (
	"fmt"
	"strconv"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/nav"
	"github.com/lobehub/lobe-mygo/pkg/overlay"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func panelHeading(c *ui.Context, glyph *ui.SVG, title, detail string) {
	ui.Row(c).Gap(9).AlignItems(ui.Center).Children(func() {
		leadIcon(c, glyph, 28)
		ui.Column(c).Gap(3).Grow(1).MinWidth(0).Children(func() {
			display.Text(c, title, display.TextProps{Strong: true, FontSize: 15})
			if detail != "" {
				muted(c, detail).FontSize(13)
			}
		})
	})
}

func (a *desktop) details(c *ui.Context, i launcher.Instance) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if icon(c, lucide.ArrowLeft(), "返回实例列表", false) {
			a.inspected = 0
		}
		display.Text(c, i.Name, display.TextProps{Strong: true, FontSize: 19, Rows: 1}).Grow(1).MinWidth(0).Ellipsis("…")
		if a.state.Selected != i.ID && button(c, "设为启动目标", action.Default, a.busy) {
			a.choose(i.ID)
		}
		a.instanceMenu(c.Key(i.ID), i)
		ui.Column(c).Width(152).Shrink(0).Children(func() {
			d := a.launchDrafts[i.ID]
			if d != nil && d.revision == i.Revision && d.fields != launchFields(i.Launch) &&
				a.running == nil && a.task.Stage == "" {
				if launchButton(c, "应用并启动", lucide.Play(), a.busy || a.pickerBusy) {
					a.detailPage = detailLaunch
					a.saveInstanceLaunch(i, d, true)
				}
			} else if a.playButton(c, i, a.pickerBusy) {
				a.play(i, false)
			}
		})
	})
	contextTabs(c, &a.detailPage, instanceDetailTabs)
	switch a.detailPage {
	case detailLaunch:
		a.instanceLaunchSettings(c.Key(i.ID), i)
	case detailContent:
		a.instanceContent(c.Key(i.ID), i)
	case detailFiles:
		a.instanceFiles(c.Key(i.ID), i)
	default:
		a.instanceOverview(c.Key(i.ID), i)
	}
}

func (a *desktop) instanceOverview(c *ui.Context, i launcher.Instance) {
	layout.ScrollArea(c, layout.ScrollAreaProps{}, func() {
		surface(c).Padding(12, 16).Gap(8).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() { property(c, "版本", i.Version+" · "+loaderName(i)) })
				if (i.Vanilla() || i.Fabric()) &&
					button(c, "更换版本", action.Default, a.busy || a.runningID == i.ID) {
					a.changeVersion(i)
				}
				if (i.Vanilla() || i.Fabric()) && button(c, "加载器", action.Default, a.busy || a.runningID == i.ID) {
					a.configureLoader(i)
				}
			})
			layout.Divider(c, layout.DividerProps{})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				muted(c, "游戏文件").Width(112).Shrink(0)
				status, tone := a.instanceStatus(i)
				display.Tag(c, status, display.TagProps{Color: tone, Size: theme.Small})
				if a.state.Selected == i.ID {
					display.Tag(c, "启动目标", display.TagProps{Color: display.TagGreen, Size: theme.Small})
				}
				ui.Spacer(c).Grow(1)
				label := "下载文件"
				if a.installed[i.ID] {
					label = "校验文件"
				}
				if button(c, label, action.Default, a.busy || a.running != nil) {
					a.play(i, true)
				}
			})
			layout.Divider(c, layout.DividerProps{})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() { property(c, "游戏目录", i.GameDir) })
				action.CopyButton(c, i.GameDir, action.CopyButtonProps{}).Label("复制游戏目录")
				if icon(c, lucide.FolderOpen(), "打开游戏目录", false) {
					a.openPath(i.GameDir, true)
				}
			})
		})
	}).Grow(1).MinHeight(0)
}

func (a *desktop) instanceMenu(c *ui.Context, i launcher.Instance) {
	box := ui.Row(c).Shrink(0)
	open := ui.Local(box, "open", func() bool { return false })
	box.Children(func() {
		chosen := overlay.DropdownMenu(c, open, overlay.DropdownMenuProps{Items: []overlay.MenuItem{
			{Key: "rename", Label: "重命名", Icon: lucide.Pencil(), Hotkey: "f2", Disabled: a.busy},
			{Key: "duplicate", Label: "复制实例配置…", Icon: lucide.Copy(), Disabled: a.busy || (!i.Vanilla() && !i.Fabric())},
			{Divider: true},
			{Key: "remove", Label: "移除配置…", Icon: lucide.Trash2(), Danger: true, Disabled: a.busy || a.runningID == i.ID},
		}}, func(trigger ui.Element) {
			action.StyleActionIcon(c, trigger, lucide.Ellipsis(), action.ActionIconProps{Title: "更多实例操作", BlockSize: 32})
		})
		switch chosen {
		case "rename":
			a.edit(i)
		case "duplicate":
			a.duplicate(i)
		case "remove":
			a.form = editor{open: true, mode: "remove", target: i}
		}
	})
}

func property(c *ui.Context, label, value string) {
	ui.Row(c).AlignItems(ui.Start).Gap(12).Children(func() {
		muted(c, label).Width(112).Shrink(0)
		display.Text(c, value, display.TextProps{}).Grow(1).MinWidth(0).Selectable()
	})
}

func settingsRow(c *ui.Context, label string, control func()) {
	ui.Row(c).MinHeight(38).Gap(8).AlignItems(ui.Center).Children(func() {
		display.Text(c, label, display.TextProps{Strong: true, FontSize: 13}).Width(136).Shrink(0)
		ui.Row(c).Grow(1).MinWidth(0).Gap(6).AlignItems(ui.Center).Children(control)
	})
}

func settingsInput(c *ui.Context, label string, value *string, placeholder string, width float32, disabled bool) {
	row := ui.Column(c).Grow(1).MinWidth(0)
	if width > 0 {
		row.MaxWidth(width)
	}
	row.Children(func() {
		field.Input(c, value, field.InputProps{Placeholder: placeholder}).Label(label).Disabled(disabled)
	})
}

func (a *desktop) gameSettings(c *ui.Context, disabled bool) {
	surface(c).Padding(8, 14).Gap(5).Children(func() {
		settingsRow(c, "新实例目录", func() {
			settingsInput(c, "新实例目录", &a.draft.GameRoot, "选择目录", 0, disabled)
			if button(c, "浏览…", action.Default, disabled) {
				a.pickGameRoot()
			}
		})
		layout.Divider(c, layout.DividerProps{})
		settingsRow(c, "最大内存（MiB）", func() {
			settingsInput(c, "最大内存（MiB）", &a.memory, "1024–65536", 125, disabled)
			if button(c, "默认", action.Text, disabled || a.memory == "4096") {
				a.memory = "4096"
			}
		})
	})
}

func (a *desktop) javaSettings(c *ui.Context, disabled bool) {
	surface(c).Padding(8, 14).Gap(5).Children(func() {
		settingsRow(c, "Java 路径", func() {
			previous := a.draft.JavaPath
			settingsInput(c, "Java 路径", &a.draft.JavaPath, "自动选择", 0, disabled)
			if a.draft.JavaPath != previous {
				a.javaInfo = ""
			}
			if button(c, "浏览…", action.Default, disabled) {
				a.pickJava()
			}
			if button(c, "检测", action.Default, disabled) {
				a.detectJava()
			}
			if a.draft.JavaPath != "" && button(c, "自动", action.Text, disabled) {
				a.draft.JavaPath, a.javaInfo = "", ""
			}
		})
		if a.javaInfo != "" {
			muted(c, a.javaInfo).FontSize(12).Selectable()
		}
	})
}

var settingsTabs = []nav.TabItem{
	{Label: "游戏", Icon: lucide.Gamepad2()},
	{Label: "Java", Icon: lucide.Coffee()},
	{Label: "账号", Icon: lucide.UserRound()},
	{Label: "关于", Icon: lucide.CircleHelp()},
}

func (a *desktop) settings(c *ui.Context) {
	workspace(c, func() {
		splitWorkspace(c, 168, func() {
			muted(c, "设置").Padding(3, 5)
			items := make([]display.ListItem, 0, len(settingsTabs))
			for id, item := range settingsTabs {
				items = append(items, display.ListItem{Key: strconv.Itoa(id), Label: item.Label, Icon: item.Icon})
			}
			if key := display.List(c, display.ListProps{Items: items, Active: strconv.Itoa(a.settingsPage), Compact: true}); key != "" {
				a.settingsPage, _ = strconv.Atoi(key)
				c.Invalidate()
			}
			if a.settingsPage == 2 {
				layout.Divider(c, layout.DividerProps{})
				a.accountList(c)
			}
		}, func() {
			key := strconv.Itoa(a.settingsPage)
			if a.settingsPage == 2 {
				key += "/" + a.browsedAccount
			}
			detailPane(c.Key(key), func() {
				disabled := a.busy || a.pickerBusy
				layout.ScrollArea(c.Key(key), layout.ScrollAreaProps{}, func() {
					ui.Column(c).Gap(10).FillWidth().MaxWidth(860).Children(func() {
						switch a.settingsPage {
						case 0:
							heading(c, "游戏设置")
							a.gameSettings(c, disabled)
						case 1:
							heading(c, "Java 设置")
							a.javaSettings(c, disabled)
						case 2:
							if a.browsedAccount == "" {
								a.startAppearance()
								a.microsoftSettings(c, disabled || a.busy)
							} else {
								a.offlineAccount(c, disabled)
							}
						case 3:
							a.help(c)
						}
					})
				}).Grow(1).MinHeight(0)
				if a.settingsPage < 2 {
					a.settingsFooter(c)
				}
			})
		})
	})
}

func (a *desktop) settingsFooter(c *ui.Context) {
	dirty := a.draft != a.state.Settings || a.memory != strconv.Itoa(a.state.Settings.Memory)
	if !dirty {
		return
	}
	settingsActions(c, a.busy || a.pickerBusy, func() {
		a.draft, a.memory, a.javaInfo = a.state.Settings, strconv.Itoa(a.state.Settings.Memory), ""
	}, a.saveSettings)
}

func settingsActions(c *ui.Context, disabled bool, reset, apply func()) {
	layout.Divider(c, layout.DividerProps{})
	ui.Row(c).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() {
		if button(c, "还原更改", action.Text, disabled) {
			reset()
		}
		if button(c, "应用设置", action.Primary, disabled) {
			apply()
		}
	})
}

func (a *desktop) help(c *ui.Context) {
	heading(c, "关于 LXMCL")
	surface(c).Padding(8, 14).Gap(5).Children(func() {
		settingsRow(c, "许可证", func() {
			display.Tag(c, "GPLv3", display.TagProps{Color: display.TagGreen, Size: theme.Small})
			if button(c, "查看许可证", action.Default, false) {
				a.licensesOpen = true
			}
		})
		muted(c, "© 2026 CAUCRAFT 与 LXMCL 贡献者 · 可按 GPLv3 再分发 · 无担保").FontSize(12)
		layout.Divider(c, layout.DividerProps{})
		settingsRow(c, "数据目录", func() {
			muted(c, a.store.Root).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			if icon(c, lucide.FolderOpen(), "打开数据目录", a.store.Root == "") {
				a.openPath(a.store.Root, true)
			}
		})
		layout.Divider(c, layout.DividerProps{})
		settingsRow(c, "快捷键", func() {
			muted(c, "Ctrl+K 搜索 · Ctrl+J 任务 · F2 重命名 · Esc 返回 · F1 关于").FontSize(12)
		})
	})
}

func (a *desktop) openTasks() {
	a.page, a.libraryPage, a.inspected = pageLibrary, 1, 0
}

func (a *desktop) tasks(c *ui.Context) {
	ui.Column(c).Grow(1).Basis(0).MinHeight(0).Gap(8).Children(func() {
		if a.task.Stage != "" {
			surface(c).Padding(10, 14).Gap(6).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Children(func() {
						display.Text(c, a.taskName, display.TextProps{Strong: true, FontSize: 15})
						flightLoading(c, a.task.Stage)
					})
					if a.cancelTask != nil && button(c, "取消任务", action.Default, false) {
						a.cancelTask()
					}
				})
				a.taskProgress(c)
			})
		}
		if a.running != nil {
			surface(c).Padding(10, 14).Gap(6).Children(func() {
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
			surface(c).Padding(16).Gap(8).Children(func() {
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
			Description: row.Status + " · " + row.Action, Icon: lucide.Activity()})
		if key == a.browsedTask {
			selected = row
		}
	}
	w, _ := c.Size()
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
				surface(c).Padding(8, 14).Children(func() { a.taskRow(c, selected) })
			}).Grow(1).MinHeight(0)
		})
	})
}

func taskKey(row taskRecord) string {
	return strconv.FormatInt(row.At.UnixNano(), 10)
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
	ui.Column(c).Padding(5, 0).Gap(4).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			leadIcon(c, lucide.Activity(), 26)
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				display.Text(c, row.Name, display.TextProps{Strong: true, Rows: 1}).Ellipsis("…")
				detail := row.Action + " · " + row.Version + " · " + row.At.Local().Format("01-02 15:04")
				if row.Server != "" {
					detail += " · " + row.Server
				}
				muted(c, detail).FontSize(12)
			})
			display.Tag(c, row.Status, display.TagProps{Color: tone, Round: true})
			if row.Log != "" && button(c, "查看日志", action.Text, false) {
				a.showLog(row.Log)
			}
			if i, ok := row.retryTarget(a.state); ok {
				if button(c, "重试", action.Default, a.busy || a.running != nil) {
					a.playOnServer(i, row.Action == taskVerify, row.Server)
				}
			}
		})
		if row.Status == taskFailed && row.Detail != "" {
			muted(c, row.Detail).FontSize(13).Selectable()
		}
	})
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
			muted(c, a.task.Stage).FontSize(12).SingleLine().Ellipsis("…")
		})
		ui.Column(c).Grow(1).MinWidth(50).Children(func() { a.taskProgress(c) })
		if button(c, "查看任务", action.Text, false) {
			a.openTasks()
		}
		if a.cancelTask != nil && button(c, "取消任务", action.Default, false) {
			a.cancelTask()
		}
	})
}

func (a *desktop) logDialog(c *ui.Context) {
	feedback.Modal(c, &a.logOpen, feedback.ModalProps{Title: "游戏日志", NoFooter: true, Width: 760}, func() {
		ui.Column(c).Gap(12).Children(func() {
			muted(c, a.logPath).FontSize(12).Selectable()
			log := a.logText
			if log == "" {
				log = "没有可显示的日志内容。"
			}
			_, height := c.Size()
			layout.ScrollArea(c.Key(a.logPath), layout.ScrollAreaProps{}, func() {
				display.Text(c, log, display.TextProps{Code: true, FontSize: 12}).Padding(12).Selectable()
			}).Height(max(160, min(340, height-240))).Radius(8).Background(theme.TokensOf(c).FillTertiary)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() {
				action.CopyButton(c, a.logText, action.CopyButtonProps{}).Disabled(a.logText == "")
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
