package main

import (
	"cmp"
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"

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

type modComparisonView struct {
	peer    launcher.Instance
	result  content.ModComparison
	reading bool
	err     string
	scroll  ui.ListState
}

func (a *desktop) readModComparison(base, peer launcher.Instance) {
	if a.contentCancel != nil {
		a.contentCancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.contentCancel = cancel
	p := &modComparisonView{peer: peer, reading: true}
	a.content.comparison = p
	go func() {
		defer cancel()
		result, err := content.CompareMods(ctx, base, peer)
		a.post(func() {
			if a.content.comparison != p {
				return
			}
			p.reading = false
			if err != nil {
				p.err = err.Error()
				return
			}
			p.result = result
		})
	}()
}

func (a *desktop) modComparison(c *ui.Context, base launcher.Instance, p *modComparisonView) {
	peer, valid := a.state.Find(p.peer.ID)
	if p.peer.ID != 0 && (!valid || peer != p.peer) {
		if a.contentCancel != nil {
			a.contentCancel()
		}
		valid = false
		p = &modComparisonView{err: "对照实例已变化，请重新选择。"}
		a.content.comparison = p
	}
	var copyRequested bool
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if icon(c, lucide.ArrowLeft(), "返回模组列表", false) {
			a.refreshContent(base)
		}
		display.Text(c, "比对模组", display.TextProps{Strong: true, FontSize: 16}).Grow(1)
		if valid && !p.reading && p.err == "" {
			copyRequested = button(c.Key(p), "复制差异清单", action.Default, false)
		}
		if icon(c, lucide.RefreshCw(), "重新比对", !valid || p.reading) {
			a.readModComparison(base, peer)
		}
	})
	var options []field.SelectOption
	for _, i := range a.state.Instances {
		if i.ID != base.ID && launcher.Mods.Supports(i) {
			options = append(options, field.SelectOption{Value: strconv.Itoa(i.ID), Label: fmt.Sprintf("%s · #%d", i.Name, i.ID)})
		}
	}
	selected := ""
	if valid {
		selected = strconv.Itoa(peer.ID)
	}
	old := selected
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		muted(c, "基准 · "+base.Name).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		muted(c, "对照")
		ui.Column(c).Width(180).Children(func() { catalogPicker(c, &selected, options, "选择对照实例", "", false) })
	})
	if selected != old {
		id, _ := strconv.Atoi(selected)
		if target, ok := a.state.Find(id); ok {
			a.readModComparison(base, target)
		}
		return
	}
	switch {
	case p.err != "":
		feedback.Alert(c, p.err, feedback.AlertProps{Type: feedback.Error})
		return
	case len(options) == 0:
		muted(c, "还没有其他支持模组的实例。")
		return
	case !valid:
		muted(c, "选择要与当前实例比对的实例。")
		return
	case p.reading:
		flightLoading(c, "正在读取两个实例的本地模组…")
		return
	}
	if copyRequested && a.content.comparison == p {
		c.WriteClipboardThen(modComparisonReport(base, peer, p.result), func(err error) {
			if a.content.comparison != p {
				return
			}
			if err != nil {
				a.message("复制失败，请重试", err)
				return
			}
			a.message("已复制模组差异清单", nil)
		})
	}
	if ids := p.result.Ambiguous; len(ids) > 0 {
		display.Text(c, "重复 ID 或文件名未配对："+strings.Join(ids, "、"), display.TextProps{Rows: 2}).Ellipsis("…").Selectable()
	}
	if n := p.result.UnknownVersions; n > 0 {
		muted(c, fmt.Sprintf("%d 项缺少可核对的版本", n))
	}
	if len(p.result.Rows) == 0 {
		muted(c, "未发现可确认的差异。")
		return
	}
	ui.List(c, &p.scroll, len(p.result.Rows)).Grow(1).MinHeight(0).Rows(func(row ui.ListRow) {
		c, d := row.Context, p.result.Rows[row.Index]
		name := d.Name
		if d.ByFilename {
			name = "文件 · " + name
		}
		ui.Column(c).Gap(3).Padding(5, 4).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				display.Tag(c, modDifferenceLabel(d), display.TagProps{Size: theme.Small})
				display.Text(c, name, display.TextProps{Strong: true, Rows: 1}).Grow(1).MinWidth(0).Ellipsis("…").Selectable()
			})
			muted(c, modSide(d.Before)+"  →  "+modSide(d.After)).Selectable()
		})
	})
}

func modDifferenceLabel(d content.ModDifference) string {
	switch {
	case d.Before == nil:
		return "新增"
	case d.After == nil:
		return "缺少"
	case d.VersionChanged() && d.Before.Enabled != d.After.Enabled:
		return "版本及启用状态"
	case d.VersionChanged():
		return "版本"
	default:
		return "启用状态"
	}
}

// A shared field must fit on one line and cannot carry a file path.
func shareField(s string) string {
	if strings.ContainsAny(s, "/\\") || strings.ContainsFunc(s, func(r rune) bool {
		return unicode.IsControl(r) || r == '\u2028' || r == '\u2029'
	}) {
		return ""
	}
	return strings.TrimSpace(s)
}

func modComparisonReport(base, peer launcher.Instance, result content.ModComparison) string {
	var report strings.Builder
	platform := func(i launcher.Instance) string {
		return "Minecraft " + cmp.Or(shareField(i.Version), "版本未知") + " · " + cmp.Or(shareField(i.LoaderName()), "加载器未知")
	}
	fmt.Fprintf(&report, "基准：%s\n对照：%s\n", platform(base), platform(peer))
	for _, d := range result.Rows {
		fmt.Fprintf(&report, "%s · %s：%s → %s\n", modDifferenceLabel(d), cmp.Or(shareField(d.Name), "名称未知"), sharedModSide(d.Before), sharedModSide(d.After))
	}
	for _, name := range result.Ambiguous {
		fmt.Fprintf(&report, "重复未配对 · %s\n", cmp.Or(shareField(name), "名称未知"))
	}
	if n := result.UnknownVersions; n > 0 {
		fmt.Fprintf(&report, "%d 项缺少可核对的版本\n", n)
	}
	if len(result.Rows) == 0 {
		report.WriteString("未发现可确认的差异。\n")
	}
	return strings.TrimSuffix(report.String(), "\n")
}

func sharedModSide(s *content.ModState) string {
	if s == nil {
		return modSide(nil)
	}
	v := *s
	v.Version = shareField(v.Version)
	return modSide(&v)
}

func modSide(s *content.ModState) string {
	if s == nil {
		return "无"
	}
	status := "启用"
	if !s.Enabled {
		status = "停用"
	}
	if s.Version == "" {
		return "版本未知 · " + status
	}
	return s.Version + " · " + status
}
