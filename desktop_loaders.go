package main

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

type loaderSelection struct {
	version                               string // Minecraft version whose catalog is being shown.
	loader                                int
	loaderGame, loaderKind, loaderVersion string
	loaderVersions                        []launcher.LoaderRelease
	loaderBusy                            bool
	loaderErr                             string
	request                               uint64
	loaderCancel                          context.CancelFunc
}

var installLoaders = []string{launcher.LoaderVanilla, launcher.LoaderFabric, launcher.LoaderForge, launcher.LoaderNeoForge, launcher.LoaderQuilt}

func (p *loaderSelection) kind() string { return installLoaders[p.loader] }

func (p *loaderSelection) currentLoaderCatalog() bool {
	return p.loaderGame == p.version && p.loaderKind == p.kind()
}

func (p *loaderSelection) cancelLoaders() {
	if p.loaderCancel != nil {
		p.loaderCancel()
		p.loaderCancel = nil
	}
	p.request++
	p.loaderBusy = false
}

func (a *desktop) loadLoaderCatalog(p *loaderSelection, current launcher.Instance) {
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
			p.loaderBusy, p.loaderVersions, p.loaderCancel = false, versions, nil
			if err != nil {
				p.loaderErr = err.Error()
			} else if strings.EqualFold(kind, current.Loader) && current.LoaderVersion != "" {
				p.loaderVersion = current.LoaderVersion
			} else if len(versions) != 0 {
				p.loaderVersion = versions[0].Version
			}
		})
	}()
}

func (p *loaderSelection) validLoader() bool {
	return p.loader == 0 || (!p.loaderBusy && p.currentLoaderCatalog() &&
		slices.ContainsFunc(p.loaderVersions, func(v launcher.LoaderRelease) bool { return v.Version == p.loaderVersion }))
}

func (a *desktop) loaderFields(c *ui.Context, p *loaderSelection, current launcher.Instance, disabled bool) bool {
	before, version := p.loader, p.loaderVersion
	settingsRow(c, "加载器", func() {
		action.Segmented(c, &p.loader, []string{"原版", "Fabric", "Forge", "NeoForge", "Quilt"},
			action.SegmentedProps{Size: theme.Small}).Label("加载器类型").Disabled(disabled)
	})
	if p.loader == 0 {
		if before != p.loader {
			p.cancelLoaders()
			p.loaderGame, p.loaderVersion = "", ""
		}
		return before != p.loader
	}
	kind := p.kind()
	if !disabled && !p.currentLoaderCatalog() {
		a.loadLoaderCatalog(p, current)
	}
	settingsRow(c, "加载器版本", func() {
		switch {
		case p.loaderBusy:
			flightLoading(c, "读取 "+kind+" 版本…")
		case p.loaderErr != "":
			if button(c, "重试读取 "+kind, action.Default, disabled) {
				a.loadLoaderCatalog(p, current)
			}
		case len(p.loaderVersions) == 0:
			muted(c, "此游戏版本没有可用的 "+kind+" 版本").FontSize(12)
		default:
			options := make([]field.SelectOption, 0, len(p.loaderVersions))
			pinned := strings.EqualFold(kind, current.Loader) && current.LoaderVersion != ""
			for _, v := range p.loaderVersions {
				if v.Version == current.LoaderVersion {
					pinned = false
				}
				label := v.Version
				if v.Preview {
					label += " · 预览"
				}
				options = append(options, field.SelectOption{Value: v.Version, Label: label})
			}
			// Keep a pinned version visible even when upstream delists it.
			if pinned {
				options = append(options, field.SelectOption{Value: current.LoaderVersion, Label: current.LoaderVersion + " · 当前"})
			}
			catalogPicker(c.Key(kind), &p.loaderVersion, options, kind+" 版本", "", disabled).Grow(1).MinWidth(0).MaxWidth(254)
		}
	})
	if p.loaderErr != "" {
		feedback.Alert(c, p.loaderErr, feedback.AlertProps{Type: feedback.Warning})
	}
	return before != p.loader || version != p.loaderVersion
}
