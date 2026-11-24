package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/nav"
)

const (
	downloadGame = iota
	downloadModpacks
	downloadMods
	downloadResourcePacks
	downloadShaders
	downloadTasks
)

var downloadTabs = []nav.TabItem{
	{Label: "游戏版本", Icon: lucide.Package()},
	{Label: "整合包", Icon: lucide.Archive()},
	{Label: "模组", Icon: lucide.Puzzle()},
	{Label: "资源包", Icon: lucide.Layers()},
	{Label: "光影包", Icon: lucide.Sun()},
	{Label: "下载任务", Icon: lucide.ListTodo()},
}

func downloadKind(tab int) launcher.ContentKind {
	switch tab {
	case downloadMods:
		return launcher.Mods
	case downloadShaders:
		return launcher.ShaderPacks
	default:
		return launcher.ResourcePacks
	}
}

func downloadTab(kind launcher.ContentKind) int {
	switch kind {
	case launcher.Mods:
		return downloadMods
	case launcher.ShaderPacks:
		return downloadShaders
	default:
		return downloadResourcePacks
	}
}

func contentLabel(kind launcher.ContentKind) string { return downloadTabs[downloadTab(kind)].Label }

func (a *desktop) downloads(c *ui.Context) {
	workspace(c, func() {
		splitWorkspace(c, 122, func() {
			muted(c, "下载分类").Padding(3, 6)
			sidebarNavigation(c.Key("download-nav"), &a.downloadPage, downloadTabs)
		}, func() {
			a.clearResourceBindings(a.downloadPage)
			switch a.downloadPage {
			case downloadGame:
				a.installPage(c.Key("install-game"))
			case downloadModpacks:
				a.modpackPage(c.Key("install-modpack"))
			case downloadTasks:
				a.tasks(c.Key("downloads-history"))
			default:
				kind := downloadKind(a.downloadPage)
				a.resourceDownloads(c.Key(kind), a.browserFor(kind))
			}
		})
	})
}

func (a *desktop) clearResourceBindings(keep int) {
	for kind, p := range a.packs {
		if p.target.ID != 0 && downloadTab(kind) != keep {
			p.setTarget(launcher.Instance{})
		}
	}
}

func (a *desktop) boundDownload() bool {
	if a.page != pageDownloads || a.downloadPage < downloadMods || a.downloadPage > downloadShaders {
		return false
	}
	p := a.packs[downloadKind(a.downloadPage)]
	return p != nil && p.target.ID != 0
}

func catalogListWidth(w float32) float32 { return min(256, max(158, (w-150)*0.24)) }

type installView struct {
	loaderSelection
	query, name, group, err string
	java                    *releaseJavaPreview
	cache                   *installCachePreview
	directory               *installDirectoryCheck
	space                   *installSpaceCheck
	existing                *installExistingView
	scroll                  ui.ScrollState
}

func (a *desktop) openInstall(version string, fabric bool) {
	a.page, a.downloadPage = pageDownloads, downloadGame
	if version != "" {
		a.install.version, a.install.query = version, ""
	}
	if fabric {
		a.install.loader = 1
	}
}

// The cached selection is confined to the game installation page.
func (a *desktop) installReleases() ([]launcher.Version, bool) {
	if len(a.cachedReleases) > 0 && (len(a.releases) == 0 || a.catalogErr != "") {
		return a.cachedReleases, true
	}
	return a.releases, false
}

func (a *desktop) installPage(c *ui.Context) {
	p := &a.install
	if len(a.releases) == 0 && a.catalogErr == "" {
		a.refreshCatalog()
	}
	versions, cached := a.installReleases()
	hasMatches := false
	w, _ := c.Size()
	splitWorkspace(c, catalogListWidth(w), func() {
		previousQuery := p.query
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			field.SearchBar(c, &p.query, field.SearchBarProps{Placeholder: "搜索游戏版本"}).Label("搜索游戏版本").Grow(1).MinWidth(0)
			if icon(c, lucide.RefreshCw(), "刷新游戏版本", a.catalogBusy) {
				a.refreshCatalog()
			}
		})
		hasMatches = a.installVersions(c, p, versions, cached, p.query != previousQuery)
		if p.java != nil && (!hasMatches || p.java.version != p.version) {
			p.clearJavaPreview()
		}
	}, func() {
		if cached {
			muted(c, "上次同步的正式版列表 · 仅可创建原版实例").FontSize(13)
		}
		if !hasMatches {
			p.cache, p.directory, p.space = nil, nil, nil
			ui.Column(c).Grow(1).MinWidth(0).Gap(10).Justify(ui.Center).Children(func() {
				switch {
				case a.catalogBusy && !cached:
					flightLoading(c, "正在读取官方版本…")
				case a.catalogErr != "" && !cached:
					title := "无法读取游戏版本列表"
					if len(versions) > 0 {
						title = "版本列表未更新，上次读取的列表中没有匹配项"
					}
					if catalogRetry(c, title, a.catalogErr, "重试读取版本", "复制版本列表错误") {
						a.refreshCatalog()
					}
				case len(versions) > 0:
					display.Empty(c, display.EmptyProps{Icon: lucide.Search(), Title: "没有匹配的游戏版本", Description: "支持 Minecraft 1.20.1 及之后的正式版。"})
				}
				if len(versions) > 0 {
					ui.Row(c).Justify(ui.Center).Children(func() {
						if button(c, "清除搜索", action.Default, false) {
							p.query, p.scroll.Y = "", 0
							c.Invalidate()
						}
					})
				}
			})
			return
		}
		a.installDetails(c, p, versions, cached)
	})
}

func (a *desktop) installVersions(c *ui.Context, p *installView, versions []launcher.Version, cached, queryChanged bool) bool {
	q := strings.ToLower(strings.TrimSpace(p.query))
	if queryChanged {
		p.scroll.Y = 0
	}
	items := make([]display.ListItem, 0, len(versions))
	selectedVisible := false
	exact := ""
	for idx, v := range versions {
		if q != "" && !strings.Contains(strings.ToLower(v.ID), q) {
			continue
		}
		if v.ID == p.version {
			selectedVisible = true
		}
		if strings.EqualFold(v.ID, q) {
			exact = v.ID
		}
		item := display.ListItem{Key: v.ID, Label: v.ID, Icon: lucide.Package()}
		if idx == 0 && !cached {
			item.Extra = "最新正式版"
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return false
	}
	if !selectedVisible || queryChanged && exact != "" {
		p.version, p.err = items[0].Key, ""
		if exact != "" {
			p.version = exact
		}
	}
	layout.ScrollArea(c, layout.ScrollAreaProps{State: &p.scroll}, func() {
		if key := display.List(c, display.ListProps{Items: items, Active: p.version, Compact: true}); key != "" && key != p.version {
			p.version, p.err = key, ""
			c.Invalidate()
		}
	}).Grow(1).MinHeight(0)
	return true
}

func (a *desktop) installDetails(c *ui.Context, p *installView, versions []launcher.Version, cached bool) {
	groups := a.state.GroupNames()
	loader := p.kind()
	if cached {
		loader = launcher.LoaderVanilla
	}
	targetReady := false
	heading(c, "Minecraft "+p.version, func() {
		if button(c, "查看已有实例", action.Default, false) {
			a.browseExistingVersion(p.version)
			c.Invalidate()
		}
	})
	layout.Divider(c, layout.DividerProps{})
	layout.ScrollArea(c, layout.ScrollAreaProps{}, func() {
		ui.Column(c).Gap(8).Padding(4, 2).Children(func() {
			settingsRow(c, "实例名称", func() {
				before := p.name
				settingsInput(c, "实例名称", &p.name, a.installName(p.version, loader), 0, false)
				if before != p.name {
					p.err = ""
				}
			})
			if len(groups) > 0 || p.group != "" {
				settingsRow(c, "实例分组", func() {
					previous := p.group
					groupDestinationPicker(c, &p.group, groups, "选择安装分组", a.busy).Grow(1).MinWidth(0)
					if previous != p.group {
						p.err = ""
					}
				})
			}
			if cached {
				settingsRow(c, "加载器", func() { muted(c, "原版") })
			} else if a.loaderFields(c, &p.loaderSelection, launcher.Instance{}, false) {
				p.err = ""
			}
			layout.Divider(c, layout.DividerProps{})
			directory, ready := a.installDestinationPreview(c, p, loader)
			targetReady = ready
			a.installSpacePreview(c, p, directory)
			a.releaseJavaDetails(c, p, versions, cached)
			a.installCacheDetails(c, p, loader)
			if !cached && a.catalogErr != "" {
				feedback.Alert(c, a.catalogErr, feedback.AlertProps{Type: feedback.Warning})
			}
			if !cached && a.catalogSaveErr != "" {
				feedback.Alert(c, a.catalogSaveErr, feedback.AlertProps{Type: feedback.Warning})
			}
			if p.err != "" {
				feedback.Alert(c, p.err, feedback.AlertProps{Type: feedback.Error})
			}
		})
	}).Grow(1).MinHeight(0)
	groupMissing := p.group != "" && !slices.Contains(groups, p.group)
	if groupMissing {
		feedback.Alert(c, i18n.Text("instance.group_missing"), feedback.AlertProps{Type: feedback.Warning})
	}
	layout.Divider(c, layout.DividerProps{})
	ui.Row(c).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() {
		locked := a.busy || a.running != nil || groupMissing || !targetReady || (!cached && !p.validLoader())
		if button(c, "仅创建实例", action.Default, locked) {
			a.submitInstall(p, false)
		}
		if button(c, "安装实例", action.Primary, locked || cached) {
			a.submitInstall(p, true)
		}
	})
}

func (a *desktop) installName(version, loader string) string {
	base := version
	if loader != launcher.LoaderVanilla {
		base += " " + loader
	}
	candidate := base
	for n := 2; slices.ContainsFunc(a.state.Instances, func(i launcher.Instance) bool {
		return strings.EqualFold(candidate, i.Name)
	}); n++ {
		candidate = fmt.Sprintf("%s (%d)", base, n)
	}
	return candidate
}

func (a *desktop) installDraftName(p *installView, loader string) string {
	if name := strings.TrimSpace(p.name); name != "" {
		return name
	}
	return a.installName(p.version, loader)
}

func (a *desktop) submitInstall(p *installView, installNow bool) {
	if a.busy || a.running != nil || a.ctx == nil {
		return
	}
	versions, cached := a.installReleases()
	if cached && installNow {
		p.err = "联网刷新游戏版本后可安装。"
		return
	}
	if !strings.Contains(strings.ToLower(p.version), strings.ToLower(strings.TrimSpace(p.query))) ||
		!slices.ContainsFunc(versions, func(v launcher.Version) bool { return v.ID == p.version }) {
		p.err = "请从官方版本列表选择游戏版本"
		return
	}
	if p.group != "" && !slices.Contains(a.state.GroupNames(), p.group) {
		return
	}
	loader, loaderVersion := p.kind(), ""
	if cached {
		loader = launcher.LoaderVanilla
	} else if p.loader != 0 {
		if !p.validLoader() {
			p.err = "请选择兼容的 " + p.kind() + " 版本"
			return
		}
		loaderVersion = p.loaderVersion
	}
	next, instance, err := newLoaderInstallation(a.state, a.installDraftName(p, loader), p.version, loader, loaderVersion)
	if err != nil {
		p.err = err.Error()
		return
	}
	if p.group != "" {
		next.InstanceGroups[instance.ID] = p.group
	}
	p.err = ""
	a.save(next, instance.ID, func(err error) {
		if err != nil {
			p.err, p.directory = err.Error(), nil
			a.message("无法保存新实例", err)
			return
		}
		p.name, p.group = "", ""
		if a.page == pageDownloads && a.downloadPage == downloadGame {
			a.query, a.instanceFilter = "", 0
			a.inspect(instance)
		}
		if installNow {
			a.play(instance, true)
			return
		}
		status, _ := a.instanceStatus(instance)
		a.message("已创建实例 · "+instance.Name+" · 游戏文件"+status, nil)
	})
}

func newLoaderInstallation(state launcher.State, name, version, loader, loaderVersion string) (launcher.State, launcher.Instance, error) {
	return state.PrepareInstallation(name, version, loader, loaderVersion)
}
