package main

import (
	"fmt"
	"strconv"
	"strings"

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

const (
	pageHome = iota
	pageLibrary
	pageNetwork
	pageSettings
)

var mainTabs = []nav.TabItem{
	{Label: "主页", Icon: lucide.Gamepad2()},
	{Label: "版本", Icon: lucide.Package()},
	{Label: "联机", Icon: lucide.Server()},
	{Label: "设置", Icon: lucide.Settings()},
}

func button(c *ui.Context, label string, variant action.ButtonVariant, disabled bool) bool {
	return action.Button(c, label, action.ButtonProps{Variant: variant}).Disabled(disabled).Clicked()
}

func icon(c *ui.Context, svg *ui.SVG, label string, disabled bool) bool {
	return action.ActionIcon(c, svg, action.ActionIconProps{Title: label}).Disabled(disabled).Clicked()
}

func muted(c *ui.Context, label string) ui.Element {
	return display.Text(c, label, display.TextProps{Type: display.TextSecondary})
}

func heading(c *ui.Context, title, sub string, controls ...func()) {
	ui.Row(c).Gap(16).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Column(c).Gap(4).Grow(1).MinWidth(0).Children(func() {
			display.Text(c, title, display.TextProps{Heading: 2})
			muted(c, sub)
		})
		if len(controls) != 0 {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(controls[0])
		}
	})
}

func (a *desktop) view(c *ui.Context) {
	theme.Apply(c)
	tk := theme.TokensOf(c)
	if a.ready && !a.form.open {
		if a.page == pageLibrary && a.inspected != 0 && !a.busy && c.Shortcut(0, ui.KeyF2) {
			if i, ok := a.state.Find(a.inspected); ok {
				a.edit(i)
			}
		}
		if c.Shortcut(0, ui.KeyF1) {
			a.page, a.settingsPage = pageSettings, 3
		}
	}
	ui.Column(c).Fill().Background(tk.BgLayout).Children(func() {
		a.topBar(c)
		switch {
		case a.loadErr != "":
			ui.Column(c).Grow(1).Padding(28).Children(func() {
				feedback.Alert(c, "无法读取本机配置", feedback.AlertProps{Type: feedback.Error, Description: a.loadErr})
			})
		case !a.ready:
			ui.Column(c).Grow(1).Center().Children(func() { muted(c, "正在读取启动器设置…") })
		default:
			switch a.page {
			case pageHome:
				a.home(c)
			case pageLibrary:
				a.library(c)
			case pageNetwork:
				a.network(c)
			case pageSettings:
				a.settings(c)
			}
		}
		if a.task.Stage != "" {
			a.progress(c)
		} else if a.notice != "" {
			ui.Row(c).Padding(8, 18).Gap(8).AlignItems(ui.Center).Background(tk.BgContainer).Children(func() {
				tone := feedback.Info
				if a.noticeError {
					tone = feedback.Error
				}
				feedback.Alert(c, a.notice, feedback.AlertProps{Type: tone, Variant: feedback.Plain}).Grow(1).MinWidth(0)
				if icon(c, lucide.X(), "关闭通知", false) {
					a.notice = ""
				}
			})
		}
		a.editor(c)
	})
}

func (a *desktop) topBar(c *ui.Context) {
	tk, bar := theme.TokensOf(c), c.TitleBar()
	ui.Row(c).Height(max(bar.Height, 54)).Padding(0, bar.Right+14, 0, bar.Left+18).
		Gap(18).AlignItems(ui.Center).Background(tk.BgContainer).Shrink(0).Children(func() {
		ui.Row(c).Height(52).Gap(9).AlignItems(ui.Center).DragWindow().Children(func() {
			ui.Image(c, a.logo).Size(29, 29).Fit(ui.Contain)
			display.Text(c, "LXMCL", display.TextProps{Strong: true, FontSize: 16})
		})
		nav.Tabs(c, &a.page, nav.TabsProps{Items: mainTabs, Variant: nav.TabsRounded}, nil).AlignSelf(ui.Center)
		ui.Row(c).Grow(1).Height(52).DragWindow()
		name := "离线账号"
		if a.state.Settings.Player != "" {
			name = a.state.Settings.Player
		}
		ui.Row(c).Gap(3).AlignItems(ui.Center).Children(func() {
			if icon(c, lucide.UserRound(), "离线账号", !a.ready) || button(c, name, action.Text, !a.ready) {
				a.page, a.settingsPage = pageSettings, 2
			}
		})
	})
}

func (a *desktop) home(c *ui.Context) {
	ui.Box(c).Grow(1).MinHeight(0).Children(func() {
		ui.Image(c, a.hero).Absolute().Top(0).Left(0).Right(0).Bottom(0).Fit(ui.Cover)
		layout.Block(c, layout.BlockProps{Variant: layout.BlockOutlined, Shadow: true}).
			Absolute().Bottom(26).Right(26).Width(345).Padding(17).Gap(12).Children(func() {
			i, ok := a.state.Find(a.state.Selected)
			if !ok {
				muted(c, "尚无游戏实例")
				if button(c, "创建实例", action.Primary, a.busy) {
					a.create()
				}
				return
			}
			selection := strconv.Itoa(i.ID)
			options := make([]field.SelectOption, 0, len(a.state.Instances))
			for _, item := range a.state.Instances {
				options = append(options, field.SelectOption{Value: strconv.Itoa(item.ID), Label: item.Name + " · " + item.Version})
			}
			field.Select(c, &selection, field.SelectProps{Options: options, Disabled: a.busy, Size: theme.Large})
			if selection != strconv.Itoa(i.ID) {
				id, _ := strconv.Atoi(selection)
				a.choose(id)
			}
			label := "下载并启动"
			if a.installed[i.ID] {
				label = "启动游戏"
			}
			if a.running != nil {
				label = "游戏运行中"
			} else if a.task.Stage != "" {
				label = a.task.Stage
			}
			if action.Button(c, label, action.ButtonProps{Variant: action.Primary, Size: theme.Large}).
				Height(47).Disabled(a.busy || a.running != nil).Clicked() {
				a.play(i, false)
			}
			ui.Row(c).Justify(ui.Center).Gap(18).Children(func() {
				if icon(c, lucide.FolderOpen(), "打开游戏目录", false) {
					a.openPath(i.GameDir, true)
				}
				if icon(c, lucide.SlidersHorizontal(), "实例设置", false) {
					a.inspect(i)
				}
			})
		})
	})
}

func contextTabs(c *ui.Context, selected *int, labels ...string) {
	items := make([]nav.TabItem, len(labels))
	for i, label := range labels {
		items[i].Label = label
	}
	nav.Tabs(c, selected, nav.TabsProps{Items: items, Variant: nav.TabsSquare}, nil)
}

func workspace(c *ui.Context, content func()) {
	ui.Column(c).Grow(1).MinHeight(0).Padding(22, 28).Gap(18).Children(content)
}

func (a *desktop) library(c *ui.Context) {
	workspace(c, func() {
		if i, ok := a.state.Find(a.inspected); ok {
			a.details(c, i)
			return
		}
		contextTabs(c, &a.libraryPage, "我的实例", "任务")
		if a.libraryPage == 1 {
			a.tasks(c)
			return
		}
		heading(c, "我的实例", "管理独立的 Minecraft Java 版游戏目录。", func() {
			if button(c, "＋ 创建实例", action.Primary, a.busy) {
				a.create()
			}
		})
		ui.Column(c).MaxWidth(360).Children(func() {
			field.Input(c, &a.query, field.InputProps{Placeholder: "按名称或版本搜索", Prefix: lucide.Search()}).Label("搜索实例")
		})
		layout.ScrollArea(c, layout.ScrollAreaProps{State: &a.scroll}, func() {
			ui.Column(c).Gap(8).FillWidth().Children(func() {
				query, shown := strings.ToLower(strings.TrimSpace(a.query)), 0
				for _, i := range a.state.Instances {
					if strings.Contains(strings.ToLower(i.Name+" "+i.Version), query) {
						a.instanceRow(c, i)
						shown++
					}
				}
				if shown == 0 {
					display.Empty(c, display.EmptyProps{Icon: lucide.Package(), Title: "没有实例",
						Description: "创建一个原版实例，或者换个搜索条件。"})
				}
			})
		}).Grow(1).MinHeight(0)
	})
}

func (a *desktop) instanceRow(c *ui.Context, i launcher.Instance) {
	c = c.Key(i.ID)
	tk := theme.TokensOf(c)
	layout.Block(c, layout.BlockProps{Variant: layout.BlockOutlined}).Padding(8, 12).Children(func() {
		ui.Row(c).Height(47).Gap(12).AlignItems(ui.Center).Children(func() {
			row := ui.ButtonBase(c).Row().Grow(1).MinWidth(0).Gap(12).Height(44).AlignItems(ui.Center).
				Justify(ui.Start).Label(i.Name)
			row.Children(func() {
				ui.Icon(c, lucide.Package()).FontSize(22).TextColor(tk.TextTertiary)
				ui.Column(c).MinWidth(0).Children(func() {
					display.Text(c, i.Name, display.TextProps{Strong: true}).Ellipsis("…")
					muted(c, i.Version+" · "+loaderName(i)).Ellipsis("…")
				})
			})
			if row.DoubleClicked() || row.Shortcut(0, ui.KeyEnter) {
				a.inspect(i)
			} else if row.Clicked() {
				a.choose(i.ID)
			}
			if a.state.Selected == i.ID {
				display.Tag(c, "启动目标", display.TagProps{Color: display.TagGreen})
			}
			if a.installed[i.ID] {
				display.Tag(c, "已安装", display.TagProps{})
			}
			if icon(c, lucide.Pencil(), "重命名", a.busy) || (!a.form.open && !a.busy && row.Shortcut(0, ui.KeyF2)) {
				a.edit(i)
			}
			if icon(c, lucide.FolderOpen(), "打开目录", false) {
				a.openPath(i.GameDir, true)
			}
			if button(c, "管理", action.Default, false) {
				a.inspect(i)
			}
		})
	})
}

func loaderName(i launcher.Instance) string {
	if i.Loader == "" || strings.EqualFold(i.Loader, "vanilla") {
		return "原版"
	}
	return i.Loader
}

func (a *desktop) details(c *ui.Context, i launcher.Instance) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if button(c, "‹ 返回实例", action.Text, false) {
			a.inspected = 0
		}
		muted(c, "实例概览")
	})
	heading(c, i.Name, "Minecraft "+i.Version+" · "+loaderName(i), func() {
		if button(c, "重命名", action.Default, a.busy) {
			a.edit(i)
		}
		if button(c, "启动游戏", action.Primary, a.busy || a.running != nil) {
			a.play(i, false)
		}
	})
	layout.Block(c, layout.BlockProps{Variant: layout.BlockOutlined}).Padding(18).Gap(12).Children(func() {
		property(c, "实例目录", i.GameDir)
		property(c, "版本", i.Version)
		property(c, "加载器", loaderName(i))
		status := "未安装"
		if a.installed[i.ID] {
			status = "已校验安装"
		}
		if a.runningID == i.ID {
			status = "游戏运行中"
		}
		property(c, "状态", status)
		ui.Row(c).Gap(8).Children(func() {
			if button(c, "打开目录", action.Default, false) {
				a.openPath(i.GameDir, true)
			}
			if button(c, "下载 / 校验", action.Default, a.busy || a.running != nil) {
				a.play(i, true)
			}
			if button(c, "设为启动目标", action.Text, a.busy || a.state.Selected == i.ID) {
				a.choose(i.ID)
			}
		})
	})
	if button(c, "移除实例配置…", action.Text, a.busy || a.runningID == i.ID) {
		a.form = editor{open: true, mode: "remove", target: i}
	}
}

func property(c *ui.Context, label, value string) {
	ui.Row(c).AlignItems(ui.Start).Gap(12).Children(func() {
		muted(c, label).Width(105).Shrink(0)
		display.Text(c, value, display.TextProps{}).Grow(1).MinWidth(0)
	})
}

func (a *desktop) network(c *ui.Context) {
	workspace(c, func() {
		heading(c, "联机", "多人服务尚未接入。")
		display.Empty(c, display.EmptyProps{Icon: lucide.Server(), Title: "当前使用离线账号",
			Description: "可以启动原版客户端进行本地游玩。正版账号、Realms 和需要正版验证的服务器暂不支持。",
			Action: func() {
				if button(c, "账号设置", action.Default, false) {
					a.page, a.settingsPage = pageSettings, 2
				}
			}})
	})
}

func labeled(c *ui.Context, label string, value *string, placeholder string, disabled bool) {
	ui.Column(c).Gap(5).Children(func() {
		muted(c, label)
		field.Input(c, value, field.InputProps{Placeholder: placeholder}).Label(label).Disabled(disabled)
	})
}

func (a *desktop) settings(c *ui.Context) {
	workspace(c, func() {
		contextTabs(c, &a.settingsPage, "常规", "Java", "账号", "关于 / 帮助")
		layout.ScrollArea(c, layout.ScrollAreaProps{}, func() {
			ui.Column(c).Gap(16).MaxWidth(750).FillWidth().Children(func() {
				switch a.settingsPage {
				case 0:
					heading(c, "常规设置", "配置新实例的保存位置和游戏进程的最大内存。")
					labeled(c, "游戏目录", &a.draft.GameRoot, "绝对路径", a.busy)
					labeled(c, "最大内存（MiB）", &a.memory, "4096", a.busy)
					muted(c, "已创建实例保留原目录。")
				case 1:
					heading(c, "Java 运行时", "留空时会按游戏版本自动查找兼容的本机 Java。")
					labeled(c, "Java 可执行文件", &a.draft.JavaPath, "例如 C:\\Program Files\\Java\\bin\\java.exe", a.busy)
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						if button(c, "检测 / 验证 Java", action.Default, a.busy) {
							a.detectJava()
						}
						if button(c, "使用自动检测", action.Text, a.busy) {
							a.draft.JavaPath, a.javaInfo = "", ""
						}
						muted(c, a.javaInfo)
					})
				case 2:
					heading(c, "离线账号", "名称用于本地游玩；保持同一名称可保留对应的玩家身份。")
					labeled(c, "玩家名", &a.draft.Player, "3–16 位字母、数字或下划线", a.busy)
					feedback.Alert(c, "Microsoft 登录尚未接入", feedback.AlertProps{Type: feedback.Info})
				case 3:
					heading(c, "关于 LXMCL", "CAUCRAFT 的 Minecraft Java 版启动器。")
					property(c, "桌面界面", "MyGo + Lobe MyGo")
					property(c, "数据目录", a.store.Root)
					property(c, "快捷键", "F2 重命名 · F1 打开帮助")
					feedback.Alert(c, "首次启动", feedback.AlertProps{Description: "创建原版实例 → 安装匹配的 Java → 下载并启动。下载中断后可继续，启动错误可在版本 → 任务里查看日志目录。"})
				}
				if a.settingsPage != 3 && button(c, "应用设置", action.Primary, a.busy) {
					a.saveSettings()
				}
			})
		}).Grow(1).MinHeight(0)
	})
}

func (a *desktop) tasks(c *ui.Context) {
	heading(c, "任务", "查看当前下载、游戏进程和最近日志。")
	if a.task.Stage != "" {
		feedback.Alert(c, a.taskName+" · "+a.task.Stage, feedback.AlertProps{})
	}
	if a.running != nil {
		feedback.Alert(c, fmt.Sprintf("游戏进程 PID %d", a.running.PID), feedback.AlertProps{Type: feedback.Success})
	}
	if a.lastLog != "" {
		property(c, "最近日志", a.lastLog)
		if button(c, "打开日志目录", action.Default, false) {
			a.openLogFolder()
		}
	}
	if a.task.Stage == "" && a.running == nil && a.lastLog == "" {
		display.Empty(c, display.EmptyProps{Icon: lucide.Download(), Title: "暂无任务"})
	}
}

func (a *desktop) progress(c *ui.Context) {
	tk := theme.TokensOf(c)
	ui.Row(c).Padding(9, 18).Gap(12).AlignItems(ui.Center).Background(tk.BgContainer).Children(func() {
		display.Text(c, a.taskName+" · "+a.task.Stage, display.TextProps{Strong: true})
		if a.task.Total > 0 {
			feedback.Progress(c, 100*float32(a.task.Done)/float32(a.task.Total),
				feedback.ProgressProps{ShowInfo: true}).Grow(1).MinWidth(50)
		} else {
			ui.Spacer(c).Grow(1)
		}
		if a.cancelTask != nil && button(c, "取消任务", action.Default, false) {
			a.cancelTask()
		}
	})
}

func (a *desktop) editor(c *ui.Context) {
	if a.form.mode == "" {
		return
	}
	titles := map[string]string{"create": "创建实例", "rename": "重命名实例", "remove": "移除实例配置"}
	okText := "保存"
	if a.form.mode == "remove" {
		okText = "移除配置"
	} else if a.form.mode == "create" {
		okText = "创建"
	}
	submit := false
	if feedback.Modal(c, &a.form.open, feedback.ModalProps{
		Title: titles[a.form.mode], OkText: okText, CancelText: "取消", Width: 465,
	}, func() {
		ui.Column(c).Gap(10).Children(func() {
			if a.form.mode == "remove" {
				muted(c, "移除「"+a.form.target.Name+"」的配置。游戏目录和存档会保留。")
				return
			}
			muted(c, "实例名称")
			in := field.Input(c, &a.form.name, field.InputProps{Placeholder: "例如：我的生存世界", Invalid: a.form.err != ""}).
				Label("实例名称").Disabled(a.busy)
			if a.form.focus {
				in.Focus()
				a.form.focus = false
			}
			submit = in.Submitted()
			if a.form.mode == "create" {
				labeled(c, "游戏版本", &a.form.version, "例如 1.21.1", a.busy)
				if len(a.releases) > 0 {
					opts := make([]field.SelectOption, 0, len(a.releases))
					for _, v := range a.releases {
						opts = append(opts, field.SelectOption{Value: v.ID, Label: v.ID})
					}
					field.Select(c, &a.form.version, field.SelectProps{Options: opts, Placeholder: "选择官方版本", Disabled: a.busy})
				} else if a.catalogBusy {
					muted(c, "正在读取官方版本列表…")
				} else if a.catalogErr != "" {
					muted(c, a.catalogErr)
				}
			}
			if a.form.err != "" {
				feedback.Alert(c, a.form.err, feedback.AlertProps{Type: feedback.Error})
			}
		})
	}) || submit {
		if a.form.open {
			a.submitEditor()
		}
	}
}
