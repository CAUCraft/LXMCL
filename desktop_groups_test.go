package main

import "testing"

func TestGroupFilterAndDirectInstanceNavigation(t *testing.T) {
	a, tt := testDesktop(t)
	first, second := a.state.Instances[0], a.state.Instances[1]
	group := "朋友"
	a.state.InstanceGroups = map[int]string{first.ID: group}
	a.state.LibraryGroup, a.libraryGroup = &group, &group
	a.page, a.query, a.instanceFilter = pageLibrary, first.Name, 1
	tt.SetSize(820, 530)
	tt.Frame()
	if !tt.HasText(first.Name) || tt.HasText(second.Name) {
		t.Fatal(tt.Texts())
	}
	a.inspect(second)
	tt.Frame()
	if !tt.HasText(second.Name) || a.inspected != second.ID || a.state.Selected != first.ID || a.state.LibraryGroup == nil || *a.state.LibraryGroup != group || a.libraryGroup != nil {
		t.Fatal("direct navigation lost its target or the saved preference")
	}
	for _, label := range []string{"更多实例操作", "设置分组…"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	tt.Type("另一个分组")
	if err := tt.Click("取消"); err != nil || a.form.open || a.state.InstanceGroups[second.ID] != "" {
		t.Fatal("cancel changed membership", err)
	}
}
