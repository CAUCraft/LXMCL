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

type modCopyView struct {
	selected []content.File
	target   launcher.Instance
	plan     *content.ModCopyPlan
	err      string
	scroll   ui.ListState
}

func (a *desktop) closeModCopy() {
	if a.contentCancel != nil {
		a.contentCancel()
	}
	a.content.copyMods = nil
}

func (a *desktop) previewModCopy(source, target launcher.Instance, selected []content.File) {
	a.closeModCopy()
	ctx, cancel := context.WithCancel(a.ctx)
	a.contentCancel = cancel
	p := &modCopyView{selected: selected, target: target}
	a.content.copyMods = p
	go func() {
		defer cancel()
		plan, err := content.PreviewModCopy(ctx, source, target, selected)
		a.post(func() {
			if a.content.copyMods != p {
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

func (a *desktop) modCopy(c *ui.Context, source launcher.Instance, p *modCopyView) {
	target, valid := a.state.Find(p.target.ID)
	if p.target.ID != 0 && (!valid || target != p.target) {
		a.closeModCopy()
		p = &modCopyView{selected: p.selected, err: "目标实例已变化，请重新选择。"}
		a.content.copyMods = p
		valid = false
	}
	var options []field.SelectOption
	for _, i := range a.state.Instances {
		if content.ModCopyTarget(source, i) {
			options = append(options, field.SelectOption{Value: strconv.Itoa(i.ID), Label: fmt.Sprintf("%s · #%d", i.Name, i.ID)})
		}
	}
	display.Text(c, "复制所选模组", display.TextProps{Strong: true, FontSize: 16})
	muted(c, fmt.Sprintf("来源 · %s · Minecraft %s · %s", source.Name, source.Version, source.Loader)).SingleLine().Ellipsis("…")
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
		if i, ok := a.state.Find(id); ok {
			a.previewModCopy(source, i, p.selected)
		}
		return
	}
	switch {
	case p.err != "":
		feedback.Alert(c, p.err, feedback.AlertProps{Type: feedback.Error})
	case len(options) == 0:
		muted(c, "没有游戏版本和加载器相同的其他实例。")
	case !valid:
		muted(c, fmt.Sprintf("已选 %d 项。选择目标实例后可预览冲突。", len(p.selected)))
	case p.plan == nil:
		flightLoading(c, "正在核对本地模组…")
	default:
		muted(c, fmt.Sprintf("可复制 %d 项 · 跳过冲突 %d 项", p.plan.Ready, len(p.plan.Items)-p.plan.Ready))
		ui.List(c, &p.scroll, len(p.plan.Items)).Grow(1).MinHeight(0).Rows(func(row ui.ListRow) {
			c, item := row.Context, p.plan.Items[row.Index]
			ui.Column(c).Gap(2).Padding(4, 0).Children(func() {
				display.Text(c, item.File.Name, display.TextProps{Strong: true, Rows: 1}).Ellipsis("…").Selectable()
				status := "复制 · 已启用"
				if !item.File.Enabled {
					status = "复制 · 已停用"
				}
				if item.Conflict != "" {
					status = "跳过 · " + item.Conflict
				}
				muted(c, status).SingleLine().Ellipsis("…").Selectable()
			})
		})
	}
	if p.plan == nil {
		ui.Spacer(c).Grow(1)
	}
	muted(c, "前置模组需自行核对。").FontSize(12)
	ui.Row(c).Gap(8).Children(func() {
		ui.Spacer(c).Grow(1)
		if button(c, "取消", action.Default, false) {
			a.closeModCopy()
			return
		}
		if button(c.Key(p), "确认复制", action.Primary, !valid || p.plan == nil || p.plan.Ready == 0 || a.busy || a.running != nil || a.pickerBusy) {
			plan := p.plan
			a.closeModCopy()
			a.runInstanceTask(target, fmt.Sprintf("复制 %d 项模组到 %s", plan.Ready, target.Name), launcher.Mods, func(ctx context.Context) error {
				done, err := plan.Apply(ctx)
				if err != nil {
					return fmt.Errorf("已复制 %d / %d 项：%w", done, plan.Ready, err)
				}
				return nil
			})
		}
	})
}
