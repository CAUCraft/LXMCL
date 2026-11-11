package main

import (
	"context"
	"fmt"
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
)

type packBrowser struct {
	kind                  launcher.ContentKind
	target                launcher.Instance
	filter                launcher.ResourceFilter
	query, applied, order string
	page                  int
	result                launcher.PackSearch
	searched, searchBusy  bool
	err                   string
	packSelection
	parents         []packSelection
	dependencyNames *dependencyNames
	request         uint64
	cancel          context.CancelFunc
	scroll          ui.ScrollState
}

type packSelection struct {
	project             launcher.PackProject
	versions            []launcher.PackVersion
	version, versionErr string
	versionBusy         bool
	detailKind          launcher.ContentKind
	dependency          *launcher.ModDependency
}

func (a *desktop) browserFor(kind launcher.ContentKind) *packBrowser {
	if a.packs == nil {
		a.packs = make(map[launcher.ContentKind]*packBrowser)
	}
	p := a.packs[kind]
	if p == nil {
		p = &packBrowser{kind: kind, order: "downloads"}
		a.packs[kind] = p
	}
	return p
}

func (p *packBrowser) setTarget(i launcher.Instance) {
	if p.target == i {
		return
	}
	p.target = i
	p.resetCatalog()
}

func (p *packBrowser) resetCatalog() {
	p.abort()
	p.searched, p.searchBusy, p.err = false, false, ""
	p.result, p.page = launcher.PackSearch{}, 0
	p.clearProject()
}

func (p *packBrowser) abort() {
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.request++
}

func (p *packBrowser) clearProject() {
	p.packSelection = packSelection{}
	p.parents = nil
	p.stopDependencyNames()
}

func (a *desktop) openResourceDownloads(i launcher.Instance, kind launcher.ContentKind) {
	p := a.browserFor(kind)
	if kind.Supports(i) {
		p.setTarget(i)
	}
	a.page, a.downloadPage = pageDownloads, downloadTab(kind)
}

func (a *desktop) searchPacks(p *packBrowser, page int) {
	if a.ctx == nil || p.target.ID != 0 && !p.kind.Supports(p.target) {
		return
	}
	p.abort()
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	p.cancel = cancel
	p.clearProject()
	p.searched, p.searchBusy, p.err, p.page = true, true, "", page
	p.result, p.scroll.Y = launcher.PackSearch{}, 0
	request, target, filter, kind, query, order := p.request, p.target, p.filter, p.kind, p.applied, p.order
	go func() {
		defer cancel()
		var result launcher.PackSearch
		var err error
		if target.ID != 0 {
			result, err = launcher.SearchModrinth(ctx, target, kind, query, order, page)
		} else {
			result, err = launcher.SearchResourceCatalog(ctx, filter, kind, query, order, page)
		}
		a.post(func() {
			if p.request != request {
				return
			}
			p.searchBusy, p.result = false, result
			if err != nil {
				p.err = err.Error()
			}
		})
	}()
}

func (a *desktop) applyPackSearch(p *packBrowser) {
	p.applied = strings.TrimSpace(p.query)
	a.searchPacks(p, 0)
}

func (a *desktop) browsePack(p *packBrowser, project launcher.PackProject) {
	if a.ctx == nil || p.target.ID != 0 && !p.kind.Supports(p.target) {
		return
	}
	p.abort()
	p.clearProject()
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	p.cancel = cancel
	p.project, p.version, p.versions, p.versionErr, p.versionBusy = project, "", nil, "", true
	request, target, filter, kind := p.request, p.target, p.filter, p.kind
	go func() {
		defer cancel()
		var versions []launcher.PackVersion
		var err error
		if target.ID != 0 {
			versions, err = launcher.ModrinthVersions(ctx, project.ID, target, kind)
		} else {
			versions, err = launcher.ResourceCatalogVersions(ctx, project.ID, filter, kind)
		}
		a.post(func() {
			if p.request != request {
				return
			}
			p.versionBusy, p.versions = false, versions
			if err != nil {
				p.versionErr = err.Error()
			} else {
				p.selectDefaultVersion()
			}
		})
	}()
}

func (p *packBrowser) selectDefaultVersion() {
	if len(p.versions) == 0 {
		return
	}
	p.version = p.versions[0].ID
	if n := slices.IndexFunc(p.versions, func(v launcher.PackVersion) bool { return v.Type == "release" }); n >= 0 {
		p.version = p.versions[n].ID
	}
}

func (p *packBrowser) selectedVersion() launcher.PackVersion {
	for _, v := range p.versions {
		if v.ID == p.version {
			return v
		}
	}
	return launcher.PackVersion{}
}

func (p *packBrowser) selectionKind() launcher.ContentKind {
	if p.detailKind != "" {
		return p.detailKind
	}
	return p.kind
}

func (a *desktop) resourceDownloads(c *ui.Context, p *packBrowser) {
	if p.target.ID != 0 {
		if current, ok := a.state.Find(p.target.ID); ok && p.kind.Supports(current) {
			p.refreshTarget(current)
		} else {
			p.setTarget(launcher.Instance{})
		}
	}
	label := contentLabel(p.kind)
	a.resourceTargetBar(c, p)
	layout.Divider(c, layout.DividerProps{})
	if !p.searched {
		a.applyPackSearch(p)
	}
	a.packSearchBar(c, p, label)
	w, _ := c.Size()
	splitWorkspace(c, catalogListWidth(w), func() {
		a.packResults(c, p)
	}, func() {
		a.packDetails(c.Key(p.project.ID), p)
	})
}

func (p *packBrowser) refreshTarget(current launcher.Instance) {
	old := p.target
	if current.ID != old.ID || current.Revision != old.Revision || current.Version != old.Version ||
		current.Loader != old.Loader || current.LoaderVersion != old.LoaderVersion {
		p.setTarget(current)
	} else {
		p.target = current
	}
}

func (a *desktop) resourceTargetBar(c *ui.Context, p *packBrowser) {
	if p.target.ID != 0 {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			display.Text(c, "安装到 "+p.target.Name, display.TextProps{Strong: true, Rows: 1}).MinWidth(0).Ellipsis("…")
			muted(c, p.target.Version+" · "+p.target.LoaderName()).SingleLine().Ellipsis("…")
			ui.Spacer(c).Grow(1)
			if button(c, "管理已安装", action.Default, false) {
				target, kind := p.target, p.selectionKind()
				if !kind.Supports(target) {
					kind = p.kind
				}
				a.inspect(target)
				a.detailPage = detailContent
				tab := 0
				switch kind {
				case launcher.Mods:
					tab = 1
				case launcher.ShaderPacks:
					tab = 2
				}
				a.content = contentView{id: target.ID, revision: target.Revision, tab: tab}
				a.refreshContent(target)
			}
			if button(c, "独立下载", action.Text, false) {
				p.setTarget(launcher.Instance{})
			}
		})
		return
	}
	if len(a.releases) == 0 && !a.catalogBusy && a.catalogErr == "" {
		a.refreshCatalog()
	}
	options := []field.SelectOption{{Value: "", Label: "全部游戏版本"}}
	for _, release := range a.releases {
		options = append(options, field.SelectOption{Value: release.ID, Label: release.ID})
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		before := p.filter
		catalogPicker(c.Key("game-filter"), &p.filter.Game, options, "筛选游戏版本", "", false).Width(152).Shrink(0)
		if p.kind == launcher.Mods {
			ui.Column(c).Width(126).Shrink(0).Children(func() {
				field.Select(c, &p.filter.Loader, field.SelectProps{Options: []field.SelectOption{
					{Value: "", Label: "全部加载器"}, {Value: "fabric", Label: "Fabric"}, {Value: "forge", Label: "Forge"},
					{Value: "neoforge", Label: "NeoForge"}, {Value: "quilt", Label: "Quilt"},
				}}).Label("筛选加载器")
			})
		}
		if p.filter != before {
			p.resetCatalog()
			c.Invalidate()
		}
		ui.Spacer(c).Grow(1)
		muted(c, "Modrinth").FontSize(12)
	})
}

func (a *desktop) packSearchBar(c *ui.Context, p *packBrowser, label string) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		submit := false
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			submit = field.Input(c, &p.query, field.InputProps{Placeholder: "搜索" + label, Prefix: lucide.Search()}).
				Label("搜索" + label).Submitted()
		})
		if button(c, "搜索", action.Default, false) || submit {
			a.applyPackSearch(p)
		}
		before := p.order
		ui.Column(c).Width(116).Shrink(0).Children(func() {
			field.Select(c, &p.order, field.SelectProps{Options: []field.SelectOption{
				{Value: "downloads", Label: "下载量"},
				{Value: "relevance", Label: "相关性"},
				{Value: "updated", Label: "最近更新"},
			}}).Label("搜索排序")
		})
		if p.order != before {
			a.applyPackSearch(p)
		}
	})
}

func (a *desktop) packResults(c *ui.Context, p *packBrowser) {
	switch {
	case p.searchBusy:
		flightLoading(c, "正在搜索…")
	case p.err != "":
		feedback.Alert(c, p.err, feedback.AlertProps{Type: feedback.Error})
		if button(c, "重试搜索", action.Default, false) {
			a.searchPacks(p, p.page)
		}
	case len(p.result.Hits) == 0:
		muted(c, "没有匹配项目").Padding(6)
	default:
		items := make([]display.ListItem, 0, len(p.result.Hits))
		for _, hit := range p.result.Hits {
			items = append(items, display.ListItem{Key: hit.ID, Label: hit.Title,
				Description: hit.Author, Avatar: func() { a.projectIcon(c, "resource-"+hit.ID, hit.IconURL, downloadTabs[downloadTab(p.kind)].Icon, 28) }})
		}
		layout.ScrollArea(c, layout.ScrollAreaProps{State: &p.scroll}, func() {
			if id := display.List(c, display.ListProps{Items: items, Active: p.project.ID, Compact: true}); id != "" && id != p.project.ID {
				if n := slices.IndexFunc(p.result.Hits, func(v launcher.PackProject) bool { return v.ID == id }); n >= 0 {
					a.browsePack(p, p.result.Hits[n])
					c.Invalidate()
				}
			}
		}).Grow(1).MinHeight(0)
		layout.Divider(c, layout.DividerProps{})
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if icon(c, lucide.ChevronLeft(), "上一页", p.page == 0) {
				a.searchPacks(p, p.page-1)
			}
			muted(c, fmt.Sprintf("%d 页 · %d 项", p.page+1, p.result.Total)).FontSize(12).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			if icon(c, lucide.ChevronRight(), "下一页", (p.page+1)*launcher.PackPageSize >= p.result.Total || p.page >= 500) {
				a.searchPacks(p, p.page+1)
			}
		})
	}
}

func (a *desktop) packDetails(c *ui.Context, p *packBrowser) {
	if p.project.ID == "" && p.dependency == nil {
		ui.Column(c).Grow(1).Center().Children(func() {
			muted(c, "选择一个项目查看可下载版本")
		})
		return
	}
	kind := p.selectionKind()
	back := false
	a.projectHeading(c, "resource-heading", p.project.Title, p.project.IconURL, downloadTabs[downloadTab(kind)].Icon, func() {
		if len(p.parents) > 0 && button(c, "返回上一级", action.Text, false) {
			back = true
		}
		if site := p.project.PageURL(kind); site != "" && (p.dependency == nil || p.detailKind != "") &&
			icon(c, lucide.ExternalLink(), "在 Modrinth 查看项目", false) {
			c.OpenURL(site)
		}
	})
	if back {
		p.backDependency()
		c.Invalidate()
		return
	}
	if p.project.Author != "" || p.project.Downloads > 0 {
		byline := p.project.Author
		if p.project.Downloads > 0 {
			count := fmt.Sprintf("%d 次下载", p.project.Downloads)
			if byline != "" {
				byline += " · "
			}
			byline += count
		}
		muted(c, byline).FontSize(12)
	}
	if p.project.Description != "" {
		display.Text(c, p.project.Description, display.TextProps{Type: display.TextSecondary, FontSize: 13, Rows: 2}).Ellipsis("…")
	}
	layout.Divider(c, layout.DividerProps{})
	switch {
	case p.versionBusy:
		flightLoading(c, "读取可下载版本…")
	case p.versionErr != "":
		feedback.Alert(c, p.versionErr, feedback.AlertProps{Type: feedback.Error})
		if button(c, "重试读取版本", action.Default, false) {
			if p.dependency != nil {
				a.loadPackDependency(p)
			} else {
				a.browsePack(p, p.project)
			}
		}
	case len(p.versions) == 0:
		muted(c, "没有符合当前筛选条件的文件")
	default:
		a.packVersions(c, p, kind)
	}
}

func (a *desktop) packVersions(c *ui.Context, p *packBrowser, kind launcher.ContentKind) {
	display.Text(c, "文件版本", display.TextProps{Strong: true, FontSize: 13})
	items := make([]display.ListItem, 0, len(p.versions))
	for _, v := range p.versions {
		file, _ := v.DownloadFile(kind)
		var description []string
		if !v.Published.IsZero() {
			description = append(description, v.Published.Local().Format("2006-01-02"))
		}
		description = append(description, fmt.Sprintf("%.1f MiB", float64(file.Size)/(1<<20)), file.Filename)
		if p.target.ID == 0 {
			if len(v.GameVersions) > 0 {
				description = append([]string{strings.Join(v.GameVersions[:min(3, len(v.GameVersions))], " / ")}, description...)
			}
			if kind == launcher.Mods && len(v.Loaders) > 0 {
				description = append([]string{strings.Join(v.Loaders, " / ")}, description...)
			}
		}
		channel := "正式版"
		switch v.Type {
		case "beta":
			channel = "Beta"
		case "alpha":
			channel = "Alpha"
		}
		items = append(items, display.ListItem{Key: v.ID, Label: v.Number, Description: strings.Join(description, " · "), Icon: lucide.FileArchive(), Extra: channel})
	}
	layout.ScrollArea(c.Key("files"), layout.ScrollAreaProps{}, func() {
		if key := display.List(c, display.ListProps{Items: items, Active: p.version, Compact: true}); key != "" {
			p.version = key
			c.Invalidate()
		}
	}).Grow(1).MinHeight(0)
	layout.Divider(c, layout.DividerProps{})
	version := p.selectedVersion()
	if a.packDependencies(c.Key("dependencies-"+version.ID), p, version) {
		return
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		note, labelAction := "保存所选文件", "保存到文件夹…"
		locked := a.busy || a.running != nil || a.pickerBusy || version.ID == ""
		if p.target.ID != 0 {
			note, labelAction = "安装到 "+p.target.Name, "安装"+contentLabel(kind)
		}
		muted(c, note).FontSize(12).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		if button(c, labelAction, action.Primary, locked) {
			r := resourceTask{Kind: string(kind), Project: p.project.ID, Version: version.ID, Title: p.project.Title}
			if p.target.ID != 0 {
				a.downloadResource(p.target, r, version)
			} else {
				a.pickDownloadFolder(func(folder string) { a.saveResourceFile(folder, r, version) })
			}
		}
	})
	if kind == launcher.ShaderPacks {
		muted(c, "在游戏中使用光影需要安装 Iris 等兼容模组。").FontSize(12)
	}
}

func (a *desktop) saveResourceFile(folder string, r resourceTask, version launcher.PackVersion) {
	game := a.game
	kind := launcher.ContentKind(r.Kind)
	a.runFolderDownload(folder, "保存"+contentLabel(kind), r.Title, &r, func(ctx context.Context, report func(launcher.Progress)) error {
		if version.ID == "" {
			var err error
			version, err = launcher.ModrinthVersion(ctx, r.Project, r.Version)
			if err != nil {
				return err
			}
		}
		return game.DownloadResourceFile(ctx, kind, version, folder, report)
	})
}

func (a *desktop) downloadResource(i launcher.Instance, r resourceTask, version launcher.PackVersion) {
	kind := launcher.ContentKind(r.Kind)
	a.runContentTask(i, "安装"+contentLabel(kind), &r, func(ctx context.Context, report func(launcher.Progress)) error {
		if version.ID == "" {
			var err error
			version, err = launcher.ModrinthVersion(ctx, r.Project, r.Version)
			if err != nil {
				return err
			}
		}
		return launcher.DownloadModrinth(ctx, i, kind, version, report)
	})
}
