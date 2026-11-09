package main

import (
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
	return action.ActionIcon(c, svg, action.ActionIconProps{Title: label, BlockSize: 28}).Disabled(disabled).Clicked()
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

func heading(c *ui.Context, title string, controls ...func()) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Shrink(0).Children(func() {
		display.Text(c, title, display.TextProps{Strong: true, FontSize: 19, Rows: 1}).
			Grow(1).MinWidth(0).Ellipsis("…")
		if len(controls) != 0 {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(controls[0])
		}
	})
}

const (
	searchHotkey = "mod+k"
	tasksHotkey  = "mod+j"
)

func (a *desktop) view(c *ui.Context) {
	theme.Apply(c)
	if a.ready && !a.form.open && !a.logOpen && !a.licensesOpen {
		if mods, key, ok := display.HotkeyBinding(searchHotkey, display.HotkeyAuto); ok && c.Shortcut(mods, key) {
			a.page, a.libraryPage, a.inspected = pageLibrary, 0, 0
			a.instanceFilter = 0
			a.focusSearch = len(a.state.Instances) > 0
		}
		if mods, key, ok := display.HotkeyBinding(tasksHotkey, display.HotkeyAuto); ok && c.Shortcut(mods, key) {
			a.openTasks()
		}
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
		a.licensesDialog(c.Key("third-party-licenses"))
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

const titleBarHeight = 42

func (a *desktop) topBar(c *ui.Context) {
	tk, bar := theme.TokensOf(c), c.TitleBar()
	w, _ := c.Size()
	height := max(bar.Height, titleBarHeight)
	ui.Box(c).Height(height).FillWidth().Background(tk.BgContainer).Shrink(0).DragWindow().Children(func() {
		ui.Row(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).
			Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			nav.Tabs(c, &a.page, nav.TabsProps{Items: mainTabs, Variant: nav.TabsRounded, Size: theme.Small}, nil).
				Label("主导航").AlignSelf(ui.Center).Shrink(0)
		})
		ui.Row(c).Absolute().Left(bar.Left + 14).Top(0).Bottom(0).
			Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Image(c, a.logo).Size(23, 23).Fit(ui.Contain)
			display.Text(c, "LXMCL", display.TextProps{Strong: true, FontSize: 14})
		})
		ui.Row(c).Absolute().Right(bar.Right + 10).Top(0).Bottom(0).Gap(6).
			AlignItems(ui.Center).Children(func() {
			ink, _ := brand(c)
			feedback.Badge(c, feedback.BadgeProps{Dot: a.task.Stage != "" || a.running != nil, Color: ink}, func() {
				if icon(c, lucide.ListTodo(), "任务中心", !a.ready) {
					a.openTasks()
				}
			}).AlignSelf(ui.Center)
			name := a.state.Settings.Player
			if name == "" {
				name = "离线账号"
			}
			account := action.StyleButton(c, ui.ButtonBase(c).Label("离线账号"), "",
				action.ButtonProps{Variant: action.Text, Size: theme.Small}).PaddingX(5).Gap(6).Shrink(0).Disabled(!a.ready)
			account.Children(func() {
				ink, wash := brand(c)
				display.Avatar(c, name, display.AvatarProps{
					Size: 20, Shape: display.AvatarCircle, Fallback: display.AvatarText, Background: wash,
				})
				if w >= 920 {
					display.Text(c, name, display.TextProps{FontSize: 12, Rows: 1}).MaxWidth(88).Ellipsis("…").TextColor(ink)
				}
			})
			if account.Clicked() {
				a.page, a.settingsPage = pageSettings, 2
			}
		})
	})
}

func (a *desktop) home(c *ui.Context) {
	ui.Box(c).Grow(1).MinHeight(0).Clip().Background(ui.Hex("#19352e")).Children(func() {
		ui.Image(c, a.hero).Absolute().Top(0).Left(0).Right(0).Bottom(0).Fit(ui.Cover)
		card := surface(c).Absolute().Bottom(22).Right(22).Width(324).Padding(12).Gap(8).
			Shadow(0, 10, 28, 0, ui.Hex("#0e221a").Alpha(0.28))
		card.Children(func() { a.launchCard(c) })
	})
}

func (a *desktop) launchCard(c *ui.Context) {
	i, ok := a.state.Find(a.state.Selected)
	if len(a.state.Instances) == 0 {
		panelHeading(c, lucide.PackageOpen(), "暂无实例", "")
		if launchButton(c, "新建实例", lucide.Plus(), a.busy) {
			a.create()
		}
		return
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		display.Text(c, "启动实例", display.TextProps{Strong: true, FontSize: 13}).Grow(1)
		if ok {
			status, tone := a.instanceStatus(i)
			display.Tag(c, status, display.TagProps{Color: tone, Size: theme.Small})
		}
	})
	selection := ""
	if ok {
		selection = strconv.Itoa(i.ID)
	}
	previous := selection
	options := make([]field.SelectOption, 0, len(a.state.Instances))
	for _, item := range a.state.Instances {
		options = append(options, field.SelectOption{Value: strconv.Itoa(item.ID), Label: item.Name + " · " + item.Version})
	}
	selectBox := field.Select(c, &selection, field.SelectProps{Options: options, Placeholder: "选择实例", Disabled: a.busy}).
		Label("启动实例")
	if a.busy {
		selectBox.TextColor(theme.TokensOf(c).TextSecondary)
	}
	if selection != previous {
		id, _ := strconv.Atoi(selection)
		a.choose(id)
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			if a.playButton(c, i, !ok) {
				a.play(i, false)
			}
		})
		if icon(c, lucide.FolderOpen(), "打开游戏目录", !ok) {
			a.openPath(i.GameDir, true)
		}
		if icon(c, lucide.SlidersHorizontal(), "实例设置", !ok) {
			a.inspect(i)
		}
	})
}

func (a *desktop) playButton(c *ui.Context, i launcher.Instance, disabled bool) bool {
	label, glyph := "下载并启动", lucide.Download()
	switch {
	case a.running != nil:
		label, glyph = "游戏运行中", lucide.Gamepad2()
	case a.task.Stage != "":
		label, glyph = "任务进行中", lucide.Activity()
	case a.installed[i.ID]:
		label, glyph = "启动游戏", lucide.Play()
	}
	return launchButton(c, label, glyph, disabled || a.busy || a.running != nil)
}

// Keep Lobe's button focus, hit testing and transition; tint the one primary hero action.
func launchButton(c *ui.Context, label string, glyph *ui.SVG, disabled bool) bool {
	b := action.StyleButton(c, ui.ButtonBase(c).Label(label), "", action.ButtonProps{Variant: action.Primary, Size: theme.Large}).
		Height(36).FillWidth().Disabled(disabled)
	if !disabled {
		bg := ui.Hex("#24684b")
		switch {
		case b.Pressed():
			bg = ui.Hex("#174936")
		case b.Hovered():
			bg = ui.Hex("#2c7656")
		}
		b.Background(bg).Border(1, bg).TextColor(ui.Hex("#ffffff"))
	} else {
		tk := theme.TokensOf(c)
		b.Background(tk.FillSecondary).Border(1, tk.BorderSecondary).TextColor(tk.TextSecondary)
	}
	b.Children(func() {
		ui.Icon(c, glyph).FontSize(17)
		ui.Text(c, label).SingleLine()
	})
	return b.Clicked()
}

func contextTabs(c *ui.Context, selected *int, items []nav.TabItem) {
	nav.Tabs(c, selected, nav.TabsProps{Items: items, Variant: nav.TabsSquare, Size: theme.Small}, nil)
	layout.Divider(c, layout.DividerProps{})
}

func workspace(c *ui.Context, content func()) {
	w, _ := c.Size()
	padding := float32(24)
	if w > 1060 {
		padding = max(34, (w-1160)/2)
	}
	ui.Column(c).Grow(1).MinHeight(0).Padding(12, padding).Gap(10).Children(content)
}

var libraryTabs = []nav.TabItem{
	{Label: "我的实例", Icon: lucide.Package()},
	{Label: "任务", Icon: lucide.Activity()},
}

func (a *desktop) library(c *ui.Context) {
	workspace(c, func() {
		if i, ok := a.state.Find(a.inspected); ok && a.libraryPage == 0 {
			a.details(c, i)
			return
		}
		contextTabs(c, &a.libraryPage, libraryTabs)
		if a.libraryPage == 1 {
			a.tasks(c)
			return
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if len(a.state.Instances) > 0 {
				ui.Column(c).Grow(1).MinWidth(0).MaxWidth(340).Children(func() {
					hotkey := searchHotkey
					if a.form.open || a.logOpen || a.licensesOpen {
						hotkey = ""
					}
					search := field.SearchBar(c, &a.query, field.SearchBarProps{
						Placeholder: "搜索实例", ShortKey: hotkey,
					}).Label("搜索实例")
					if a.focusSearch {
						search.Focus()
						a.focusSearch = false
					}
				})
			}
			ui.Spacer(c).Grow(1)
			if len(a.state.Instances) > 0 {
				action.Segmented(c, &a.instanceFilter, []string{"全部", "已缓存", "待下载"},
					action.SegmentedProps{Size: theme.Small}).Label("实例筛选")
			}
			if button(c, "新建实例", action.Primary, a.busy) {
				a.create()
			}
		})
		if len(a.state.Instances) == 0 {
			emptyPanel(c, lucide.PackageOpen(), "暂无实例")
			return
		}
		layout.ScrollArea(c, layout.ScrollAreaProps{State: &a.scroll}, func() {
			query := strings.ToLower(strings.TrimSpace(a.query))
			var items []display.ListItem
			for _, i := range a.state.Instances {
				if !strings.Contains(strings.ToLower(i.Name+" "+i.Version+" "+loaderName(i)), query) {
					continue
				}
				if a.instanceFilter == 1 && !a.installed[i.ID] || a.instanceFilter == 2 && a.installed[i.ID] {
					continue
				}
				status, _ := a.instanceStatus(i)
				if i.ID == a.state.Selected {
					status = "启动目标 · " + status
				}
				items = append(items, display.ListItem{
					Key: strconv.Itoa(i.ID), Label: i.Name,
					Description: i.Version + " · " + loaderName(i),
					Avatar:      func() { leadIcon(c, lucide.Package(), 28) },
					Extra:       status,
					Actions: func() {
						if icon(c, lucide.FolderOpen(), "打开 "+i.Name+" 的目录", false) {
							a.openPath(i.GameDir, true)
						}
					},
				})
			}
			if len(items) == 0 {
				surface(c).Padding(12).Children(func() {
					display.Empty(c, display.EmptyProps{Icon: lucide.SearchX(), Title: "未找到实例", Action: func() {
						if button(c, "清除筛选", action.Default, false) {
							a.query, a.instanceFilter = "", 0
						}
					}})
				})
				return
			}
			surface(c).Padding(5).Children(func() {
				clicked := display.List(c, display.ListProps{
					Items: items, Active: strconv.Itoa(a.state.Selected), Compact: true,
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

func emptyPanel(c *ui.Context, glyph *ui.SVG, title string) {
	surface(c).Padding(12).Children(func() {
		display.Empty(c, display.EmptyProps{Icon: glyph, Title: title})
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
	case "version":
		title, okText = "更换版本", "保存版本"
	}
	submit := false
	if feedback.Modal(c, &a.form.open, feedback.ModalProps{
		Title: title, OkText: okText, CancelText: "取消", Width: 480,
	}, func() {
		ui.Column(c).Gap(12).Children(func() {
			if a.form.mode == "remove" {
				feedback.Alert(c, "移除「"+a.form.target.Name+"」的配置",
					feedback.AlertProps{Type: feedback.Warning, Description: "游戏目录与存档会保留。"})
				if a.form.err != "" {
					feedback.Alert(c, a.form.err, feedback.AlertProps{Type: feedback.Error})
				}
				return
			}
			if a.form.mode == "version" {
				panelHeading(c, lucide.Package(), a.form.target.Name, "当前版本 "+a.form.target.Version)
				feedback.Alert(c, "更换版本会继续使用原游戏目录。降级前请备份存档。",
					feedback.AlertProps{Type: feedback.Warning})
			} else {
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
			}
			if a.form.mode == "create" || a.form.mode == "version" {
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
