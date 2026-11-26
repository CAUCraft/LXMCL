package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

type packComparisonView struct {
	base, peer launcher.Instance
	rows       []content.ResourcePackMatch
	cancel     context.CancelFunc
	loading    bool
	err        error
	scroll     ui.ListState

	copying, copied, copyFailed bool
}

func (a *desktop) closePackComparison() {
	if p := a.content.packComparison; p != nil {
		if p.cancel != nil {
			p.cancel()
		}
		a.content.packComparison = nil
	}
}

func (a *desktop) packComparisonInstance(i launcher.Instance) bool {
	current, ok := a.state.Find(i.ID)
	return ok && current == i && a.installed[i.ID] && launcher.ResourcePacks.Supports(i)
}

func (a *desktop) inPackComparison(p *packComparisonView) bool {
	return a.page == pageLibrary && a.detailPage == detailContent && a.inspected == p.base.ID &&
		a.content.kind() == launcher.ResourcePacks && a.packComparisonInstance(p.base)
}

func (a *desktop) readPackComparison(base, peer launcher.Instance) {
	if a.ctx == nil {
		return
	}
	a.closePackComparison()
	ctx, cancel := context.WithCancel(a.ctx)
	p := &packComparisonView{base: base, peer: peer, cancel: cancel, loading: true}
	a.content.packComparison = p
	go func() {
		defer cancel()
		rows, err := content.CompareResourcePacks(ctx, base, peer)
		a.post(func() {
			if a.content.packComparison == p && a.inPackComparison(p) && a.packComparisonInstance(peer) {
				p.rows, p.err, p.loading = rows, err, false
			}
		})
	}()
}

func (a *desktop) packComparisonCopyable(p *packComparisonView) bool {
	return a.content.packComparison == p && !p.loading && p.err == nil && p.peer.ID != 0 &&
		a.inPackComparison(p) && a.packComparisonInstance(p.peer)
}

func (a *desktop) copyPackComparison(c *ui.Context, p *packComparisonView) {
	if p.copying || !a.packComparisonCopyable(p) {
		return
	}
	p.copying, p.copied, p.copyFailed = true, false, false
	c.WriteClipboardThen(packComparisonReport(p.rows), func(err error) {
		if a.packComparisonCopyable(p) {
			p.copying, p.copied, p.copyFailed = false, err == nil, err != nil
		}
	})
}

func (a *desktop) resourcePackComparison(c *ui.Context, p *packComparisonView) {
	if !a.inPackComparison(p) {
		a.closePackComparison()
		c.Invalidate()
		return
	}
	if p.peer.ID != 0 && !a.packComparisonInstance(p.peer) {
		a.closePackComparison()
		p = &packComparisonView{base: p.base, err: errors.New("对照实例已变化，请重新选择。")}
		a.content.packComparison = p
	}
	var back, refresh, copyReport bool
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		back = icon(c, lucide.ArrowLeft(), "返回资源包列表", false)
		display.Text(c, "比对资源包", display.TextProps{Strong: true, FontSize: 16}).Grow(1)
		copyReport = button(c.Key(p), "复制比对清单", action.Default, p.copying || !a.packComparisonCopyable(p))
		refresh = icon(c, lucide.RefreshCw(), "重新比对资源包", p.peer.ID == 0 || p.loading)
	})
	switch {
	case back:
		a.closePackComparison()
		c.Invalidate()
		return
	case refresh:
		a.readPackComparison(p.base, p.peer)
		c.Invalidate()
		return
	}

	var options []field.SelectOption
	for _, i := range a.state.Instances {
		if i.ID != p.base.ID && a.installed[i.ID] && launcher.ResourcePacks.Supports(i) {
			options = append(options, field.SelectOption{Value: strconv.Itoa(i.ID), Label: fmt.Sprintf("#%d · %s", i.ID, i.Name)})
		}
	}
	selected := ""
	if p.peer.ID != 0 {
		selected = strconv.Itoa(p.peer.ID)
	}
	old := selected
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		muted(c, fmt.Sprintf("当前 #%d · %s", p.base.ID, p.base.Name)).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		muted(c, "对照").Shrink(0)
		ui.Column(c).Width(220).Shrink(0).Children(func() {
			catalogPicker(c, &selected, options, "选择对照实例", "", false)
		})
	})
	if selected != old {
		id, _ := strconv.Atoi(selected)
		if peer, ok := a.state.Find(id); ok && id != p.base.ID && a.packComparisonInstance(peer) {
			a.readPackComparison(p.base, peer)
		}
		return
	}
	if copyReport {
		a.copyPackComparison(c, p)
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		muted(c, "本地清单 · 仅比较名称、类型和文件停用状态").FontSize(12).Grow(1).MinWidth(0)
		if p.copied {
			muted(c, "已复制比对清单").FontSize(12).Shrink(0)
		}
	})
	if p.copyFailed {
		feedback.Alert(c, "复制比对清单失败，请重试", feedback.AlertProps{Type: feedback.Error})
	}
	switch {
	case p.err != nil:
		var scan *content.ResourcePackScanError
		if errors.As(p.err, &scan) {
			label := "无法读取对照实例的资源包"
			if scan.InstanceID == p.base.ID {
				label = "无法读取当前实例的资源包"
			}
			feedback.Alert(c, label, feedback.AlertProps{Type: feedback.Error, Description: "检查对应的游戏目录后重新比对。"})
		} else {
			feedback.Alert(c, p.err.Error(), feedback.AlertProps{Type: feedback.Error})
		}
	case len(options) == 0:
		muted(c, "还没有其他已缓存游戏文件的实例。")
	case p.peer.ID == 0:
		muted(c, "选择另一实例以比对本地资源包。")
	case p.loading:
		flightLoading(c, "正在读取两个实例的本地资源包…")
	case len(p.rows) == 0:
		muted(c, "两个实例都没有本地资源包。")
	default:
		ui.List(c, &p.scroll, len(p.rows)).Grow(1).MinHeight(0).Rows(func(row ui.ListRow) {
			packComparisonRow(row.Context, p.rows[row.Index])
		})
	}
}

func packComparisonRow(c *ui.Context, p content.ResourcePackMatch) {
	kind, label := packComparisonKind(p.Directory), packMatchGroups[packGroup(p)].label
	ui.Column(c).Gap(3).Padding(4, 6).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			display.Tag(c, label, display.TagProps{Size: theme.Small})
			display.Text(c, p.Name, display.TextProps{Strong: true, Rows: 1}).Grow(1).MinWidth(0).Ellipsis("…").Selectable()
			muted(c, kind).FontSize(12).Shrink(0)
		})
		muted(c, "当前："+packComparisonSide(p.Current, p.Directory)+"    对照："+packComparisonSide(p.Other, p.Directory)).FontSize(12)
	})
}

func packComparisonSide(state content.ResourcePackState, directory bool) string {
	switch state {
	case content.PackAbsent:
		return "缺少"
	case content.PackDisabled:
		return "已停用"
	case content.PackDuplicate:
		return "同名重复"
	default:
		if directory {
			return "有目录"
		}
		return "未停用"
	}
}
