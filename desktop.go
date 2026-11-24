package main

import (
	"strconv"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
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
	pageDownloads
	pageNetwork
	pageSettings
	pageAccount
)

var mainTabs = []nav.TabItem{
	{Label: "主页", Icon: lucide.Gamepad2()},
	{Label: "实例", Icon: lucide.Package()},
	{Label: "下载", Icon: lucide.Download()},
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
	return ui.Text(c, label).TextColor(c.Theme().TextMuted)
}

func brand(c *ui.Context) (ink, wash ui.Color) {
	if c.Theme().Dark {
		return ui.Hex("#96c8ff"), ui.Hex("#1e324c")
	}
	return ui.Hex("#175ca7"), ui.Hex("#e6f1ff")
}

func leadIcon(c *ui.Context, svg *ui.SVG, size float32) {
	ink, wash := brand(c)
	ui.Box(c).Size(size, size).Shrink(0).Radius(10).Center().Background(wash).Children(func() {
		ui.Icon(c, svg).FontSize(size * 0.47).TextColor(ink)
	})
}

func surface(c *ui.Context) ui.Element {
	return layout.Block(c, layout.BlockProps{Variant: layout.BlockOutlined})
}

func sectionPane(c *ui.Context) ui.Element {
	return layout.Block(c, layout.BlockProps{Variant: layout.BlockBorderless})
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
	if a.ready && !a.form.open && a.serverForm == nil && a.serverBackup == nil && a.restore == nil && a.localBackups == nil && !a.logOpen && !a.licensesOpen &&
		!a.modpacks.keyOpen &&
		(a.stationLogin == nil || !a.stationLogin.open) &&
		(a.microsoft == nil || !a.microsoft.open) {
		if mods, key, ok := display.HotkeyBinding(searchHotkey, display.HotkeyAuto); ok && c.Shortcut(mods, key) {
			a.page, a.inspected = pageLibrary, 0
			a.instanceFilter, a.libraryGroup, a.libraryLoader, a.libraryVersion = 0, nil, "", ""
			a.focusSearch = len(a.state.Instances) > 0
		}
		if mods, key, ok := display.HotkeyBinding(tasksHotkey, display.HotkeyAuto); ok && c.Shortcut(mods, key) {
			a.openTasks()
		}
		if a.page == pageLibrary && a.inspected != 0 {
			if c.Shortcut(0, ui.KeyEscape) {
				a.backFromDetail()
			} else if !a.busy && c.Shortcut(0, ui.KeyF2) {
				if i, ok := a.state.Find(a.inspected); ok {
					a.edit(i)
				}
			}
		}
		if a.page == pageAccount && c.Shortcut(0, ui.KeyEscape) {
			a.page = a.accountReturn
		}
		if c.Shortcut(0, ui.KeyF1) {
			a.page, a.settingsPage = pageSettings, 2
		}
	}
	ui.Column(c).Fill().Background(theme.TokensOf(c).BgContainer).Children(func() {
		a.topBar(c)
		if a.page != pageNetwork && a.gameServers != nil {
			a.closeGameServers()
		}
		if p := a.screenshots; p != nil && (a.page != pageLibrary || a.detailPage != detailScreenshots || a.inspected != p.instance.ID) {
			a.closeScreenshots()
		}
		if p := a.backups; p != nil && (a.page != pageLibrary || a.detailPage != detailFiles || a.inspected != p.instanceID) {
			if p.cancel != nil {
				p.cancel()
			}
			a.backups = nil
		}
		if a.page != pageDownloads {
			a.clearResourceBindings(-1)
		}
		switch {
		case a.loadErr != "":
			ui.Column(c).Grow(1).Padding(24).Center().Children(func() {
				ui.Column(c).FillWidth().MaxWidth(680).Gap(12).Children(func() {
					feedback.Alert(c, "无法读取本机配置", feedback.AlertProps{Type: feedback.Error, Description: a.loadErr})
					muted(c, "修复提示中的文件或恢复读取权限后，可重新读取配置。").FontSize(13)
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						if button(c, "重新读取配置", action.Primary, false) {
							a.load()
						}
						if button(c, "打开配置目录", action.Default, false) {
							a.openPath(a.store.Root, false)
						}
						action.CopyButton(c, a.loadErr, action.CopyButtonProps{}).Label("复制错误详情")
					})
				})
			})
		case !a.ready:
			ui.Column(c).Grow(1).Center().Children(func() { flightLoading(c, "正在读取启动器设置…") })
		default:
			switch a.page {
			case pageHome:
				a.home(c)
			case pageLibrary:
				a.library(c)
			case pageDownloads:
				a.downloads(c)
			case pageNetwork:
				a.network(c)
			case pageSettings:
				a.settings(c)
			case pageAccount:
				a.accounts(c.Key("accounts"))
			}
		}
		if a.page != pageDownloads || a.downloadPage != downloadGame {
			a.install.group = ""
		}
		if a.task.Stage != "" && !(a.page == pageDownloads && a.downloadPage == downloadTasks) {
			a.progress(c)
		} else if a.notice != "" && a.task.Stage == "" {
			a.noticeBar(c)
		}
		a.editor(c.Key("instance-editor"))
		a.logDialog(c.Key("game-log"))
		a.localBackupDialog(c.Key("local-backups"))
		a.restoreDialog(c.Key("restore-save"))
		a.serverDialog(c.Key("server-editor"))
		a.serverBackupDialog(c.Key("server-backup"))
		a.licensesDialog(c.Key("third-party-licenses"))
		a.microsoftLoginDialog(c.Key("microsoft-login"))
		a.stationLoginDialog(c.Key("station-login"))
		a.curseKeyDialog(c.Key("curseforge-api-key"))
	})
}

func (a *desktop) noticeBar(c *ui.Context) {
	showingNoticeTask := a.page == pageDownloads && a.downloadPage == downloadTasks && a.browsedTask == a.noticeTaskKey && !a.noticeTaskFiltered()
	tk := theme.TokensOf(c)
	layout.Divider(c, layout.DividerProps{})
	ui.Row(c).Padding(5, 18).Gap(8).AlignItems(ui.Center).Background(tk.BgContainer).Children(func() {
		if a.noticeError {
			ui.Icon(c, lucide.CircleX()).FontSize(16).TextColor(tk.Error).Shrink(0)
			display.Text(c, a.notice, display.TextProps{Rows: 2}).Grow(1).MinWidth(0).Ellipsis("…").Selectable()
			action.CopyButton(c, a.notice, action.CopyButtonProps{}).Label("复制通知详情")
		} else {
			feedback.Alert(c, a.notice, feedback.AlertProps{Type: feedback.Info, Variant: feedback.Plain}).Grow(1).MinWidth(0)
		}
		if a.noticeHistory {
			if button(c, "查看任务记录", action.Text, false) {
				a.openTasks()
			}
		} else if a.noticeAccount {
			if button(c, "打开账号页", action.Text, false) {
				a.openAccounts(a.state.StationKey == "")
			}
		} else if a.noticeJava {
			if button(c, "Java 设置", action.Text, false) {
				target := a.noticeJavaTarget
				if target.ID == 0 {
					a.page, a.settingsPage = pageSettings, 1
				} else if i, ok := a.state.Find(target.ID); ok && i.Revision == target.Revision {
					a.inspect(i)
					a.detailPage = detailLaunch
				} else {
					a.openNoticeTask()
				}
			}
		} else if a.noticeTaskKey != "" && !showingNoticeTask && button(c, "查看任务", action.Text, false) {
			a.openNoticeTask()
		}
		if icon(c, lucide.X(), "关闭通知", false) {
			a.notice = ""
		}
	})
}

func (a *desktop) backFromDetail() {
	if a.detailPage == detailContent && a.content.restoreMods != nil {
		a.closeModRestore()
		return
	}
	if a.detailPage == detailContent && a.content.packPreview != nil {
		a.closePackPreview()
		return
	}
	if a.detailPage == detailContent && a.content.savedPacks != nil {
		a.closeSavedPacks()
		return
	}
	if a.detailPage == detailContent && a.content.copyMods != nil {
		a.closeModCopy()
		return
	}
	if p := a.screenshots; p != nil && p.viewerFrom != "" && a.detailPage == detailScreenshots && a.inspected == p.instance.ID {
		a.leaveScreenshotViewer(p)
		return
	}
	if a.detailPage != detailOverview {
		a.detailPage = detailOverview
	} else {
		a.inspected = 0
	}
}

const titleBarHeight = 42

func (a *desktop) topBar(c *ui.Context) {
	tk, bar := theme.TokensOf(c), c.TitleBar()
	w, _ := c.Size()
	height := max(bar.Height, titleBarHeight)
	ui.Box(c).Height(height).FillWidth().Background(tk.BgContainer).Shrink(0).DragWindow().Children(func() {
		ui.Row(c).Absolute().Left(bar.Left + 14).Top(0).Bottom(0).
			Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Image(c, a.logo).Size(23, 23).Fit(ui.Contain)
			display.Text(c, "LXMCL", display.TextProps{Strong: true, FontSize: 14})
		})
		if !a.ready {
			return
		}
		ui.Row(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).
			Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			a.mainNavigation(c)
		})
		ui.Row(c).Absolute().Right(bar.Right + 10).Top(0).Bottom(0).Gap(6).
			AlignItems(ui.Center).Children(func() {
			ink, _ := brand(c)
			feedback.Badge(c, feedback.BadgeProps{Dot: a.task.Stage != "" || a.running != nil, Color: ink}, func() {
				if icon(c, lucide.ListTodo(), "任务中心", !a.ready) {
					a.openTasks()
				}
			}).AlignSelf(ui.Center)
			name := a.accountName()
			account := action.StyleButton(c, ui.ButtonBase(c).Label("账号中心"), "",
				action.ButtonProps{Variant: action.Text, Size: theme.Small}).PaddingX(5).Gap(6).Shrink(0).Disabled(!a.ready)
			if a.page == pageAccount {
				_, wash := brand(c)
				account.Background(wash)
			}
			account.Children(func() {
				if profile, ok := a.state.Station(a.state.StationKey); ok {
					a.stationAvatar(c, profile, 24)
				} else {
					a.accountAvatar(c, name, a.state.MicrosoftProfile != "", 24)
				}
				ink, _ := brand(c)
				if w >= 920 {
					display.Text(c, name, display.TextProps{FontSize: 12, Rows: 1}).MaxWidth(88).Ellipsis("…").TextColor(ink)
				}
			})
			if account.Clicked() {
				if a.page == pageAccount {
					a.page = a.accountReturn
				} else {
					a.openAccounts(false)
				}
			}
		})
	})
}

func (a *desktop) mainNavigation(c *ui.Context) {
	if a.page != pageAccount && !a.boundDownload() {
		nav.Tabs(c, &a.page, nav.TabsProps{Items: mainTabs, Variant: nav.TabsRounded, Size: theme.Small}, nil).
			Label("主导航").AlignSelf(ui.Center).Shrink(0)
		return
	}
	// Account and instance-scoped downloads are entered from their own controls.
	tk := theme.TokensOf(c)
	ui.Row(c).Gap(4).Padding(3).Radius(tk.BorderRadiusLG).Background(tk.BgLayout).
		Label("主导航").AlignSelf(ui.Center).Shrink(0).Children(func() {
		for page, item := range mainTabs {
			b := action.StyleButton(c, ui.ButtonBase(c.Key(page)).Label(item.Label), "",
				action.ButtonProps{Variant: action.Text, Size: theme.Small}).Height(26).PaddingX(10)
			b.Children(func() {
				ui.Icon(c, item.Icon).FontSize(14)
				ui.Text(c, item.Label).FontSize(12).SingleLine()
			})
			if b.Clicked() {
				a.clearResourceBindings(-1)
				a.page = page
			}
		}
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
		panelHeading(c, lucide.PackageOpen(), "开始游戏", "选择游戏版本，创建你的第一个实例。")
		if launchButton(c, "安装实例…", lucide.Download(), false) {
			a.create()
		}
		a.homeAccount(c)
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
	catalogPicker(c, &selection, options, "切换启动实例", "", a.busy)
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
			a.detailPage = detailLaunch
		}
	})
	a.homeAccount(c)
}

func (a *desktop) homeAccount(c *ui.Context) {
	layout.Divider(c, layout.DividerProps{})
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		kind := "离线"
		if a.state.StationKey != "" {
			kind = "皮肤站"
		} else if a.state.MicrosoftProfile != "" {
			kind = "Microsoft"
		}
		muted(c, kind+" · "+a.accountName()).FontSize(12).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		if action.Button(c, "切换账号", action.ButtonProps{Variant: action.Text, Size: theme.Small}).Label("切换启动账号").Clicked() {
			a.openAccounts(false)
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

func launchButton(c *ui.Context, label string, glyph *ui.SVG, disabled bool) bool {
	b := action.StyleButton(c, ui.ButtonBase(c).Label(label), "", action.ButtonProps{Variant: action.Primary}).
		FillWidth().Disabled(disabled)
	b.Children(func() {
		ui.Icon(c, glyph).FontSize(17)
		ui.Text(c, label).SingleLine()
	})
	return b.Clicked()
}

func sidebarNavigation(c *ui.Context, selected *int, items []nav.TabItem) {
	rows := make([]display.ListItem, 0, len(items))
	for id, item := range items {
		rows = append(rows, display.ListItem{Key: strconv.Itoa(id), Label: item.Label, Icon: item.Icon})
	}
	if key := display.List(c, display.ListProps{Items: rows, Active: strconv.Itoa(*selected), Compact: true}); key != "" {
		*selected, _ = strconv.Atoi(key)
		c.Invalidate()
	}
}

func workspace(c *ui.Context, content func()) {
	ui.Column(c).Grow(1).MinHeight(0).Padding(12, 18).Gap(10).Children(content)
}

func (a *desktop) library(c *ui.Context) {
	workspace(c, func() {
		if p := a.screenshots; p != nil && p.viewerFrom != "" {
			if i, ok := a.state.Find(a.inspected); ok && i == p.instance {
				a.screenshotViewer(c.Key("screenshot-viewer"), p)
				return
			}
			a.closeScreenshots()
		}
		if len(a.state.Instances) == 0 {
			ui.Column(c).Grow(1).Center().Children(func() {
				display.Empty(c, display.EmptyProps{Icon: lucide.PackageOpen(), Title: "暂无实例",
					Description: "安装一个 Minecraft 版本，或导入本地原版实例。",
					Action: func() {
						if button(c, "安装实例…", action.Primary, false) {
							a.create()
						}
						if button(c, "导入本地实例…", action.Default, a.busy || a.pickerBusy) {
							a.startPrismImport()
						}
					}})
			})
			return
		}
		w, _ := c.Size()
		query, filter := a.query, a.instanceFilter
		compact := w < 960
		filters := func() {
			a.libraryGroupPicker(c)
			a.libraryLoaderPicker(c)
			a.libraryVersionPicker(c)
			action.Segmented(c, &a.instanceFilter, []string{"全部", "已缓存", "待下载"},
				action.SegmentedProps{Size: theme.Small}).Label("实例筛选")
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				hotkey := searchHotkey
				if w < 920 || a.form.open || a.restore != nil || a.logOpen || a.licensesOpen {
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
			if !compact {
				filters()
			}
			if button(c, "导入本地实例…", action.Default, a.busy || a.pickerBusy) {
				a.startPrismImport()
			}
			if button(c, "安装实例…", action.Primary, false) {
				a.create()
			}
		})
		if compact {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(filters)
		}
		if a.query != query || a.instanceFilter != filter {
			a.scroll.Y = 0
		}
		splitWorkspace(c, listWidth(w), func() {
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				muted(c, "实例列表").Grow(1)
				a.librarySortPicker(c)
			})
			_, inspecting := a.state.Find(a.inspected)
			a.instanceList(c, inspecting)
			if inspecting {
				layout.Divider(c, layout.DividerProps{})
				sidebarNavigation(c.Key("instance-nav"), &a.detailPage, instanceDetailTabs)
			}
		}, func() {
			i, ok := a.state.Find(a.inspected)
			detailPane(c.Key(a.inspected), func() {
				switch {
				case ok:
					a.details(c, i)
				default:
					ui.Column(c).Grow(1).Center().Children(func() {
						display.Empty(c, display.EmptyProps{Icon: lucide.Package(), Title: "选择一个实例查看详情"})
					})
				}
			})
		})
	})
}

func matchesInstanceSearch(i launcher.Instance, query string) bool {
	return query == "" || strings.Contains(strings.ToLower(i.Name+" "+i.Version+" "+loaderName(i)), query)
}

func (a *desktop) matchesCacheFilter(i launcher.Instance) bool {
	switch a.instanceFilter {
	case 1:
		return a.installed[i.ID]
	case 2:
		return !a.installed[i.ID]
	default:
		return true
	}
}

func (a *desktop) instanceList(c *ui.Context, inspecting bool) {
	var items []display.ListItem
	for _, i := range a.libraryInstances() {
		detail := i.Version + " · " + loaderName(i)
		if a.state.PinnedInstances[i.ID] {
			detail = "已置顶 · " + detail
		}
		if group := a.state.InstanceGroups[i.ID]; group != "" {
			detail += " · " + group
		}
		if i.ID == a.state.Selected {
			detail += " · 启动目标"
		}
		item := display.ListItem{
			Key: strconv.Itoa(i.ID), Label: i.Name,
			Description: detail, Icon: loaderIcon(i),
			Actions:    func() { a.instanceMenu(c.Key("list-"+strconv.Itoa(i.ID)), i, true) },
			ShowAction: i.ID == a.listMenuID,
		}
		if picture := a.instanceIcons[i.ID]; picture != nil {
			item.Avatar = func() { instanceIcon(c, i, picture, 20, "列表图标 "+i.Name) }
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		display.Empty(c, display.EmptyProps{Icon: lucide.SearchX(), Title: "未找到实例", Action: func() {
			hasPreference := a.state.LibraryGroup != nil || a.state.LibraryLoader != "" || a.state.LibraryVersion != "" ||
				a.libraryGroup != nil || a.libraryLoader != "" || a.libraryVersion != ""
			if button(c, "清除筛选", action.Default, a.busy && hasPreference) {
				if hasPreference {
					next := a.state.Clone()
					next.LibraryGroup, next.LibraryLoader, next.LibraryVersion = nil, "", ""
					a.saveLibraryFilters(next, nil, "", "")
				}
				a.query, a.instanceFilter, a.scroll.Y = "", 0, 0
			}
		}})
		return
	}
	var clicked string
	rows := func() {
		clicked = display.List(c, display.ListProps{Items: items, Active: strconv.Itoa(a.inspected), Compact: true})
	}
	if inspecting && len(items) <= 3 {
		rows()
	} else {
		list := layout.ScrollArea(c, layout.ScrollAreaProps{State: &a.scroll}, rows).MinHeight(0)
		if inspecting {
			list.Height(188).Shrink(1)
		} else {
			list.Grow(1)
		}
	}
	if id, err := strconv.Atoi(clicked); err == nil && id != a.inspected {
		if i, ok := a.state.Find(id); ok {
			a.inspect(i)
			c.Invalidate()
		}
	}
}

func loaderIcon(i launcher.Instance) *ui.SVG {
	switch {
	case i.Fabric():
		return lucide.Layers()
	case i.Forge():
		return lucide.Anvil()
	case i.NeoForge():
		return lucide.Hammer()
	case i.Quilt():
		return lucide.Grid2X2()
	default:
		return lucide.Box()
	}
}

func loaderName(i launcher.Instance) string {
	return i.LoaderName()
}

func (a *desktop) instanceStatus(i launcher.Instance) (string, display.TagColor) {
	switch {
	case a.runningID == i.ID:
		return "游戏运行中", display.TagProcessing
	case a.installed[i.ID]:
		return "已缓存", display.TagSuccess
	default:
		return "尚未安装", display.TagGray
	}
}

func emptyPanel(c *ui.Context, glyph *ui.SVG, title string) {
	sectionPane(c).Padding(12).Children(func() {
		display.Empty(c, display.EmptyProps{Icon: glyph, Title: title})
	})
}

func (a *desktop) editor(c *ui.Context) {
	if a.form.mode == "" {
		return
	}
	title, okText := "重命名实例", "保存"
	width := float32(480)
	switch a.form.mode {
	case "worlds":
		title, width = "本地存档", 640
		if p := a.form.worlds; p != nil {
			switch {
			case p.copy != nil:
				title, width = "复制世界", 552
			case p.packs != nil:
				title, width = "世界数据包", 552
			case p.retry != nil:
				width = 552
			}
		}
	case "prism":
		title = "导入 Prism / MultiMC 实例"
	case "icon":
		title = "实例图标 · " + a.form.target.Name
	case "moveGroups":
		title = "批量设置分组"
	case "renameGroup":
		title, okText = "重命名分组 · "+a.form.originalGroup, "保存名称"
	case "group":
		title, okText = "设置分组 · "+a.form.target.Name, "保存分组"
	case "note":
		title, okText = "实例备注 · "+a.form.target.Name, "保存备注"
	case "remove":
		title, okText = "移除实例配置", "移除配置"
	case "version":
		title, okText = "更换版本", "保存版本"
	case "duplicate":
		title, okText = "复制实例", "创建副本"
	case "prismExport":
		title, okText = "导出 Prism / MultiMC 实例", "选择目标文件…"
	case "export":
		title, okText = "导出整合包", "选择保存位置…"
	case "loader":
		title, okText = "选择加载器", "保存加载器"
	}
	submit := false
	if feedback.Modal(c, &a.form.open, feedback.ModalProps{
		Title: title, OkText: okText, CancelText: "取消", Width: width, NoFooter: a.form.loaders != nil || a.form.icon != nil || a.form.prism != nil || a.form.worlds != nil || a.form.groupMove != nil,
	}, func() {
		ui.Column(c).Gap(12).Children(func() {
			if p := a.form.groupMove; p != nil {
				submit = a.groupMoveEditor(c, p)
				return
			}
			if a.form.worlds != nil {
				a.worldEditor(c)
				return
			}
			if d := a.form.prismExport; d != nil {
				a.prismExportEditor(c, d)
				return
			}
			if a.form.prism != nil {
				a.prismEditor(c)
				return
			}
			if a.form.icon != nil {
				submit = a.instanceIconEditor(c)
				return
			}
			if a.form.mode == "group" || a.form.mode == "renameGroup" {
				submit = a.groupEditor(c)
				return
			}
			if a.form.mode == "note" {
				area := field.TextArea(c, &a.form.note, field.TextAreaProps{MinRows: 4, MaxRows: 7, Placeholder: "记录游玩进度、坐标或下次要做的事"}).Label("备注内容").Disabled(a.busy)
				if a.form.focus {
					area.Focus()
					a.form.focus = false
				}
				if a.form.err != "" {
					feedback.Alert(c, a.form.err, feedback.AlertProps{Type: feedback.Error})
				}
				return
			}
			if a.form.mode == "loader" {
				a.loaderForm(c)
				return
			}
			if a.form.mode == "export" {
				panelHeading(c, lucide.FileArchive(), a.form.target.Name, "Modrinth 整合包 (.mrpack)")
				settingsInput(c, "整合包版本", &a.form.version, "1.0.0", 170, a.busy)
				a.portableOptions(c)
				if a.form.err != "" {
					feedback.Alert(c, a.form.err, feedback.AlertProps{Type: feedback.Error})
				}
				return
			}
			if a.form.mode == "remove" {
				feedback.Alert(c, "移除「"+a.form.target.Name+"」的配置",
					feedback.AlertProps{Type: feedback.Warning, Description: "游戏目录与存档会保留。"})
				if a.form.err != "" {
					feedback.Alert(c, a.form.err, feedback.AlertProps{Type: feedback.Error})
				}
				return
			}
			if a.form.mode == "version" {
				panelHeading(c, lucide.Package(), a.form.target.Name, "当前版本 "+a.form.target.Version+" · "+loaderName(a.form.target))
				feedback.Alert(c, "更换版本会继续使用原游戏目录。降级前请备份存档。",
					feedback.AlertProps{Type: feedback.Warning})
			} else {
				if a.form.mode == "duplicate" {
					ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Children(func() {
							panelHeading(c, lucide.Copy(), a.form.target.Name, "Minecraft "+a.form.target.Version+" · "+loaderName(a.form.target))
						})
						if groups := a.state.GroupNames(); len(groups) > 0 || a.form.copyGroup != "" {
							ui.Column(c).Width(148).Gap(4).Children(func() {
								muted(c, "副本分组").FontSize(12)
								groupDestinationPicker(c, &a.form.copyGroup, groups, "选择副本分组", a.busy)
							})
						}
					})
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
				if a.form.mode == "duplicate" {
					choice.Checkbox(c, &a.form.copyContent, "同时复制资源和游戏设置", choice.CheckboxProps{}).Label("同时复制资源和游戏设置").Disabled(a.busy || a.running != nil)
					if !a.form.copyContent {
						a.portableOptions(c)
					}
					ui.Row(c).Gap(20).Children(func() {
						choice.Checkbox(c, &a.form.files.Screenshots, "游戏截图", choice.CheckboxProps{}).Disabled(a.busy || a.running != nil)
						if !a.form.copyContent {
							choice.Checkbox(c, &a.form.files.ServerList, "游戏内服务器列表", choice.CheckboxProps{}).Disabled(a.busy || a.running != nil)
						}
					})
					if a.form.copyContent {
						display.Text(c, "包含模组、资源包、光影、配置和服务器列表。存档与日志保留在原实例。", display.TextProps{FontSize: 13})
					} else {
						muted(c, "勾选要带入副本的文件。留空可仅复制版本和启动设置。").FontSize(12)
					}
					if a.running != nil {
						muted(c, "退出游戏后可同时复制资源。").FontSize(12)
					}
				}
			}
			if a.form.mode == "version" {
				a.versionFields(c)
			}
			if a.form.err != "" {
				feedback.Alert(c, a.form.err, feedback.AlertProps{Type: feedback.Error})
			}
			if p := a.form.loaders; a.form.mode == "version" && p != nil {
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if button(c, "取消", action.Default, false) {
						a.form.open = false
					}
					submit = button(c, "保存版本", action.Primary, a.busy || p.version == a.form.target.Version || !p.validLoader())
				})
			}
		})
	}) || submit {
		if a.form.open {
			a.submitEditor()
		}
	}
	if p := a.form.loaders; !a.form.open && p != nil && p.loaderCancel != nil {
		p.cancelLoaders()
	}
	if p := a.form.worlds; !a.form.open && p != nil {
		p.closeWorldCopy()
		p.closeWorldPacks()
		p.closeWorldThumbnail()
		p.closeWorldUsage()
		if p.cancel != nil {
			p.cancel()
			p.cancel = nil
		}
	}
}

func (a *desktop) loaderForm(c *ui.Context) {
	f := &a.form
	p := f.loaders
	panelHeading(c, lucide.Layers(), f.target.Name, "Minecraft "+f.target.Version+" · "+loaderName(f.target))
	if a.loaderFields(c, p, f.target, a.busy || !f.open) {
		f.err = ""
	}
	currentKind := f.target.Loader
	if f.target.Vanilla() {
		currentKind = launcher.LoaderVanilla
	}
	changed := !strings.EqualFold(p.kind(), currentKind) || p.loaderVersion != f.target.LoaderVersion
	canSave := changed && p.validLoader()
	if canSave {
		muted(c, "原模组文件会保留，请检查与所选加载器的兼容性。").FontSize(12)
	}
	muted(c, "当前加载器 · "+loaderName(f.target)).FontSize(12)
	if f.err != "" {
		feedback.Alert(c, f.err, feedback.AlertProps{Type: feedback.Error})
	}
	ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
		if button(c, "取消", action.Default, false) {
			f.open = false
		}
		if button(c, "保存加载器", action.Primary, a.busy || !canSave) {
			a.submitEditor()
		}
	})
}

func (a *desktop) portableOptions(c *ui.Context) {
	f := &a.form.files
	if a.form.mode == "duplicate" {
		ui.Row(c).Gap(18).Children(func() {
			choice.Checkbox(c, &f.ModFiles, "模组", choice.CheckboxProps{}).Disabled(a.busy)
			choice.Checkbox(c, &f.ResourcePacks, "资源包", choice.CheckboxProps{}).Disabled(a.busy)
			choice.Checkbox(c, &f.ShaderPacks, "光影包", choice.CheckboxProps{}).Disabled(a.busy)
		})
	} else {
		choice.Checkbox(c, &f.Mods, "模组、资源包和光影包", choice.CheckboxProps{}).Disabled(a.busy)
	}
	choice.Checkbox(c, &f.Config, "游戏选项、模组配置和脚本", choice.CheckboxProps{}).Disabled(a.busy)
	choice.Checkbox(c, &f.Worlds, "世界存档", choice.CheckboxProps{}).Disabled(a.busy)
}
