package main

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
)

type releaseJavaPreview struct {
	version string
	major   int
	cancel  context.CancelFunc
}

func (p *installView) clearJavaPreview() {
	if p.java != nil && p.java.cancel != nil {
		p.java.cancel()
	}
	p.java = nil
}

func (a *desktop) releaseJavaDetails(c *ui.Context, p *installView, versions []launcher.Version, cached bool) {
	preview := p.java
	loading := preview != nil && preview.cancel != nil
	ui.Row(c).MinHeight(38).Gap(8).AlignItems(ui.Center).Children(func() {
		if button(c, "查看 Java 要求", action.Default, loading || a.ctx == nil) {
			release := launcher.Version{ID: p.version}
			if !cached {
				if index := slices.IndexFunc(versions, func(v launcher.Version) bool { return v.ID == p.version }); index >= 0 {
					release = versions[index]
				}
			}
			a.readReleaseJava(p, release)
		}
		switch {
		case loading:
			muted(c, "读取原版元数据…").FontSize(13)
		case preview != nil && preview.major > 0:
			muted(c, fmt.Sprintf("原版需要 Java %d", preview.major)).FontSize(13)
		case preview != nil:
			muted(c, "原版 Java 要求未知").FontSize(13)
		}
	})
	if preview != nil && !loading {
		muted(c, "加载器可能有额外要求，实际使用的 Java 依启动设置而定。").FontSize(12)
	}
}

func (a *desktop) readReleaseJava(p *installView, release launcher.Version) {
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	request := &releaseJavaPreview{version: p.version, cancel: cancel}
	p.java = request
	game := a.game
	go func() {
		defer cancel()
		major, _ := game.ReleaseJavaMajor(ctx, release)
		a.post(func() {
			if p.java == request && p.version == request.version {
				request.major, request.cancel = major, nil
			}
		})
	}()
}
