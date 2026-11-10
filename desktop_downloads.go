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
		return launcher.FabricMods
	case downloadShaders:
		return launcher.ShaderPacks
	default:
		return launcher.ResourcePacks
	}
}

func downloadTab(kind launcher.ContentKind) int {
	switch kind {
	case launcher.FabricMods:
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
		contextTabs(c, &a.downloadPage, downloadTabs)
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
}

type installView struct {
	query, version, name string
	loader               int
	fabricGame, fabric   string
	fabricVersions       []string
	fabricBusy           bool
	fabricErr, err       string
	request              uint64
	fabricCancel         context.CancelFunc
	scroll               ui.ScrollState
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

func (a *desktop) loadInstallFabric() {
	p := &a.install
	if p.version == "" || a.ctx == nil {
		return
	}
	if p.fabricCancel != nil {
		p.fabricCancel()
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	p.fabricCancel = cancel
	p.request++
	request, game := p.request, p.version
	p.fabricGame, p.fabric, p.fabricErr, p.fabricVersions, p.fabricBusy = game, "", "", nil, true
	go func() {
		defer cancel()
		versions, err := launcher.FabricVersions(ctx, game)
		a.post(func() {
			if p.request != request {
				return
			}
			p.fabricBusy, p.fabricVersions = false, versions
			if err != nil {
				p.fabricErr = err.Error()
			} else if len(versions) != 0 {
				p.fabric = versions[0]
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
	splitWorkspace(c, min(282, max(210, w*0.25)), func() {
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
	defaultName := a.installName(p)
	heading(c, "Minecraft "+p.version)
	layout.Divider(c, layout.DividerProps{})
	layout.ScrollArea(c, layout.ScrollAreaProps{}, func() {
		ui.Column(c).Gap(8).Padding(4, 2).Children(func() {
			settingsRow(c, "实例名称", func() {
				before := p.name
				settingsInput(c, "实例名称", &p.name, defaultName, 0, false)
				if before != p.name {
					p.err = ""
				}
			})
			settingsRow(c, "加载器", func() {
				before := p.loader
				action.Segmented(c, &p.loader, []string{"原版", "Fabric"}, action.SegmentedProps{Size: theme.Small}).Label("安装加载器")
				if before != p.loader {
					p.err = ""
				}
			})
			if p.loader == 1 {
				if p.fabricGame != p.version {
					a.loadInstallFabric()
				}
				settingsRow(c, "Fabric 版本", func() {
					switch {
					case p.fabricBusy:
						flightLoading(c, "读取 Fabric 版本…")
					case p.fabricErr != "":
						if button(c, "重试读取 Fabric", action.Default, false) {
							a.loadInstallFabric()
						}
					case len(p.fabricVersions) == 0:
						muted(c, "此游戏版本没有可用的 Fabric 稳定版").FontSize(12)
					default:
						options := make([]field.SelectOption, 0, len(p.fabricVersions))
						for _, v := range p.fabricVersions {
							options = append(options, field.SelectOption{Value: v, Label: v})
						}
						catalogPicker(c, &p.fabric, options, "Fabric 版本", "", false).Grow(1).MinWidth(0).MaxWidth(224)
					}
				})
				if p.fabricErr != "" {
					feedback.Alert(c, p.fabricErr, feedback.AlertProps{Type: feedback.Warning})
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
		locked := a.busy || a.running != nil || (p.loader == 1 && (p.fabricBusy || p.fabric == "" || p.fabricGame != p.version))
		if button(c, "安装实例", action.Primary, locked) {
			a.submitInstall(p, defaultName)
		}
	})
}

func (a *desktop) installName(p *installView) string {
	base := p.version
	if p.loader == 1 {
		base += " Fabric"
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
	name, fabric := strings.TrimSpace(p.name), ""
	if name == "" {
		name = defaultName
	}
	if p.loader == 1 {
		if p.fabricGame != p.version || !slices.Contains(p.fabricVersions, p.fabric) {
			p.err = "请选择兼容的 Fabric 版本"
			return
		}
		fabric = p.fabric
	}
	next, instance, err := newInstallation(a.state, name, p.version, fabric)
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

func newInstallation(state launcher.State, name, version, fabric string) (launcher.State, launcher.Instance, error) {
	next := state.Clone()
	if err := next.Create(name, version); err != nil {
		return state, launcher.Instance{}, err
	}
	i, ok := next.Find(next.Selected)
	if !ok {
		return state, launcher.Instance{}, errors.New("无法取得新实例")
	}
	if fabric != "" {
		if err := next.ConfigureLoader(i.ID, i.Revision, fabric); err != nil {
			return state, launcher.Instance{}, err
		}
		i, _ = next.Find(i.ID)
	}
	if _, ok := state.Find(state.Selected); ok {
		next.Selected = state.Selected
	}
	return next, i, nil
}
