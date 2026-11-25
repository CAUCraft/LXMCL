package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
)

func TestCopyInstalledModInventoryFromResources(t *testing.T) {
	a, tt := testDesktop(t)
	for i := range a.state.Instances {
		p := &a.state.Instances[i]
		p.Name, p.Loader, p.LoaderVersion = "同名实例", launcher.LoaderFabric, "0.16.14"
	}
	first, second := a.state.Instances[0], a.state.Instances[1]
	empty := first
	empty.ID, empty.Name, empty.GameDir = 3, "空模组实例", t.TempDir()
	a.state.Instances = append(a.state.Instances, empty)

	writeMod := func(i launcher.Instance, name, manifest string) {
		t.Helper()
		var data bytes.Buffer
		archive := zip.NewWriter(&data)
		entry, err := archive.Create("fabric.mod.json")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(manifest)); err != nil {
			t.Fatal(err)
		}
		if err := archive.Close(); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(i.GameDir, "mods")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeMod(first, "home-only.jar", `{"id":"home","name":"Home Mod","version":"9.0"}`)
	writeMod(second, "z-disabled.jar.disabled", `{"id":"beacon","name":"Beacon","version":"2.0"}`)
	writeMod(second, "a-enabled.jar", `{"id":"beacon","name":"Beacon","version":"1.0"}`)
	writeMod(second, "unknown.jar", `{"id":"local_mod","name":"/private/mod","version":"${jarVersion}"}`)

	tt.SetSize(820, 530)
	tt.SetPreferences(ui.Preferences{})
	click := func(labels ...string) {
		t.Helper()
		for _, label := range labels {
			if err := tt.Click(label); err != nil {
				t.Fatalf("%s: %v (%v)", label, err, tt.Texts())
			}
			tt.Frame()
		}
	}
	showInventory := func(i launcher.Instance) {
		t.Helper()
		click("资源", "模组")
		if a.content.id != i.ID || a.content.kind() != launcher.Mods {
			t.Fatal("resources did not bind to the browsed instance")
		}
		files, err := launcher.ContentInventory(t.Context(), i, launcher.Mods)
		if err != nil {
			t.Fatal(err)
		}
		a.content.files = files
		a.content.sortFiles()
		tt.Frame()
	}
	const label = "复制全部模组清单"
	const full = "Beacon · 1.0 · 启用\nBeacon · 2.0 · 停用\nlocal_mod · 版本未知 · 启用"
	checkCopy := func(want string) {
		t.Helper()
		if tt.Clipboard() != want || !tt.HasText("已复制全部模组清单") {
			t.Fatalf("clipboard = %q, want %q (%v)", tt.Clipboard(), want, tt.Texts())
		}
	}

	click("实例", "1.21.4 · Fabric 0.16.14")
	showInventory(second)
	click("已停用", "搜索本地资源")
	tt.Type("不存在")
	tt.Frame()
	if len(a.content.visibleFiles(nil)) != 0 || !tt.HasText("没有匹配的资源") {
		t.Fatal("the search and filter did not hide the inventory", tt.Texts())
	}
	for range 12 {
		if tt.Focused(label) {
			break
		}
		tt.Key(0, ui.KeyTab)
	}
	if !tt.Focused(label) {
		t.Fatal("the full inventory copy button was not reachable by Tab")
	}
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	checkCopy(full)
	tt.Advance(500 * time.Millisecond)
	capture(t, tt, "copy-installed-mod-list-light")

	click("清除资源筛选", "本地资源排序", "最近修改")
	tt.SetDark(true)
	tt.Advance(500 * time.Millisecond)
	click(label)
	checkCopy(full)
	capture(t, tt, "copy-installed-mod-list-dark")

	tt.SetClipboard("keep on failed copy")
	tt.FailClipboard(errors.New("clipboard busy"))
	click(label)
	if tt.Clipboard() != "keep on failed copy" || !tt.HasText("复制模组清单失败，请重试：clipboard busy") {
		t.Fatal("copy failure did not retain the clipboard and provide feedback", tt.Texts())
	}
	tt.FailClipboard(nil)

	click("1.21.1 · Fabric 0.16.14 · 启动目标")
	showInventory(first)
	click(label)
	checkCopy("Home Mod · 9.0 · 启用")
	click("空模组实例")
	showInventory(empty)
	if _, ok := tt.Access(label); ok || !tt.HasText("暂无本地模组") || tt.Clipboard() != "Home Mod · 9.0 · 启用" {
		t.Fatal("empty inventory offered a copy or retained the previous instance's list", tt.Texts())
	}
	if a.state.Selected != first.ID || a.busy || a.task.Stage != "" {
		t.Fatal("copy changed the home launch target or started a task")
	}
}
