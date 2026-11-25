package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/saves"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

type worldCopyView struct {
	world    saves.World
	target   launcher.Instance
	plan     *saves.WorldCopyPlan
	cancel   context.CancelFunc
	err      string
	canceled bool
}

func (p *worldBrowser) closeWorldCopy() {
	if p.copy != nil && p.copy.cancel != nil {
		p.copy.cancel()
	}
	p.copy = nil
}

func (a *desktop) previewWorldCopy(browser *worldBrowser, source, target launcher.Instance, world saves.World, canceled bool) {
	browser.closeWorldCopy()
	p := &worldCopyView{world: world, target: target, canceled: canceled}
	browser.copy = p
	if !saves.WorldCopyCompatible(source, target) {
		p.err = "游戏版本和加载器必须一致，模组实例还须使用相同的加载器版本。"
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	p.cancel = cancel
	go func() {
		defer cancel()
		plan, err := saves.PreviewWorldCopy(ctx, source, target, world)
		a.post(func() {
			if !a.form.open || a.form.worlds != browser || browser.copy != p {
				return
			}
			p.cancel = nil
			if err != nil {
				p.err = err.Error()
			} else {
				p.plan = plan
			}
		})
	}()
}

func (a *desktop) worldCopyEditor(c *ui.Context, browser *worldBrowser, source launcher.Instance, p *worldCopyView) {
	current, exists := a.state.Find(source.ID)
	stale := !exists || current != source
	target, ok := a.state.Find(p.target.ID)
	valid := ok && target == p.target
	panelHeading(c, lucide.Copy(), p.world.Folder(), fmt.Sprintf("来源实例 · %s · #%d", source.Name, source.ID))
	muted(c, worldSavedVersionLabel(p.world)).FontSize(12).SingleLine().Ellipsis("…")
	muted(c, "源实例配置版本 · Minecraft "+source.Version+" · "+loaderName(source)).FontSize(12)

	options := make([]field.SelectOption, 0, len(a.state.Instances))
	for _, i := range a.state.Instances {
		if i.ID != source.ID {
			options = append(options, field.SelectOption{Value: strconv.Itoa(i.ID), Label: fmt.Sprintf("%s · #%d", i.Name, i.ID)})
		}
	}
	selected := ""
	if valid {
		selected = strconv.Itoa(target.ID)
	}
	before := selected
	locked := a.busy || a.running != nil || a.pickerBusy || a.ctx == nil
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		muted(c, "目标实例")
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			catalogPicker(c, &selected, options, "选择目标实例", "", stale || locked).FillWidth()
		})
	})
	if selected != before {
		id, _ := strconv.Atoi(selected)
		if i, ok := a.state.Find(id); ok {
			a.previewWorldCopy(browser, source, i, p.world, false)
		}
		return
	}
	if valid {
		muted(c, "目标实例配置版本 · Minecraft "+target.Version+" · "+loaderName(target)).FontSize(12)
	}
	switch {
	case stale:
		feedback.Alert(c, "来源实例已变化，请重新打开存档列表。", feedback.AlertProps{Type: feedback.Warning})
	case len(options) == 0:
		muted(c, "没有其他实例可选。")
	case !valid && p.target.ID != 0:
		feedback.Alert(c, "目标实例已变化，请重新选择。", feedback.AlertProps{Type: feedback.Warning})
	case !valid:
		muted(c, "请选择游戏版本及加载器一致的目标实例。")
	case p.err != "":
		feedback.Alert(c, p.err, feedback.AlertProps{Type: feedback.Warning})
	case p.plan == nil:
		flightLoading(c, "正在核对存档和目标位置…")
	default:
		muted(c, "目标路径").FontSize(12)
		display.Text(c, p.plan.Destination(), display.TextProps{FontSize: 13, Rows: 2}).Ellipsis("…").Selectable()
		if filepath.Base(p.plan.Destination()) != p.world.Folder() {
			muted(c, "同名存档保留，复制到新目录。").FontSize(12)
		}
		muted(c, "实例配置的游戏版本与加载器一致。模组依赖仍需自行核对。").FontSize(12)
	}
	if p.canceled {
		muted(c, "复制已取消，确认后可重试。").FontSize(12)
	}
	ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
		if button(c, "返回存档列表", action.Default, false) {
			browser.closeWorldCopy()
			return
		}
		if button(c, "确认复制", action.Primary, stale || !valid || p.plan == nil || locked) {
			a.runWorldCopy(browser, source, p)
		}
	})
}

func (a *desktop) runWorldCopy(browser *worldBrowser, source launcher.Instance, p *worldCopyView) {
	target, plan, world := p.target, p.plan, p.world
	a.form.open = false
	browser.closeWorldCopy()
	row := taskRecord{Action: "复制世界", InstanceID: target.ID, Revision: target.Revision,
		Name: target.Name, Version: target.Version, World: world.Folder()}
	a.runFileTaskWithResult(row, func(ctx context.Context, _ func(launcher.Progress)) error {
		return plan.Apply(ctx)
	}, func(record *taskRecord) {
		if record.Status == taskCompleted {
			record.Detail = fmt.Sprintf("来源 · %s · #%d\n目标目录 · %s", source.Name, source.ID, plan.Destination())
		}
	}, func(err error) {
		if !errors.Is(err, context.Canceled) || a.form.open || a.form.worlds != browser ||
			a.page != pageLibrary || a.detailPage != detailFiles || a.inspected != source.ID {
			return
		}
		if current, ok := a.state.Find(source.ID); !ok || current != source {
			return
		}
		a.form.open = true
		a.previewWorldCopy(browser, source, target, world, true)
	})
}
