package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

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
	query, version, name                  string
	loader                                int
	loaderGame, loaderKind, loaderVersion string
	loaderVersions                        []launcher.LoaderRelease
	loaderBusy                            bool
	loaderErr, err                        string
	request                               uint64
	loaderCancel                          context.CancelFunc
	scroll                                ui.ScrollState
}

var installLoaders = []string{launcher.LoaderVanilla, launcher.LoaderFabric, launcher.LoaderForge, launcher.LoaderNeoForge, launcher.LoaderQuilt}

func (p *installView) kind() string { return installLoaders[p.loader] }

func (p *installView) currentLoaderCatalog() bool {
	return p.loaderGame == p.version && p.loaderKind == p.kind()
}

func (p *installView) cancelLoaders() {
	if p.loaderCancel != nil {
		p.loaderCancel()
		p.loaderCancel = nil
	}
	p.request++
	p.loaderBusy = false
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

func (a *desktop) loadInstallLoaders() {
	p := &a.install
	if p.version == "" || p.loader == 0 || a.ctx == nil {
		return
	}
	p.cancelLoaders()
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	p.loaderCancel = cancel
	request, game, kind := p.request, p.version, p.kind()
	p.loaderGame, p.loaderKind, p.loaderVersion, p.loaderErr, p.loaderVersions, p.loaderBusy = game, kind, "", "", nil, true
	go func() {
		defer cancel()
		versions, err := launcher.LoaderVersions(ctx, game, kind)
		a.post(func() {
			if p.request != request {
				return
			}
			p.loaderBusy, p.loaderVersions = false, versions
			if err != nil {
				p.loaderErr = err.Error()
			} else if len(versions) != 0 {
				p.loaderVersion = versions[0].Version
			}
		})
	}()
}

func (a *desktop) installPage(c *ui.Context) {
	p := &a.install
	if len(a.releases) == 0 && a.catalogErr == "" {
		a.refreshCatalog()
	}
	if p.version == "" && len(a.releases) != 0 {
		p.version = a.releases[0].ID
	}
	w, _ := c.Size()
	splitWorkspace(c, catalogListWidth(w), func() {
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			field.SearchBar(c, &p.query, field.SearchBarProps{Placeholder: "搜索游戏版本"}).Label("搜索游戏版本").Grow(1).MinWidth(0)
			if icon(c, lucide.RefreshCw(), "刷新游戏版本", a.catalogBusy) {
				a.refreshCatalog()
			}
		})
		a.installVersions(c, p)
	}, func() {
		if p.version == "" {
			if a.catalogErr != "" {
				feedback.Alert(c, a.catalogErr, feedback.AlertProps{Type: feedback.Error})
				ui.Row(c).Children(func() {
					if button(c, "重试读取版本", action.Default, a.catalogBusy) {
						a.refreshCatalog()
					}
				})
			} else if a.catalogBusy {
				flightLoading(c, "正在读取官方版本…")
			}
			return
		}
		a.installDetails(c, p)
	})
}

func (a *desktop) installVersions(c *ui.Context, p *installView) {
	q := strings.ToLower(strings.TrimSpace(p.query))
	items := make([]display.ListItem, 0, len(a.releases))
	for idx, v := range a.releases {
		if q != "" && !strings.Contains(strings.ToLower(v.ID), q) {
			continue
		}
		item := display.ListItem{Key: v.ID, Label: v.ID, Icon: lucide.Package()}
		if idx == 0 {
			item.Extra = "最新正式版"
		}
		items = append(items, item)
	}
	if len(items) == 0 && !a.catalogBusy && a.catalogErr == "" && len(a.releases) > 0 {
		muted(c, "没有匹配的版本").Padding(5)
		return
	}
	layout.ScrollArea(c, layout.ScrollAreaProps{State: &p.scroll}, func() {
		if key := display.List(c, display.ListProps{Items: items, Active: p.version, Compact: true}); key != "" && key != p.version {
			p.version, p.err = key, ""
			c.Invalidate()
		}
	}).Grow(1).MinHeight(0)
}

func (a *desktop) installDetails(c *ui.Context, p *installView) {
	heading(c, "Minecraft "+p.version)
	layout.Divider(c, layout.DividerProps{})
	layout.ScrollArea(c, layout.ScrollAreaProps{}, func() {
		ui.Column(c).Gap(8).Padding(4, 2).Children(func() {
			settingsRow(c, "实例名称", func() {
				before := p.name
				settingsInput(c, "实例名称", &p.name, a.installName(p), 0, false)
				if before != p.name {
					p.err = ""
				}
			})
			settingsRow(c, "加载器", func() {
				before := p.loader
				action.Segmented(c, &p.loader, []string{"原版", "Fabric", "Forge", "NeoForge", "Quilt"}, action.SegmentedProps{Size: theme.Small}).Label("安装加载器")
				if before != p.loader {
					p.err = ""
					if p.loader == 0 && p.loaderBusy {
						p.cancelLoaders()
						p.loaderGame = ""
					}
				}
			})
			if p.loader != 0 {
				kind := p.kind()
				if !p.currentLoaderCatalog() {
					a.loadInstallLoaders()
				}
				settingsRow(c, "加载器版本", func() {
					switch {
					case p.loaderBusy:
						flightLoading(c, "读取 "+kind+" 版本…")
					case p.loaderErr != "":
						if button(c, "重试读取 "+kind, action.Default, false) {
							a.loadInstallLoaders()
						}
					case len(p.loaderVersions) == 0:
						muted(c, "此游戏版本没有可用的 "+kind+" 版本").FontSize(12)
					default:
						options := make([]field.SelectOption, 0, len(p.loaderVersions))
						for _, v := range p.loaderVersions {
							label := v.Version
							if v.Preview {
								label += " · 预览"
							}
							options = append(options, field.SelectOption{Value: v.Version, Label: label})
						}
						catalogPicker(c.Key(kind), &p.loaderVersion, options, kind+" 版本", "", false).Grow(1).MinWidth(0).MaxWidth(254)
					}
				})
				if p.loaderErr != "" {
					feedback.Alert(c, p.loaderErr, feedback.AlertProps{Type: feedback.Warning})
				}
			}
			layout.Divider(c, layout.DividerProps{})
			dir := filepath.Join(a.state.Settings.GameRoot, fmt.Sprintf("instance-%d", a.state.NextID))
			settingsRow(c, "游戏目录", func() {
				muted(c, dir).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Selectable()
				action.CopyButton(c, dir, action.CopyButtonProps{}).Label("复制新实例目录")
			})
			if a.catalogErr != "" {
				feedback.Alert(c, a.catalogErr, feedback.AlertProps{Type: feedback.Warning})
			}
			if p.err != "" {
				feedback.Alert(c, p.err, feedback.AlertProps{Type: feedback.Error})
			}
		})
	}).Grow(1).MinHeight(0)
	layout.Divider(c, layout.DividerProps{})
	ui.Row(c).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() {
		locked := a.busy || a.running != nil || (p.loader != 0 && (p.loaderBusy || p.loaderVersion == "" || !p.currentLoaderCatalog()))
		if button(c, "安装实例", action.Primary, locked) {
			a.submitInstall(p, a.installName(p))
		}
	})
}

func (a *desktop) installName(p *installView) string {
	base := p.version
	if p.loader != 0 {
		base += " " + p.kind()
	}
	candidate := base
	for n := 2; slices.ContainsFunc(a.state.Instances, func(i launcher.Instance) bool {
		return strings.EqualFold(candidate, i.Name)
	}); n++ {
		candidate = fmt.Sprintf("%s (%d)", base, n)
	}
	return candidate
}

func (a *desktop) submitInstall(p *installView, defaultName string) {
	if a.busy || a.running != nil || a.ctx == nil {
		return
	}
	if !slices.ContainsFunc(a.releases, func(v launcher.Version) bool { return v.ID == p.version }) {
		p.err = "请从官方版本列表选择要安装的版本"
		return
	}
	name, loaderVersion := strings.TrimSpace(p.name), ""
	if name == "" {
		name = defaultName
	}
	if p.loader != 0 {
		if !p.currentLoaderCatalog() || !slices.ContainsFunc(p.loaderVersions, func(v launcher.LoaderRelease) bool { return v.Version == p.loaderVersion }) {
			p.err = "请选择兼容的 " + p.kind() + " 版本"
			return
		}
		loaderVersion = p.loaderVersion
	}
	next, instance, err := newLoaderInstallation(a.state, name, p.version, p.kind(), loaderVersion)
	if err != nil {
		p.err = err.Error()
		return
	}
	p.err = ""
	a.save(next, 0, func(err error) {
		if err != nil {
			p.err = err.Error()
			a.message("无法保存新实例", err)
			return
		}
		p.name = ""
		if a.page == pageDownloads && a.downloadPage == downloadGame {
			a.query, a.instanceFilter = "", 0
			a.inspect(instance)
		}
		a.play(instance, true)
	})
}

func newLoaderInstallation(state launcher.State, name, version, loader, loaderVersion string) (launcher.State, launcher.Instance, error) {
	next := state.Clone()
	if err := next.Create(name, version); err != nil {
		return state, launcher.Instance{}, err
	}
	i, ok := next.Find(next.Selected)
	if !ok {
		return state, launcher.Instance{}, errors.New("无法取得新实例")
	}
	if loader != launcher.LoaderVanilla {
		if err := next.ConfigureModLoader(i.ID, i.Revision, loader, loaderVersion); err != nil {
			return state, launcher.Instance{}, err
		}
		i, _ = next.Find(i.ID)
	}
	if _, ok := state.Find(state.Selected); ok {
		next.Selected = state.Selected
	}
	return next, i, nil
}
