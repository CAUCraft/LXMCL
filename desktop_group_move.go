package main

import (
	"slices"
	"strconv"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/layout"
)

type groupMoveRow struct {
	instance launcher.Instance
	previous string
	checked  bool
}

type groupMoveDraft struct {
	rows  []groupMoveRow
	group string
}

func (a *desktop) openGroupMove() {
	p := &groupMoveDraft{}
	for _, i := range a.libraryInstances() {
		p.rows = append(p.rows, groupMoveRow{instance: i, previous: a.state.InstanceGroups[i.ID]})
	}
	a.form = editor{mode: "moveGroups", open: true, groupMove: p}
}

func (p *groupMoveDraft) apply(next *launcher.State) error {
	if p.group != "" && !slices.Contains(next.GroupNames(), p.group) {
		return i18n.Error("instance.group_missing")
	}
	for _, row := range p.rows {
		if row.checked {
			if err := next.SetInstanceGroup(row.instance.ID, row.instance.Revision, row.previous, p.group); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *desktop) groupMoveEditor(c *ui.Context, p *groupMoveDraft) bool {
	previous := p.group
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		display.Text(c, "目标分组", display.TextProps{Strong: true})
		groupDestinationPicker(c, &p.group, a.state.GroupNames(), "选择目标分组", a.busy).Grow(1).MinWidth(0)
	})
	if previous != p.group {
		a.form.err = ""
	}
	if len(p.rows) == 0 {
		muted(c, "当前筛选下没有实例")
	}
	count, changed := 0, false
	layout.ScrollArea(c, layout.ScrollAreaProps{}, func() {
		for n := range p.rows {
			row := &p.rows[n]
			ui.Row(c.Key(row.instance.ID)).Gap(8).MinHeight(34).AlignItems(ui.Center).Children(func() {
				checked := row.checked
				choice.Checkbox(c, &row.checked, row.instance.Name, choice.CheckboxProps{}).Label("选择 " + row.instance.Name).Grow(1).MinWidth(0).Disabled(a.busy)
				if checked != row.checked {
					a.form.err = ""
				}
				group := "未分组"
				if row.previous != "" {
					group = groupLabel(row.previous)
				}
				muted(c, group).MaxWidth(140).SingleLine().Ellipsis("…")
			})
			if row.checked {
				count++
				changed = changed || row.previous != p.group
			}
		}
	}).Height(min(170, float32(len(p.rows)*34))).Clip()
	if a.form.err != "" {
		feedback.Alert(c, a.form.err, feedback.AlertProps{Type: feedback.Error})
	}
	submit := false
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		muted(c, "已选 "+strconv.Itoa(count)+" 个实例").Grow(1)
		if button(c, "取消", action.Default, false) {
			a.form.open = false
		}
		submit = button(c, "应用分组", action.Primary, a.busy || !changed)
	})
	return submit
}
