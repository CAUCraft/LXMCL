package main

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestLibrarySearchMatchesInstanceNotes(t *testing.T) {
	a, tt := testDesktop(t)
	tt.SetSize(820, 530)
	a.state.Instances[0].Name, a.state.Instances[1].Name = "回声基地", "回声基地"
	if err := a.state.Create("哨站样例", "1.21.4"); err != nil {
		t.Fatal(err)
	}
	first, second, third := a.state.Instances[0], a.state.Instances[1], a.state.Instances[2]
	a.state.Selected = first.ID
	a.state.PinnedInstances = map[int]bool{second.ID: true}
	a.state.InstanceNotes = map[int]string{
		first.ID:  "渔港工程",
		second.ID: "RedStone 样例\n钟楼坐标 120 64",
		third.ID:  "铁路样例\n钟楼施工计划",
	}
	const group = "工地测试"
	a.state.InstanceGroups = map[int]string{first.ID: group, second.ID: group, third.ID: "其他测试"}
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(label, err, tt.Texts())
		}
		tt.Frame()
	}
	search := func(query string) {
		t.Helper()
		click("搜索实例")
		tt.Key(ui.Cmd, ui.KeyA)
		if query == "" {
			tt.Key(0, ui.KeyBackspace)
		} else {
			tt.Type(query)
		}
		tt.Frame()
	}
	expect := func(want ...int) {
		t.Helper()
		var got []int
		for _, i := range a.libraryInstances() {
			got = append(got, i.ID)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("visible IDs %v, want %v: %v", got, want, tt.Texts())
		}
	}

	click("实例")
	search("回声基地")
	expect(second.ID, first.ID)
	search("1.21.1 原版")
	expect(first.ID)
	search("  rEdStOnE  ")
	expect(second.ID)
	search("钟楼")
	expect(second.ID, third.ID)
	selectedGroup := group
	a.state.LibraryGroup, a.libraryGroup = &selectedGroup, &selectedGroup
	tt.Frame()
	expect(second.ID)
	click("已缓存")
	expect()
	if !tt.HasText("未找到实例") {
		t.Fatal("the cache filter should exclude the note match")
	}
	tt.SetDark(true)
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	capture(t, tt, "instance-note-search-no-match-dark")
	click("待下载")
	expect(second.ID)
	tt.SetDark(false)
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	if !tt.HasText("回声基地") || tt.HasText("哨站样例") {
		t.Fatal("the visible list did not reflect the combined filters")
	}
	capture(t, tt, "instance-note-search-match-light")
	click("回声基地")
	if a.inspected != second.ID || a.state.Selected != first.ID || a.query != "钟楼" ||
		a.instanceFilter != 2 || a.libraryGroup != &selectedGroup {
		t.Fatal("inspecting a note-only result lost the query, a filter, or an instance ID")
	}
	click("返回实例列表")
	if a.inspected != 0 || a.query != "钟楼" {
		t.Fatal("returning from details lost the note search")
	}
	expect(second.ID)
	search("")
	if a.query != "" || a.instanceFilter != 2 || a.libraryGroup != &selectedGroup {
		t.Fatal("clearing search also cleared other filters")
	}
	expect(second.ID)
	click("全部")
	expect(second.ID, first.ID)
}
