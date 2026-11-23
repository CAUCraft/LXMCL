package main

import (
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
)

func (a *desktop) matchesGroup(id int) bool {
	return a.libraryGroup == nil || a.state.InstanceGroups[id] == *a.libraryGroup
}

// History and other direct navigation can reveal an instance temporarily.
// Only an explicit filter choice changes the preference restored on restart.
func (a *desktop) chooseLibraryGroup(group *string) {
	next := a.state.Clone()
	next.LibraryGroup = group
	previous := a.libraryGroup
	a.libraryGroup = group
	a.scroll.Y = 0
	a.save(next, 0, func(err error) {
		if err != nil {
			if a.libraryGroup == group {
				a.libraryGroup = previous
			}
			a.message("保存分组筛选失败", err)
			return
		}
		if a.page == pageLibrary && !a.matchesGroup(a.inspected) {
			a.inspected = 0
		}
	})
}

func groupLabel(name string) string {
	if name == "全部分组" || name == "未分组" || name == "重命名当前分组…" {
		return "分组 · " + name
	}
	return name
}

func (a *desktop) libraryGroupPicker(c *ui.Context) {
	groups := a.state.GroupNames()
	if p := a.libraryGroup; p != nil && *p != "" && !slices.Contains(groups, *p) {
		a.libraryGroup = nil
	}
	options := []field.SelectOption{{Value: "", Label: "全部分组"}, {Value: "group:", Label: "未分组"}}
	key, fallback := "", ""
	if a.libraryGroup != nil {
		key = "group:" + *a.libraryGroup
	} else if a.state.LibraryGroup != nil {
		// Direct navigation reveals all rows. Choosing all explicitly still needs a save.
		key, fallback = "revealed", "临时查看全部"
	}
	for _, g := range groups {
		options = append(options, field.SelectOption{Value: "group:" + g, Label: groupLabel(g)})
	}
	if a.libraryGroup != nil && *a.libraryGroup != "" {
		options = append(options, field.SelectOption{Value: "rename", Label: "重命名当前分组…"})
	}
	selected := key
	catalogPicker(c, &selected, options, "筛选实例分组", fallback, a.busy).Width(128)
	if selected == "rename" && a.libraryGroup != nil {
		name := *a.libraryGroup
		a.form = editor{mode: "renameGroup", open: true, focus: true, name: name, originalGroup: name}
		return
	}
	if selected != key {
		if name, ok := strings.CutPrefix(selected, "group:"); ok {
			a.chooseLibraryGroup(&name)
		} else {
			a.chooseLibraryGroup(nil)
		}
	}
}

func (a *desktop) editGroup(i launcher.Instance) {
	group := a.state.InstanceGroups[i.ID]
	a.form = editor{mode: "group", open: true, target: i, name: group, originalGroup: group, focus: true}
}

func (a *desktop) groupEditor(c *ui.Context) bool {
	display.Text(c, "分组名称", display.TextProps{Strong: true})
	placeholder := "输入分组名，留空即为未分组"
	if a.form.mode == "renameGroup" {
		placeholder = "输入新分组名称"
	}
	previous := a.form.name
	input := field.Input(c, &a.form.name, field.InputProps{Placeholder: placeholder}).Label("分组名称").Disabled(a.busy)
	if a.form.focus {
		input.Focus()
		a.form.focus = false
	}
	groups := a.state.GroupNames()
	if a.form.mode == "group" && len(groups) > 0 {
		ui.Row(c).Gap(8).Children(func() {
			options := make([]field.SelectOption, 0, len(groups))
			for _, g := range groups {
				options = append(options, field.SelectOption{Value: g, Label: g})
			}
			catalogPicker(c, &a.form.name, options, "选择已有分组", "选择已有分组", a.busy).Width(152)
			if a.form.originalGroup != "" && button(c, "移出分组", action.Text, a.busy || a.form.name == "") {
				a.form.name = ""
			}
		})
	}
	if a.form.name != previous {
		a.form.err = ""
	}
	if a.form.err != "" {
		feedback.Alert(c, a.form.err, feedback.AlertProps{Type: feedback.Error})
	}
	return input.Submitted()
}
