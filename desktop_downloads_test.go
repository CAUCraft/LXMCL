package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
)

func TestDownloadPageSelectsVersionAndLoader(t *testing.T) {
	a, tt := testDesktop(t)
	a.releases = []launcher.Version{{ID: "26.3"}, {ID: "1.21.11"}, {ID: "1.21.4"}, {ID: "1.21.1"}}
	tt.SetSize(820, 530)
	tt.SetDark(true)
	tt.Advance(300 * time.Millisecond)
	for _, label := range []string{"下载", "1.21.4", "Fabric"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	p := &a.install
	p.fabricGame, p.fabric, p.fabricVersions = p.version, "0.16.14", []string{"0.16.14", "0.16.13"}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	if a.page != pageDownloads || a.downloadPage != downloadGame || p.version != "1.21.4" || p.loader != 1 {
		t.Fatal("the install page did not retain the selected game and loader")
	}
	version, vOK := tt.Find("1.21.4")
	install, iOK := tt.Find("安装实例")
	if !vOK || !iOK || version.X+version.W >= install.X || install.Y+install.H > 530 {
		t.Fatalf("the version list and install action do not fit in the compact window: %+v %+v", version, install)
	}
	capture(t, tt, "download-game-dark")
	next, instance, err := newInstallation(a.state, "下载测试", p.version, p.fabric)
	if err != nil {
		t.Fatal(err)
	}
	if next.Selected != 1 || !instance.Fabric() || instance.LoaderVersion != "0.16.14" || instance.ID != 3 ||
		instance.GameDir != filepath.Join(a.state.Settings.GameRoot, "instance-3") || len(a.state.Instances) != 2 {
		t.Fatalf("installation would change the wrong configuration: %+v / %+v", next, instance)
	}
}

func TestResourceDownloadsKeepInstallTargetSeparate(t *testing.T) {
	a, tt := testDesktop(t)
	a.state.Instances[1].Loader, a.state.Instances[1].LoaderVersion = "Fabric", "0.16.14"
	tt.SetSize(820, 530)
	for _, label := range []string{"下载", "模组"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	p := a.browserFor(launcher.FabricMods)
	if p.target.ID != 0 {
		t.Fatal("the browser silently selected a different instance")
	}
	if err := tt.Click("下载目标实例"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("生存世界 · 1.21.1"); ok {
		t.Fatal("a vanilla instance was offered as a Fabric mod target")
	}
	if err := tt.Click("建筑测试 · 1.21.4"); err != nil {
		t.Fatal(err)
	}
	if p.target.ID != 2 || a.state.Selected != 1 {
		t.Fatal("selecting a mod download target changed the launch target")
	}
	project := launcher.PackProject{ID: "sodium", Title: "Sodium", Author: "CaffeineMC",
		Description: "Minecraft 客户端渲染优化。"}
	p.project, p.searched = project, true
	p.result = launcher.PackSearch{Hits: []launcher.PackProject{
		project, {ID: "lithium", Title: "Lithium", Author: "CaffeineMC"}, {ID: "fabric-api", Title: "Fabric API", Author: "FabricMC"},
	}, Total: 48}
	p.versions = []launcher.PackVersion{
		{ID: "build1", ProjectID: project.ID, Number: "0.6.13", Type: "release", Files: []launcher.PackFile{{Filename: "sodium-0.6.13.jar", Size: 1200000}}},
		{ID: "build2", ProjectID: project.ID, Number: "0.6.14-beta", Type: "beta", Files: []launcher.PackFile{{Filename: "sodium-0.6.14-beta.jar", Size: 1250000}}},
	}
	p.version = p.versions[0].ID
	tt.Frame()
	if err := tt.Click("0.6.14-beta"); err != nil || p.version != "build2" {
		t.Fatalf("file version did not change: %v", err)
	}
	tt.Advance(300 * time.Millisecond)
	capture(t, tt, "download-mods")
	if err := tt.Click("管理已安装"); err != nil {
		t.Fatal(err)
	}
	if a.page != pageLibrary || a.inspected != 2 || a.content.kind() != launcher.FabricMods || a.state.Selected != 1 {
		t.Fatal("returning to installed mods navigated to the wrong instance")
	}
}

func TestInstanceListMenuStaysOpenOutsideRow(t *testing.T) {
	_, tt := testDesktop(t)
	if err := tt.Click("实例"); err != nil {
		t.Fatal(err)
	}
	row, ok := tt.Find("建筑测试")
	if !ok {
		t.Fatal("instance row missing")
	}
	tt.Move(row.X+row.W/2, row.Y+row.H/2)
	if err := tt.Click("实例操作 建筑测试"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	item, ok := tt.Find("下载游戏文件")
	if !ok {
		t.Fatal("download action is missing from the instance menu")
	}
	tt.Move(item.X+item.W/2, item.Y+item.H/2)
	if _, ok := tt.Find("下载游戏文件"); !ok {
		t.Fatal("menu disappeared when the pointer left the instance row")
	}
	capture(t, tt, "instance-list-menu")
	tt.Key(0, ui.KeyEscape)
}
