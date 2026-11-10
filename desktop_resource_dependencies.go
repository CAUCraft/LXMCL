package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/overlay"
)

type dependencyNames struct {
	version  string
	projects map[string]launcher.PackProject
	cancel   context.CancelFunc
}

func (p *packBrowser) stopDependencyNames() {
	if p.dependencyNames != nil && p.dependencyNames.cancel != nil {
		p.dependencyNames.cancel()
	}
	p.dependencyNames = nil
}

func (a *desktop) fetchDependencyNames(p *packBrowser, version string, deps []launcher.ModDependency) *dependencyNames {
	if p.dependencyNames != nil && p.dependencyNames.version == version {
		return p.dependencyNames
	}
	p.stopDependencyNames()
	names := &dependencyNames{version: version}
	p.dependencyNames = names
	if a.ctx != nil {
		ctx, cancel := context.WithTimeout(a.ctx, 12*time.Second)
		names.cancel = cancel
		go func() {
			defer cancel()
			projects, err := launcher.ResourceDependencyProjects(ctx, deps)
			if err == nil {
				a.post(func() {
					if p.dependencyNames == names {
						names.projects = projects
					}
				})
			}
		}()
	}
	return names
}

func (a *desktop) packDependencies(c *ui.Context, p *packBrowser, version launcher.PackVersion) bool {
	deps := version.RequiredDependencies()
	if len(deps) == 0 {
		p.stopDependencyNames()
		return false
	}
	jumped := false
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, lucide.Info()).FontSize(15).Shrink(0)
		muted(c, "此版本需要前置，请按需另行下载。").FontSize(12).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		anchor := action.Button(c, fmt.Sprintf("查看前置 (%d)", len(deps)), action.ButtonProps{Variant: action.Default}).Shrink(0)
		open := ui.Local(anchor, "dependency-list", func() bool { return false })
		overlay.Popover(c, anchor, open, overlay.PopoverProps{Placement: overlay.Top}, func() {
			names := a.fetchDependencyNames(p, version.ID, deps)
			ui.Column(c).Width(284).Gap(6).Children(func() {
				display.Text(c, "前置依赖", display.TextProps{Strong: true})
				items := make([]display.ListItem, 0, len(deps))
				for n, dep := range deps {
					project := names.projects[dep.ProjectID]
					title := dependencyLabel(dep, project)
					detail := "查看兼容版本"
					if !dep.Linkable() {
						detail = "来源未提供下载入口"
					} else if dep.VersionID != "" {
						detail = "作者指定版本"
					}
					items = append(items, display.ListItem{Key: strconv.Itoa(n), Label: title, Description: detail,
						Icon: lucide.Link(), Disabled: !dep.Linkable() || len(p.parents) >= 16})
				}
				layout.ScrollArea(c, layout.ScrollAreaProps{}, func() {
					key := display.List(c, display.ListProps{Items: items, Compact: true})
					if n, err := strconv.Atoi(key); err == nil && n >= 0 && n < len(deps) {
						*open = false
						a.openPackDependency(p, deps[n], names.projects[deps[n].ProjectID])
						jumped = true
						c.Invalidate()
					}
				}).Height(min(180, float32(46*len(items)))).Clip()
			})
		})
	})
	return jumped
}

func dependencyLabel(dep launcher.ModDependency, project launcher.PackProject) string {
	switch {
	case project.Title != "":
		return project.Title
	case dep.Filename != "":
		return dep.Filename
	case dep.ProjectID != "":
		return "前置项目 " + dep.ProjectID
	case dep.VersionID != "":
		return "前置版本 " + dep.VersionID
	default:
		return "未命名的前置"
	}
}

func (a *desktop) openPackDependency(p *packBrowser, dep launcher.ModDependency, project launcher.PackProject) {
	if !dep.Linkable() || len(p.parents) >= 16 {
		return
	}
	p.parents = append(p.parents, p.packSelection)
	p.stopDependencyNames()
	project.ID, project.Title = dep.ProjectID, dependencyLabel(dep, project)
	p.packSelection = packSelection{project: project, dependency: &dep}
	a.loadPackDependency(p)
}

func (p *packBrowser) backDependency() {
	if n := len(p.parents); n > 0 {
		p.abort()
		p.stopDependencyNames()
		p.packSelection = p.parents[n-1]
		p.parents[n-1] = packSelection{}
		p.parents = p.parents[:n-1]
	}
}

func (a *desktop) loadPackDependency(p *packBrowser) {
	if p.dependency == nil {
		return
	}
	p.abort()
	p.versionErr, p.versionBusy = "", true
	if a.ctx == nil {
		return
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	p.cancel = cancel
	request, dep, target, filter := p.request, *p.dependency, p.target, p.filter
	go func() {
		defer cancel()
		catalog, err := launcher.OpenResourceDependency(ctx, dep, target, filter)
		a.post(func() {
			if p.request != request {
				return
			}
			p.versionBusy = false
			if catalog.Project.ID != "" {
				p.project, p.detailKind = catalog.Project, catalog.Kind
			}
			if err != nil {
				p.versionErr = err.Error()
				return
			}
			p.versions = catalog.Versions
			p.selectDefaultVersion()
		})
	}()
}
