package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
)

type localCopyView struct {
	selected       []content.File
	source, target launcher.Instance
	kind           content.Kind
	plan           *content.CopyPlan
	err            string
	scroll         ui.ListState
}

func (a *desktop) closeLocalCopy() {
	if a.contentCancel != nil {
		a.contentCancel()
	}
	a.content.copyLocal = nil
}

func (a *desktop) previewLocalCopy(source, target launcher.Instance, kind content.Kind, selected []content.File) {
	if a.ctx == nil {
		return
	}
	a.closeLocalCopy()
	ctx, cancel := context.WithCancel(a.ctx)
	a.contentCancel = cancel
	p := &localCopyView{selected: selected, source: source, target: target, kind: kind}
	a.content.copyLocal = p
	preview := content.PreviewModCopy
	if kind == launcher.ResourcePacks {
		preview = content.PreviewPackCopy
	}
	go func() {
		defer cancel()
		plan, err := preview(ctx, source, target, selected)
		a.post(func() {
			if a.content.copyLocal != p {
				return
			}
			if err != nil {
				p.err = err.Error()
			} else {
				p.plan = plan
			}
		})
	}()
}

func (a *desktop) localCopy(c *ui.Context, source launcher.Instance, p *localCopyView) {
	if p.source != source {
		a.closeLocalCopy()
		a.message("来源实例已变化，请刷新资源列表后重新选择。", nil)
		return
	}
	target, valid := a.state.Find(p.target.ID)
	if p.target.ID != 0 && (!valid || target != p.target) {
		a.closeLocalCopy()
		p = &localCopyView{selected: p.selected, source: source, kind: p.kind, err: "目标实例已变化，请重新选择。"}
		a.content.copyLocal = p
		valid = false
	}
	compatible := content.ModCopyTarget
	empty := "没有游戏版本和加载器相同的其他实例。"
	hint := "前置模组需自行核对。"
	if p.kind == launcher.ResourcePacks {
		compatible = content.PackCopyTarget
		empty = "没有 Minecraft 版本相同的其他实例。"
		hint = "保留文件启停状态。游戏内资源包选择和顺序照旧。"
	}
	var options []field.SelectOption
	for _, i := range a.state.Instances {
		if compatible(source, i) {
			options = append(options, field.SelectOption{Value: strconv.Itoa(i.ID), Label: fmt.Sprintf("%s · #%d", i.Name, i.ID)})
		}
	}
	label := contentLabel(p.kind)
	display.Text(c, "复制所选"+label, display.TextProps{Strong: true, FontSize: 16})
	muted(c, fmt.Sprintf("来源 · #%d · %s · Minecraft %s · %s", source.ID, source.Name, source.Version, source.Loader)).SingleLine().Ellipsis("…")
	selected := ""
	if valid {
		selected = strconv.Itoa(target.ID)
	}
	old := selected
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		muted(c, "复制到")
		ui.Column(c).Grow(1).MinWidth(0).Children(func() { catalogPicker(c, &selected, options, "选择目标实例", "", len(options) == 0) })
	})
	if selected != old {
		id, _ := strconv.Atoi(selected)
		if i, ok := a.state.Find(id); ok && compatible(source, i) {
			a.previewLocalCopy(source, i, p.kind, p.selected)
		}
		return
	}
	switch {
	case p.err != "":
		feedback.Alert(c, p.err, feedback.AlertProps{Type: feedback.Error})
	case len(options) == 0:
		muted(c, empty)
	case !valid:
		muted(c, fmt.Sprintf("已选 %d 项。选择目标实例后可预览冲突。", len(p.selected)))
	case p.plan == nil:
		flightLoading(c, "正在核对本地"+label+"…")
	default:
		muted(c, fmt.Sprintf("可复制 %d 项 · 跳过冲突 %d 项", p.plan.Ready, len(p.plan.Items)-p.plan.Ready))
		ui.List(c, &p.scroll, len(p.plan.Items)).Grow(1).MinHeight(0).Rows(func(row ui.ListRow) {
			c, item := row.Context, p.plan.Items[row.Index]
			ui.Column(c).Gap(2).Padding(4, 0).Children(func() {
				display.Text(c, item.File.Name, display.TextProps{Strong: true, Rows: 1}).Ellipsis("…").Selectable()
				state := "已启用"
				if !item.File.Enabled {
					state = "已停用"
				}
				status := "复制 · " + state
				if item.Conflict != "" {
					status = "跳过 · " + item.Conflict
					if p.kind == launcher.ResourcePacks {
						status = state + " · " + status
					}
				}
				muted(c, status).SingleLine().Ellipsis("…").Selectable()
			})
		})
	}
	if p.plan == nil {
		ui.Spacer(c).Grow(1)
	}
	muted(c, hint).FontSize(12)
	ui.Row(c).Gap(8).Children(func() {
		ui.Spacer(c).Grow(1)
		if button(c, "取消", action.Default, false) {
			a.closeLocalCopy()
			return
		}
		if button(c.Key(p), "确认复制", action.Primary, !valid || p.plan == nil || p.plan.Ready == 0 || a.busy || a.running != nil || a.pickerBusy) {
			plan := p.plan
			a.closeLocalCopy()
			a.runInstanceTask(target, fmt.Sprintf("复制 %d 项%s到 %s", plan.Ready, label, target.Name), p.kind, func(ctx context.Context) error {
				done, err := plan.Apply(ctx)
				if err != nil {
					return fmt.Errorf("已复制 %d / %d 项：%w", done, plan.Ready, err)
				}
				return nil
			})
		}
	})
}
