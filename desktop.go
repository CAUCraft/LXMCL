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

func brand(c *ui.Context) (ink, wash ui.Color) {
	if c.Theme().Dark {
		return ui.Hex("#9ed9b5"), ui.Hex("#233830")
	}
	return ui.Hex("#24684b"), ui.Hex("#e5f3e9")
}

func workspaceColor(c *ui.Context) ui.Color {
	if c.Theme().Dark {
		return ui.Hex("#161b19")
	}
	return ui.Hex("#f1f5f2")
}

func leadIcon(c *ui.Context, svg *ui.SVG, size float32) {
	ink, wash := brand(c)
	ui.Box(c).Size(size, size).Shrink(0).Radius(10).Center().Background(wash).Children(func() {
		ui.Icon(c, svg).FontSize(size * 0.47).TextColor(ink)
	})
}

func surface(c *ui.Context) ui.Element {
	return layout.Block(c, layout.BlockProps{Variant: layout.BlockOutlined}).Radius(12)
}

func heading(c *ui.Context, title, sub string, controls ...func()) {
	ui.Row(c).Gap(16).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Column(c).Gap(5).Grow(1).MinWidth(0).Children(func() {
			display.Text(c, title, display.TextProps{Heading: 2}).Ellipsis("…")
			muted(c, sub)
		})
		if len(controls) != 0 {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(controls[0])
		}
	})
}

func (a *desktop) view(c *ui.Context) {
	theme.Apply(c)
	if a.ready && !a.form.open && !a.logOpen {
		if a.page == pageLibrary && a.inspected != 0 {
			if c.Shortcut(0, ui.KeyEscape) {
				a.inspected = 0
			} else if !a.busy && c.Shortcut(0, ui.KeyF2) {
				if i, ok := a.state.Find(a.inspected); ok {
					a.edit(i)
				}
			}
		}
		if c.Shortcut(0, ui.KeyF1) {
			a.page, a.settingsPage = pageSettings, 3
		}
	}
	ui.Column(c).Fill().Background(workspaceColor(c)).Children(func() {
		a.topBar(c)
		switch {
		case a.loadErr != "":
			ui.Column(c).Grow(1).Padding(28).Gap(12).Children(func() {
				feedback.Alert(c, "无法读取本机配置", feedback.AlertProps{Type: feedback.Error, Description: a.loadErr})
				if button(c, "打开配置目录", action.Default, false) {
					a.openPath(a.store.Root, false)
				}
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
		if a.task.Stage != "" && !(a.page == pageLibrary && a.libraryPage == 1 && a.inspected == 0) {
			a.progress(c)
		} else if a.notice != "" && a.task.Stage == "" {
			a.noticeBar(c)
		}
		a.editor(c.Key("instance-editor"))
		a.logDialog(c.Key("game-log"))
	})
}

func (a *desktop) noticeBar(c *ui.Context) {
	tk := theme.TokensOf(c)
	layout.Divider(c, layout.DividerProps{})
	ui.Row(c).Padding(5, 18).Gap(8).AlignItems(ui.Center).Background(tk.BgContainer).Children(func() {
		tone := feedback.Info
		if a.noticeError {
			tone = feedback.Error
		}
		feedback.Alert(c, a.notice, feedback.AlertProps{Type: tone, Variant: feedback.Plain}).Grow(1).MinWidth(0)
		if a.noticeJava {
			if button(c, "Java 设置", action.Text, false) {
				a.page, a.settingsPage = pageSettings, 1
			}
		} else if a.noticeTask && button(c, "查看任务", action.Text, false) {
			a.openTasks()
		}
		if icon(c, lucide.X(), "关闭通知", false) {
			a.notice = ""
		}
	})
}

func (a *desktop) topBar(c *ui.Context) {
	tk, bar := theme.TokensOf(c), c.TitleBar()
	w, _ := c.Size()
	ui.Row(c).Height(max(bar.Height, 54)).Padding(0, bar.Right+14, 0, bar.Left+18).
		Gap(15).AlignItems(ui.Center).Background(tk.BgContainer).Shrink(0).Children(func() {
		ui.Row(c).Height(52).Gap(9).AlignItems(ui.Center).DragWindow().Children(func() {
			ui.Image(c, a.logo).Size(29, 29).Fit(ui.Contain)
			display.Text(c, "LXMCL", display.TextProps{Strong: true, FontSize: 16})
		})
		nav.Tabs(c, &a.page, nav.TabsProps{Items: mainTabs, Variant: nav.TabsRounded}, nil).AlignSelf(ui.Center)
		ui.Row(c).Grow(1).Height(52).DragWindow()
		name := a.state.Settings.Player
		if name == "" {
			name = "离线账号"
		}
		account := action.StyleButton(c, ui.ButtonBase(c).Label("离线账号"), "", action.ButtonProps{Variant: action.Text}).
			PaddingX(6).Gap(7).Disabled(!a.ready)
		account.Children(func() {
			ink, wash := brand(c)
			display.Avatar(c, name, display.AvatarProps{
				Size: 24, Shape: display.AvatarCircle, Fallback: display.AvatarText, Background: wash,
			})
			if w >= 920 {
				display.Text(c, name, display.TextProps{FontSize: 13, Rows: 1}).MaxWidth(110).Ellipsis("…").TextColor(ink)
			}
		})
		if account.Clicked() {
			a.page, a.settingsPage = pageSettings, 2
		}
	})
}

func (a *desktop) home(c *ui.Context) {
	w, h := c.Size()
	ui.Box(c).Grow(1).MinHeight(0).Clip().Background(ui.Hex("#19352e")).Children(func() {
		ui.Image(c, a.hero).Absolute().Top(0).Left(0).Right(0).Bottom(0).Fit(ui.Cover)
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).
			Gradient(ui.Transparent, ui.Hex("#122b20").Alpha(0.75), 180)
		ui.Column(c).Absolute().Bottom(34).Left(32).Width(min(510, w-410)).Gap(8).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, lucide.Trees()).FontSize(17).TextColor(ui.Hex("#cce9d2"))
				display.Text(c, "MINECRAFT  /  JAVA EDITION", display.TextProps{Strong: true, FontSize: 13, Rows: 1}).
					TextColor(ui.Hex("#e2f3e7"))
			})
			display.Text(c, "从河谷出发", display.TextProps{Strong: true, FontSize: min(52, w/22), Rows: 1}).
				TextColor(ui.Hex("#ffffff"))
			if h >= 600 {
				display.Text(c, "选择一个实例，继续你的世界。", display.TextProps{FontSize: 16}).
					TextColor(ui.Hex("#edf5ed"))
			}
		})
		card := surface(c).Absolute().Bottom(26).Right(26).Width(352).Padding(18).Gap(12).
			Shadow(0, 10, 28, 0, ui.Hex("#0e221a").Alpha(0.28))
		card.Children(func() { a.launchCard(c) })
	})
}

func (a *desktop) launchCard(c *ui.Context) {
	i, ok := a.state.Find(a.state.Selected)
	ui.Row(c).Gap(11).AlignItems(ui.Center).Children(func() {
		leadIcon(c, lucide.Gamepad2(), 40)
		ui.Column(c).Gap(3).Grow(1).MinWidth(0).Children(func() {
			display.Text(c, "启动游戏", display.TextProps{Heading: 4})
			muted(c, "选择本次启动的实例").FontSize(12)
		})
	})
	if len(a.state.Instances) == 0 {
		muted(c, "创建首个原版实例后，即可下载游戏并离线游玩。")
		if launchButton(c, "创建首个实例", lucide.Plus(), a.busy) {
			a.create()
		}
		return
	}
	selection := ""
	if ok {
		selection = strconv.Itoa(i.ID)
	}
	previous := selection
	options := make([]field.SelectOption, 0, len(a.state.Instances))
	for _, item := range a.state.Instances {
		options = append(options, field.SelectOption{Value: strconv.Itoa(item.ID), Label: item.Name + " · " + item.Version})
	}
	field.Select(c, &selection, field.SelectProps{Options: options, Placeholder: "选择游戏实例", Disabled: a.busy, Size: theme.Large}).
		Label("启动实例")
	if selection != previous {
		id, _ := strconv.Atoi(selection)
		a.choose(id)
	}
	label := "下载并启动"
	if a.installed[i.ID] {
		label = "进入游戏"
	}
	if a.running != nil {
		label = "游戏运行中"
	} else if a.task.Stage != "" {
		label = "任务进行中"
	}
	if launchButton(c, label, lucide.Play(), !ok || a.busy || a.running != nil) {
		a.play(i, false)
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if ok {
			status, tone := a.instanceStatus(i)
			display.Tag(c, status, display.TagProps{Color: tone, Size: theme.Small, Round: true})
		}
		ui.Spacer(c).Grow(1)
		if icon(c, lucide.FolderOpen(), "打开游戏目录", !ok) {
			a.openPath(i.GameDir, true)
		}
		if icon(c, lucide.SlidersHorizontal(), "实例设置", !ok) {
			a.inspect(i)
		}
	})
}

// Keep Lobe's button focus, hit testing and transition; tint the one primary hero action.
func launchButton(c *ui.Context, label string, glyph *ui.SVG, disabled bool) bool {
	b := action.StyleButton(c, ui.ButtonBase(c).Label(label), "", action.ButtonProps{Variant: action.Primary, Size: theme.Large}).
		Height(45).FillWidth().Disabled(disabled)
	if !disabled {
		bg := ui.Hex("#24684b")
		switch {
		case b.Pressed():
			bg = ui.Hex("#174936")
		case b.Hovered():
			bg = ui.Hex("#2c7656")
		}
		b.Background(bg).Border(1, bg).TextColor(ui.Hex("#ffffff"))
	}
	b.Children(func() {
		ui.Icon(c, glyph).FontSize(17)
		ui.Text(c, label).SingleLine()
	})
	return b.Clicked()
}

func contextTabs(c *ui.Context, selected *int, items []nav.TabItem) {
	nav.Tabs(c, selected, nav.TabsProps{Items: items, Variant: nav.TabsSquare}, nil)
	layout.Divider(c, layout.DividerProps{})
}

func workspace(c *ui.Context, content func()) {
	w, _ := c.Size()
	padding := float32(24)
	if w > 1060 {
		padding = max(34, (w-1160)/2)
	}
	ui.Column(c).Grow(1).MinHeight(0).Padding(20, padding).Gap(16).Children(content)
}

var libraryTabs = []nav.TabItem{
	{Label: "我的实例", Icon: lucide.Package()},
	{Label: "任务", Icon: lucide.Activity()},
}

func (a *desktop) library(c *ui.Context) {
	workspace(c, func() {
		contextTabs(c, &a.libraryPage, libraryTabs)
		if a.libraryPage == 1 {
			a.tasks(c)
			return
		}
		if i, ok := a.state.Find(a.inspected); ok {
			a.details(c, i)
			return
		}
		heading(c, "我的实例", fmt.Sprintf("%d 个实例 · 每个实例使用独立的存档目录", len(a.state.Instances)), func() {
			if button(c, "＋ 创建实例", action.Primary, a.busy) {
				a.create()
			}
		})
		if len(a.state.Instances) == 0 {
			emptyPanel(c, lucide.PackageOpen(), "还没有实例",
				"从一个原版 Minecraft Java 实例开始，游戏文件会在首次启动时下载。")
			return
		}
		ui.Column(c).MaxWidth(380).Children(func() {
			hotkey := "mod+k"
			if a.form.open || a.logOpen {
				hotkey = ""
			}
			field.SearchBar(c, &a.query, field.SearchBarProps{
				Placeholder: "搜索名称、版本或加载器", ShortKey: hotkey,
			}).Label("搜索实例")
		})
		layout.ScrollArea(c, layout.ScrollAreaProps{State: &a.scroll}, func() {
			query := strings.ToLower(strings.TrimSpace(a.query))
			var items []display.ListItem
			for _, i := range a.state.Instances {
				if !strings.Contains(strings.ToLower(i.Name+" "+i.Version+" "+loaderName(i)), query) {
					continue
				}
				status, _ := a.instanceStatus(i)
				if i.ID == a.state.Selected {
					status = "启动目标 · " + status
				}
				items = append(items, display.ListItem{
					Key: strconv.Itoa(i.ID), Label: i.Name,
					Description: i.Version + "  /  " + loaderName(i) + "  ·  " + status,
					Avatar:      func() { leadIcon(c, lucide.Package(), 34) },
					Extra:       "查看 ›",
				})
			}
			if len(items) == 0 {
				emptyPanel(c, lucide.SearchX(), "未找到匹配的实例", "试试其他名称、版本号或加载器。")
				return
			}
			surface(c).Padding(5).Children(func() {
				clicked := display.List(c, display.ListProps{
					Items: items, Active: strconv.Itoa(a.state.Selected),
				})
				if id, err := strconv.Atoi(clicked); err == nil {
					if i, ok := a.state.Find(id); ok {
						a.inspect(i)
					}
				}
			})
		}).Grow(1).MinHeight(0)
	})
}

func loaderName(i launcher.Instance) string {
	if i.Loader == "" || strings.EqualFold(i.Loader, "vanilla") {
		return "原版"
	}
	return i.Loader
}

func (a *desktop) instanceStatus(i launcher.Instance) (string, display.TagColor) {
	switch {
	case a.runningID == i.ID:
		return "游戏运行中", display.TagProcessing
	case a.installed[i.ID]:
		return "已校验", display.TagSuccess
	default:
		return "尚未安装", display.TagGray
	}
}

func emptyPanel(c *ui.Context, glyph *ui.SVG, title, detail string) {
	surface(c).Padding(23).Children(func() {
		display.Empty(c, display.EmptyProps{Icon: glyph, Title: title, Description: detail, Page: true})
	})
}

func (a *desktop) editor(c *ui.Context) {
	if a.form.mode == "" {
		return
	}
	title, okText := "重命名实例", "保存"
	switch a.form.mode {
	case "remove":
		title, okText = "移除实例配置", "移除配置"
	case "create":
		title, okText = "创建实例", "创建"
	}
	submit := false
	if feedback.Modal(c, &a.form.open, feedback.ModalProps{
		Title: title, OkText: okText, CancelText: "取消", Width: 480,
	}, func() {
		ui.Column(c).Gap(12).Children(func() {
			if a.form.mode == "remove" {
				feedback.Alert(c, "移除「"+a.form.target.Name+"」的配置",
					feedback.AlertProps{Type: feedback.Warning, Description: "游戏目录与存档会保留。"})
				return
			}
			if a.form.mode == "create" {
				muted(c, "原版 Java 版 · 游戏文件会在首次启动时下载。")
			}
			ui.Column(c).Gap(5).Children(func() {
				display.Text(c, "实例名称", display.TextProps{Strong: true})
				in := field.Input(c, &a.form.name, field.InputProps{Placeholder: "例如：我的生存世界"}).
					Label("实例名称").Disabled(a.busy)
				if a.form.focus {
					in.Focus()
					a.form.focus = false
				}
				submit = in.Submitted()
			})
			if a.form.mode == "create" {
				ui.Column(c).Gap(5).Children(func() {
					display.Text(c, "游戏版本", display.TextProps{Strong: true})
					opts := make([]field.AutoCompleteOption, 0, len(a.releases))
					for _, v := range a.releases {
						opts = append(opts, field.AutoCompleteOption{Value: v.ID})
					}
					field.AutoComplete(c, &a.form.version, field.AutoCompleteProps{
						Options: opts, Prefix: lucide.Package(), Placeholder: "例如 1.21.1",
					}).Label("游戏版本").Disabled(a.busy)
				})
				switch {
				case a.catalogBusy:
					muted(c, "正在读取官方版本列表…").FontSize(12)
				case a.catalogErr != "":
					feedback.Alert(c, a.catalogErr, feedback.AlertProps{Type: feedback.Warning})
					if button(c, "重试获取版本列表", action.Text, a.busy) {
						a.refreshCatalog()
					}
				default:
					muted(c, "输入版本号，或从建议列表中选择。").FontSize(12)
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
