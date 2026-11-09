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
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func panelHeading(c *ui.Context, glyph *ui.SVG, title, detail string) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		leadIcon(c, glyph, 38)
		ui.Column(c).Gap(3).Grow(1).MinWidth(0).Children(func() {
			display.Text(c, title, display.TextProps{Strong: true, FontSize: 15})
			if detail != "" {
				muted(c, detail).FontSize(13)
			}
		})
	})
}

func (a *desktop) details(c *ui.Context, i launcher.Instance) {
	nav.Breadcrumb(c, nav.BreadcrumbProps{Items: []nav.BreadcrumbItem{
		{Title: "我的实例", OnClick: func() { a.inspected = 0 }},
		{Title: "实例概览"},
	}})
	heading(c, i.Name, "Minecraft "+i.Version+"  /  "+loaderName(i), func() {
		if button(c, "重命名", action.Default, a.busy) {
			a.edit(i)
		}
		label := "下载并启动"
		if a.installed[i.ID] {
			label = "启动游戏"
		}
		if button(c, label, action.Primary, a.busy || a.running != nil) {
			a.play(i, false)
		}
	})
	layout.ScrollArea(c, layout.ScrollAreaProps{}, func() {
		ui.Column(c).Gap(14).FillWidth().Children(func() {
			surface(c).Padding(18).Gap(14).Children(func() {
				panelHeading(c, lucide.FolderCog(), "实例配置", "存档与设置保存在这个实例的独立目录中。")
				layout.Divider(c, layout.DividerProps{})
				status, tone := a.instanceStatus(i)
				property(c, "游戏目录", i.GameDir)
				property(c, "Minecraft 版本", i.Version)
				property(c, "加载器", loaderName(i))
				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					muted(c, "游戏文件").Width(112).Shrink(0)
					display.Tag(c, status, display.TagProps{Color: tone, Round: true})
					if a.state.Selected == i.ID {
						display.Tag(c, "首页启动目标", display.TagProps{Color: display.TagGreen, Round: true})
					}
				})
				layout.Divider(c, layout.DividerProps{})
				ui.Row(c).Gap(8).Wrap().Children(func() {
					if button(c, "打开目录", action.Default, false) {
						a.openPath(i.GameDir, true)
					}
					if button(c, "下载 / 校验", action.Default, a.busy || a.running != nil) {
						a.play(i, true)
					}
					if a.state.Selected != i.ID && button(c, "设为启动目标", action.Text, a.busy) {
						a.choose(i.ID)
					}
				})
			})
			surface(c).Padding(15, 18).Children(func() {
				ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Gap(4).Grow(1).MinWidth(0).Children(func() {
						display.Text(c, "移除实例", display.TextProps{Strong: true})
						muted(c, "仅移除配置记录，保留游戏目录和存档。").FontSize(13)
					})
					if button(c, "移除配置…", action.Default, a.busy || a.runningID == i.ID) {
						a.form = editor{open: true, mode: "remove", target: i}
					}
				})
			})
		})
	}).Grow(1).MinHeight(0)
}

func property(c *ui.Context, label, value string) {
	ui.Row(c).AlignItems(ui.Start).Gap(12).Children(func() {
		muted(c, label).Width(112).Shrink(0)
		display.Text(c, value, display.TextProps{}).Grow(1).MinWidth(0).Selectable()
	})
}

func (a *desktop) network(c *ui.Context) {
	workspace(c, func() {
		heading(c, "联机", "查看账号状态和当前支持的游玩方式。")
		surface(c).Padding(20).Gap(15).Children(func() {
			panelHeading(c, lucide.WifiOff(), "当前使用离线账号", "可启动原版客户端，在本地游玩和管理存档。")
			feedback.Alert(c, "在线服务尚未接入", feedback.AlertProps{
				Type: feedback.Info, Description: "Microsoft 登录、正版联机和 Realms 暂不可用。",
			})
			ui.Row(c).Gap(8).Children(func() {
				if button(c, "返回主页", action.Primary, false) {
					a.page = pageHome
				}
				if button(c, "账号设置", action.Default, false) {
					a.page, a.settingsPage = pageSettings, 2
				}
			})
		})
	})
}

func settingsField(c *ui.Context, label string, value *string, placeholder string, glyph *ui.SVG, disabled bool, pickLabel string, pick func()) {
	ui.Column(c).Gap(6).Children(func() {
		display.Text(c, label, display.TextProps{Strong: true, FontSize: 13})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				field.Input(c, value, field.InputProps{Placeholder: placeholder, Prefix: glyph}).
					Label(label).Disabled(disabled)
			})
			if pick != nil && button(c, pickLabel, action.Default, disabled) {
				pick()
			}
		})
	})
}

var settingsTabs = []nav.TabItem{
	{Label: "常规", Icon: lucide.Settings()},
	{Label: "Java", Icon: lucide.Coffee()},
	{Label: "账号", Icon: lucide.UserRound()},
	{Label: "帮助", Icon: lucide.CircleHelp()},
}

func (a *desktop) settings(c *ui.Context) {
	workspace(c, func() {
		contextTabs(c, &a.settingsPage, settingsTabs)
		layout.ScrollArea(c.Key(a.settingsPage), layout.ScrollAreaProps{}, func() {
			ui.Column(c).Gap(16).FillWidth().MaxWidth(860).Children(func() {
				switch a.settingsPage {
				case 0:
					heading(c, "常规设置", "为新实例选择保存位置，并设置游戏的最大内存。")
					surface(c).Padding(19).Gap(14).Children(func() {
						panelHeading(c, lucide.HardDrive(), "存储与内存", "更改游戏目录不会移动已有实例。")
						layout.Divider(c, layout.DividerProps{})
						ui.Row(c).Gap(14).Children(func() {
							ui.Column(c).Grow(1).MinWidth(0).Children(func() {
								settingsField(c, "游戏目录", &a.draft.GameRoot, "选择绝对路径", lucide.FolderOpen(), a.busy || a.pickerBusy, "浏览…", a.pickGameRoot)
							})
							ui.Column(c).Width(136).Shrink(0).Children(func() {
								settingsField(c, "最大内存（MiB）", &a.memory, "4096", nil, a.busy || a.pickerBusy, "", nil)
							})
						})
						muted(c, "范围：1024–65536 MiB。请为系统和其他程序留出内存。").FontSize(12)
					})
				case 1:
					heading(c, "Java 运行时", "指定 Java，或由启动器根据游戏版本自动检测。")
					surface(c).Padding(19).Gap(14).Children(func() {
						panelHeading(c, lucide.Coffee(), "Java 可执行文件", "路径留空时使用自动检测。")
						layout.Divider(c, layout.DividerProps{})
						previous := a.draft.JavaPath
						settingsField(c, "Java 路径", &a.draft.JavaPath, "自动检测", lucide.FileCode(), a.busy || a.pickerBusy, "浏览…", a.pickJava)
						if a.draft.JavaPath != previous {
							a.javaInfo = ""
						}
						ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
							if button(c, "检测 / 验证 Java", action.Default, a.busy) {
								a.detectJava()
							}
							if a.draft.JavaPath != "" && button(c, "使用自动检测", action.Text, a.busy) {
								a.draft.JavaPath, a.javaInfo = "", ""
							}
						})
						if a.javaInfo != "" {
							feedback.Alert(c, a.javaInfo, feedback.AlertProps{Type: feedback.Info})
						}
					})
				case 2:
					heading(c, "离线账号", "名称决定本地玩家身份。保留同一名称可以继续使用该身份。")
					surface(c).Padding(19).Gap(14).Children(func() {
						panelHeading(c, lucide.UserRound(), "本地玩家", "玩家名使用 3–16 位字母、数字或下划线。")
						layout.Divider(c, layout.DividerProps{})
						ui.Column(c).MaxWidth(360).Children(func() {
							settingsField(c, "玩家名", &a.draft.Player, "Player", lucide.UserRound(), a.busy, "", nil)
						})
						feedback.Alert(c, "Microsoft 登录尚未接入", feedback.AlertProps{Type: feedback.Info})
					})
				case 3:
					a.help(c)
				}
			})
		}).Grow(1).MinHeight(0)
		if a.settingsPage != 3 {
			a.settingsFooter(c)
		}
	})
}

func (a *desktop) settingsFooter(c *ui.Context) {
	dirty := a.draft != a.state.Settings || a.memory != strconv.Itoa(a.state.Settings.Memory)
	layout.Divider(c, layout.DividerProps{})
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		status := "设置已保存"
		if dirty {
			status = "有未保存的更改"
		}
		muted(c, status)
		ui.Spacer(c).Grow(1)
		if dirty && button(c, "还原更改", action.Text, a.busy || a.pickerBusy) {
			a.draft, a.memory, a.javaInfo = a.state.Settings, strconv.Itoa(a.state.Settings.Memory), ""
		}
		if button(c, "应用设置", action.Primary, a.busy || a.pickerBusy || !dirty) {
			a.saveSettings()
		}
	})
}

func (a *desktop) help(c *ui.Context) {
	heading(c, "关于 LXMCL", "CAUCRAFT 的 Minecraft Java 版启动器。")
	surface(c).Padding(19).Gap(14).Children(func() {
		panelHeading(c, lucide.Compass(), "首次启动", "创建实例，准备与版本匹配的 Java，然后下载并启动。")
		feedback.Alert(c, "查看启动日志", feedback.AlertProps{
			Type: feedback.Info, Description: "如果游戏未能启动，请在「版本 → 任务」查看错误和日志。",
		})
		if button(c, "前往任务", action.Default, false) {
			a.openTasks()
		}
	})
	surface(c).Padding(19).Gap(12).Children(func() {
		property(c, "桌面界面", "MyGo + Lobe MyGo")
		property(c, "数据目录", a.store.Root)
		property(c, "快捷键", "F1 打开帮助 · Ctrl+K 搜索实例 · F2 在详情重命名 · Esc 返回实例列表")
	})
}

func (a *desktop) openTasks() {
	a.page, a.libraryPage, a.inspected = pageLibrary, 1, 0
}

func (a *desktop) tasks(c *ui.Context) {
	heading(c, "任务", "查看下载校验、游戏进程与历史日志。")
	layout.ScrollArea(c, layout.ScrollAreaProps{}, func() {
		ui.Column(c).Gap(12).FillWidth().Children(func() {
			if a.task.Stage != "" {
				surface(c).Padding(16).Gap(12).Children(func() {
					panelHeading(c, lucide.Download(), a.taskName, a.task.Stage)
					a.taskProgress(c)
					if a.cancelTask != nil && button(c, "取消任务", action.Default, false) {
						a.cancelTask()
					}
				})
			}
			if a.running != nil {
				surface(c).Padding(16).Gap(12).Children(func() {
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
					if a.lastLog != "" && button(c, "打开日志目录", action.Text, false) {
						a.openLogFolder()
					}
				})
				for idx, row := range a.history {
					a.taskRow(c.Key(idx), row)
				}
			} else if a.lastLog != "" && a.running == nil {
				surface(c).Padding(16).Gap(8).Children(func() {
					property(c, "最近日志", a.lastLog)
					if button(c, "查看日志", action.Default, false) {
						a.showLog(a.lastLog)
					}
				})
			} else if a.task.Stage == "" && a.running == nil {
				emptyPanel(c, lucide.Activity(), "暂无任务", "下载、校验或启动游戏后，进度和日志会出现在这里。")
			}
		})
	}).Grow(1).MinHeight(0)
}

func (a *desktop) taskRow(c *ui.Context, row taskRecord) {
	tone := display.TagGray
	switch row.Status {
	case "已失败":
		tone = display.TagError
	case "已完成", "已退出":
		tone = display.TagSuccess
	case "已启动":
		tone = display.TagProcessing
	}
	surface(c).Padding(13, 15).Gap(9).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			leadIcon(c, lucide.Activity(), 32)
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				display.Text(c, row.Name, display.TextProps{Strong: true, Rows: 1}).Ellipsis("…")
				muted(c, row.Action+" · "+row.Version+" · "+row.At.Local().Format("01-02 15:04")).FontSize(12)
			})
			display.Tag(c, row.Status, display.TagProps{Color: tone, Round: true})
			if row.Log != "" && button(c, "查看日志", action.Text, false) {
				a.showLog(row.Log)
			}
		})
		if row.Detail != "" {
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
	ui.Row(c).Padding(8, 18).Gap(13).AlignItems(ui.Center).Background(tk.BgContainer).Children(func() {
		leadIcon(c, lucide.Activity(), 31)
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
