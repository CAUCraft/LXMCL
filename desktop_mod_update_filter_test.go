package main

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
)

func TestModUpdateStatusFilter(t *testing.T) {
	a, tt := testDesktop(t)
	i := &a.state.Instances[1]
	i.Loader, i.LoaderVersion = "Fabric", "0.16.14"
	mods := make(map[string]launcher.InstalledMod)
	for _, file := range []launcher.ContentFile{
		{Name: "a-map.jar", Enabled: true, Size: 1800},
		{Name: "b-map.jar", Enabled: false, Size: 3200},
		{Name: "c-lantern.jar", Enabled: true, Size: 2400},
		{Name: "d-river.jar", Enabled: true, Size: 3600},
		{Name: "e-local.jar", Enabled: true, Size: 1000},
	} {
		m := launcher.InstalledMod{File: file}
		m.Title = file.Name
		m.Current = launcher.PackVersion{ID: "old", ProjectID: file.Name, Number: "1.0"}
		if file.Name != "e-local.jar" {
			m.Update = launcher.PackVersion{ID: "next", ProjectID: file.Name, Number: "2.0", Changelog: json.RawMessage("null")}
		}
		mods[file.DiskName()] = m
		a.content.files = append(a.content.files, file)
	}
	if err := a.state.SetModUpdateSkipped(i.ID, i.Revision, mods["d-river.jar"].UpdateKey(), true); err != nil {
		t.Fatal(err)
	}
	a.content.id, a.content.revision, a.content.tab = i.ID, i.Revision, 1
	a.inspect(*i)
	a.detailPage = detailContent
	tt.SetSize(820, 530)
	tt.Frame()
	click := func(labels ...string) {
		t.Helper()
		for _, label := range labels {
			if err := tt.Click(label); err != nil {
				t.Fatal(label, err, tt.Texts())
			}
			tt.Frame()
		}
	}
	const picker = "模组更新状态筛选"
	const clear = "清除资源筛选"
	checkFiles := func(want ...string) {
		t.Helper()
		var got []string
		for _, file := range a.content.listing.files {
			got = append(got, file.DiskName())
		}
		if !slices.Equal(got, want) {
			t.Fatal("visible mod files", got, "want", want, tt.Texts())
		}
	}
	click(picker, "可更新")
	checkFiles()
	if !tt.HasText("尚未检查更新") || tt.HasText("没有匹配的模组") {
		t.Fatal("missing check was shown as an empty result", tt.Texts())
	}
	click(clear)
	checkFiles("a-map.jar", "b-map.jar.disabled", "c-lantern.jar", "d-river.jar", "e-local.jar")

	a.content.mods, a.content.listing = mods, nil
	tt.Frame()
	click("本地资源排序", "文件较大", picker, "可更新")
	checkFiles("b-map.jar.disabled", "c-lantern.jar", "a-map.jar")
	click("选择资源 c-lantern.jar", "已启用", "搜索本地资源")
	tt.Type("mAP")
	tt.Frame()
	checkFiles("a-map.jar")
	if len(a.content.selected) != 0 {
		t.Fatal("hidden mod stayed selected", a.content.selected)
	}
	click("全选当前列表")
	if got := a.content.selected; len(got) != 1 || !got["a-map.jar"] || !tt.HasText("更新选中 (1)") {
		t.Fatal("batch scope includes hidden or skipped mods", got, tt.Texts())
	}
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		for _, label := range []string{picker, "全选当前列表", "本地资源排序", "更新选中 (1)", "停用所选"} {
			box, ok := tt.Find(label)
			if !ok || box.X < 0 || box.Y < 42 || box.X+box.W > 820.5 || box.Y+box.H > 530.5 {
				t.Fatal("clipped control", label, box, tt.Texts())
			}
		}
		name := "mod-update-filter-light"
		if dark {
			name = "mod-update-filter-dark"
		}
		capture(t, tt, name)
	}
	click("查看发布说明 a-map.jar")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	checkFiles("a-map.jar")
	if !tt.HasText("更新选中 (1)") || a.content.modFilter != modUpdateAvailable {
		t.Fatal("returning from notes lost the filtered selection", tt.Texts())
	}
	click(picker, "已跳过")
	checkFiles()
	if !tt.HasText("没有匹配的模组") || tt.HasText("尚未检查更新") {
		t.Fatal("completed check with no matches did not show empty results", tt.Texts())
	}
	click(clear)
	checkFiles("d-river.jar", "b-map.jar.disabled", "c-lantern.jar", "a-map.jar", "e-local.jar")
	click(picker, "已跳过")
	checkFiles("d-river.jar")
	click("全选当前列表")
	if !tt.HasText("更新选中 (0)") || a.state.Selected != 1 || a.busy || len(a.history) != 0 {
		t.Fatal("filtering changed the launch target, queue or update scope", tt.Texts())
	}
}
