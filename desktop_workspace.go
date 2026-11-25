package main

import (
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/shortcut"
	"github.com/egoist/mygo"
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
		instanceIcon(c, i, a.instanceIcons[i.ID], 26, "当前实例图标")
		display.Text(c, i.Name, display.TextProps{Strong: true, FontSize: 19, Rows: 1}).Grow(1).MinWidth(0).Ellipsis("…")
		if a.state.Selected != i.ID && button(c, "设为启动目标", action.Default, a.busy) {
			a.choose(i.ID)
		}
		a.instanceMenu(c.Key(i.ID), i, false)
		ui.Column(c).Width(152).Shrink(0).Children(func() {
			d := a.launchDrafts[i.ID]
			if d != nil && d.revision == i.Revision && d.fields.changed(i.Launch) &&
				a.running == nil && a.task.Stage == "" {
				_, err := d.fields.options()
				if err != nil && a.detailPage != detailLaunch {
					if launchButton(c, "检查启动设置", lucide.Settings2(), false) {
						a.detailPage = detailLaunch
					}
				} else if launchButton(c, "应用并启动", lucide.Play(), a.busy || a.pickerBusy || err != nil) {
					a.detailPage = detailLaunch
					a.saveInstanceLaunch(i, d, true)
				}
			} else if a.playButton(c, i, a.pickerBusy) {
				a.play(i, false)
			}
		})
	})
	layout.Divider(c, layout.DividerProps{})
	switch a.detailPage {
	case detailLaunch:
		a.instanceLaunchSettings(c.Key(i.ID), i)
	case detailContent:
		a.instanceContent(c.Key(i.ID), i)
	case detailScreenshots:
		a.instanceScreenshots(c.Key(i.ID), i)
	case detailFiles:
		a.instanceFiles(c.Key(i.ID), i)
	default:
		a.instanceOverview(c.Key(i.ID), i)
	}
}

func (a *desktop) instanceOverview(c *ui.Context, i launcher.Instance) {
	layout.ScrollArea(c, layout.ScrollAreaProps{}, func() {
		sectionPane(c).Padding(12, 16).Gap(8).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() { property(c, "版本", i.Version+" · "+loaderName(i)) })
				if icon(c.Key("share-instance-version"), lucide.Copy(), "复制实例版本信息", false) {
					c.WriteClipboardThen(instanceVersionClipboard(i), func(err error) {
						if err != nil {
							a.message("复制实例版本信息失败，请重试", err)
							return
						}
						a.message("已复制实例版本信息", nil)
					})
				}
				if i.PackFeed != "" && button(c, "检查服主更新", action.Default, a.busy || a.running != nil) {
					a.readPackFeed(i)
				}
				if button(c, "更换版本", action.Default, a.busy || a.runningID == i.ID) {
					a.changeVersion(i)
				}
				if button(c, "加载器", action.Default, a.busy || a.runningID == i.ID) {
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
			layout.Divider(c, layout.DividerProps{})
			a.instanceFileUsage(c.Key("instance-file-usage"), i)
			layout.Divider(c, layout.DividerProps{})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() { property(c, "实例 ID", strconv.Itoa(i.ID)) })
				if button(c, "查看任务历史", action.Default, false) {
					a.openInstanceTaskHistory(i, "")
					c.Invalidate()
				}
			})
			layout.Divider(c, layout.DividerProps{})
			a.instanceLaunchResult(c, i)
			layout.Divider(c, layout.DividerProps{})
			a.playtimeOverview(c, i.ID)
			layout.Divider(c, layout.DividerProps{})
			note := a.state.InstanceNotes[i.ID]
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				muted(c, "实例备注").Grow(1)
				label := "添加备注…"
				if note != "" {
					label = "编辑备注…"
				}
				if button(c, label, action.Default, a.busy) {
					a.form = editor{mode: "note", open: true, target: i, note: note, originalNote: note, focus: true}
				}
			})
			if note != "" {
				display.Text(c, note, display.TextProps{Rows: 4}).Ellipsis("…").Selectable()
			}
		})
	}).Grow(1).MinHeight(0)
}

func (a *desktop) instanceLaunchResult(c *ui.Context, i launcher.Instance) {
	const label = "上次启动结果"
	if a.historyReadErr != "" {
		property(c, label, "无法读取任务记录")
		return
	}
	row, ok := a.latestFinishedLaunch(i.ID)
	if !ok {
		property(c, label, "暂无已结束的启动记录")
		return
	}
	at := "时间未知"
	if !row.At.IsZero() {
		at = row.At.Local().Format("2006-01-02 15:04")
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			property(c, label, row.Status+" · "+at)
		})
		key := taskKey(row)
		if button(c, "查看启动任务", action.Default, !a.hasUniqueInstanceTaskKey(i.ID, key)) {
			a.openInstanceTaskHistory(i, key)
			c.Invalidate()
		}
	})
}

func (a *desktop) instanceMenu(c *ui.Context, i launcher.Instance, inList bool) {
	box := ui.Row(c).Shrink(0)
	open := ui.Local(box, "open", func() bool { return false })
	title := "更多实例操作"
	if inList {
		title = "实例操作 " + i.Name
	}
	box.Children(func() {
		install := "下载游戏文件"
		if a.installed[i.ID] {
			install = "校验游戏文件"
		}
		pinned := a.state.PinnedInstances[i.ID]
		pinLabel, pinIcon := "置顶实例", lucide.Pin()
		if pinned {
			pinLabel, pinIcon = "取消置顶", lucide.PinOff()
		}
		renameKey := ""
		if a.inspected == i.ID {
			renameKey = "f2"
		}
		items := []overlay.MenuItem{
			{Key: "launch", Label: "启动此实例", Icon: lucide.Play(), Disabled: a.busy || a.running != nil},
			{Key: "verify", Label: install, Icon: lucide.Download(), Disabled: a.busy || a.running != nil},
			{Key: "folder", Label: "打开游戏目录", Icon: lucide.FolderOpen()},
			{Divider: true},
			{Key: "icon", Label: "更换图标…", Icon: lucide.Image(), Disabled: a.busy},
			{Key: "pin", Label: pinLabel, Icon: pinIcon, Disabled: a.busy},
			{Key: "group", Label: "设置分组…", Icon: lucide.Folder(), Disabled: a.busy},
			{Key: "rename", Label: "重命名", Icon: lucide.Pencil(), Hotkey: renameKey, Disabled: a.busy},
			{Key: "prismExport", Label: "导出实例 ZIP…", Icon: lucide.FileArchive(), Disabled: a.busy || a.running != nil || !i.Vanilla()},
			{Key: "export", Label: "导出整合包…", Icon: lucide.FileArchive(), Disabled: a.busy || a.running != nil},
			{Key: "duplicate", Label: "复制实例…", Icon: lucide.Copy(), Disabled: a.busy || (!i.Vanilla() && !i.Fabric() && !i.Forge() && !i.NeoForge() && !i.Quilt())},
			{Divider: true},
			{Key: "remove", Label: "移除配置…", Icon: lucide.Trash2(), Danger: true, Disabled: a.busy || a.runningID == i.ID},
		}
		if shortcut.Supported {
			items = slices.Insert(items, 1, overlay.MenuItem{Key: "shortcut", Label: "创建启动快捷方式…", Icon: lucide.ExternalLink(), Disabled: a.pickerBusy || a.busy})
		}
		chosen := overlay.DropdownMenu(c, open, overlay.DropdownMenuProps{Items: items}, func(trigger ui.Element) {
			action.StyleActionIcon(c, trigger, lucide.Ellipsis(), action.ActionIconProps{Title: title, BlockSize: 28})
		})
		if inList && *open && a.listMenuID != i.ID {
			a.listMenuID = i.ID
			c.Invalidate()
		} else if inList && !*open && a.listMenuID == i.ID {
			a.listMenuID = 0
			c.Invalidate()
		}
		switch chosen {
		case "launch":
			a.play(i, false)
		case "shortcut":
			a.chooseLaunchShortcut(i)
		case "verify":
			a.play(i, true)
		case "folder":
			a.openPath(i.GameDir, true)
		case "icon":
			a.editInstanceIcon(i)
		case "pin":
			a.setInstancePinned(i, !pinned)
		case "group":
			a.editGroup(i)
		case "rename":
			a.edit(i)
		case "prismExport":
			a.form = editor{open: true, mode: "prismExport", target: i, prismExport: &prismExportDraft{}}
		case "export":
			a.form = editor{open: true, mode: "export", target: i, version: "1.0.0", files: launcher.PortableFiles{Mods: true}}
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

func previewPath(c *ui.Context, directory, copyLabel string) {
	base := filepath.Base(directory)
	ui.Row(c).AlignItems(ui.Center).Grow(1).MinWidth(0).Tooltip(directory).Children(func() {
		muted(c, strings.TrimSuffix(directory, base)).FontSize(12).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
		muted(c, base).FontSize(12).SingleLine().Shrink(0)
	})
	action.CopyButton(c, directory, action.CopyButtonProps{Size: theme.Small}).Label(copyLabel).Shrink(0)
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
	sectionPane(c).Padding(8, 14).Gap(5).Children(func() {
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
	disabled = disabled || a.javaChecking
	sectionPane(c).Padding(8, 14).Gap(5).Children(func() {
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
		switch {
		case a.javaChecking:
			flightLoading(c, "正在检测本机 Java…")
		case a.javaError && a.javaInfo != "":
			feedback.Alert(c, "Java 检测失败", feedback.AlertProps{Type: feedback.Error, Description: a.javaInfo})
		case a.javaInfo != "":
			display.Text(c, a.javaInfo, display.TextProps{Strong: true}).Selectable()
			muted(c, "在实例的「启动设置」中可检测该游戏版本所需的 Java。").FontSize(12)
		}
	})
}

var settingsTabs = []nav.TabItem{
	{Label: "游戏", Icon: lucide.Gamepad2()},
	{Label: "Java", Icon: lucide.Coffee()},
	{Label: "关于", Icon: lucide.CircleHelp()},
}

func (a *desktop) settings(c *ui.Context) {
	workspace(c, func() {
		splitWorkspace(c, 168, func() {
			muted(c, "设置").Padding(3, 5)
			sidebarNavigation(c.Key("settings-nav"), &a.settingsPage, settingsTabs)
		}, func() {
			key := a.settingsPage
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
							a.help(c)
						}
					})
				}).Grow(1).MinHeight(0)
				a.settingsFooter(c)
			})
		})
	})
}

func (a *desktop) settingsFooter(c *ui.Context) {
	dirty := a.draft != a.state.Settings || a.memory != strconv.Itoa(a.state.Settings.Memory)
	if !dirty {
		return
	}
	_, err := a.settingsDraft()
	if err != nil {
		feedback.Alert(c, err.Error(), feedback.AlertProps{Type: feedback.Error})
	}
	settingsActions(c, a.busy || a.pickerBusy || a.javaChecking, err != nil, func() {
		a.draft, a.memory, a.javaInfo = a.state.Settings, strconv.Itoa(a.state.Settings.Memory), ""
	}, a.saveSettings)
}

func settingsActions(c *ui.Context, disabled, invalid bool, reset, apply func()) {
	layout.Divider(c, layout.DividerProps{})
	ui.Row(c).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() {
		muted(c, "有未保存的更改").FontSize(12).Grow(1).MinWidth(0)
		if button(c, "还原更改", action.Text, disabled) {
			reset()
		}
		if button(c, "应用设置", action.Primary, disabled || invalid) {
			apply()
		}
	})
}

func (a *desktop) help(c *ui.Context) {
	heading(c, "关于 LXMCL")
	sectionPane(c).Padding(8, 14).Gap(5).Children(func() {
		settingsRow(c, "版本", func() {
			version := mygo.App.Version()
			if version == "" {
				version = "开发版本"
			}
			info := "LXMCL " + version + " · " + runtime.GOOS + "/" + runtime.GOARCH
			display.Text(c, info, display.TextProps{}).Grow(1).MinWidth(0)
			action.CopyButton(c, info, action.CopyButtonProps{}).Label("复制版本信息")
		})
		layout.Divider(c, layout.DividerProps{})
		settingsRow(c, "项目", func() {
			if button(c, "源代码", action.Default, false) {
				c.OpenURL("https://github.com/CAUCraft/LXMCL")
			}
			if button(c, "反馈问题", action.Default, false) {
				c.OpenURL("https://github.com/CAUCraft/LXMCL/issues")
			}
		})
		a.releaseControls(c)
		layout.Divider(c, layout.DividerProps{})
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
