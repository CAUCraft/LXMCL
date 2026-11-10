package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
)

func TestModpackWorkspaceShowsSourcesAndNewInstanceConfirmation(t *testing.T) {
	a, tt := testDesktop(t)
	tt.SetSize(820, 530)
	tt.SetDark(true)
	p := &a.modpacks
	p.searched = true
	project := launcher.ModpackProject{ID: "demo", Name: "飞行测试包", Author: "Flight Lab",
		Source: launcher.ModrinthModpacks, Summary: "Minecraft 客户端整合包"}
	p.results = launcher.ModpackResults{Projects: []launcher.ModpackProject{project}, Total: 1}
	p.project = project
	p.releases = []launcher.ModpackRelease{
		{ID: "fabric", Title: "飞行测试包 1.0", Games: []string{"1.21.1"}, Loaders: []string{"fabric"}, Channel: "release"},
		{ID: "older", Title: "飞行测试包旧版", Games: []string{"1.19.2"}, Loaders: []string{"forge"}, Unavailable: "需要 Minecraft 1.20.1 或之后的正式版"},
	}
	for _, label := range []string{"下载", "整合包", "飞行测试包 1.0"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	projectRow, hasProject := tt.Find(project.Name)
	release, hasRelease := tt.Find("飞行测试包 1.0")
	if !hasProject || !hasRelease || projectRow.X+projectRow.W >= release.X || p.releaseID != "fabric" {
		t.Fatalf("expected separate project and version lists: %+v %+v / %q", projectRow, release, p.releaseID)
	}
	tt.Advance(300 * time.Millisecond)
	capture(t, tt, "download-modpacks-online")
	for _, label := range []string{"CurseForge", "设置 API Key…"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if !p.keyOpen {
		t.Fatal("CurseForge is missing a path to enter the API key")
	}
	tt.Key(0, ui.KeyEscape)
	if p.keyOpen {
		t.Fatal("API key dialog did not close on Escape")
	}
	if err := tt.Click("本地导入"); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "flight.mrpack")
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	entry, err := w.Create("modrinth.index.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = entry.Write([]byte(`{"formatVersion":1,"game":"minecraft","name":"飞行测试包","versionId":"1.0","dependencies":{"minecraft":"1.21.1","forge":"52.1.14"}}`)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	p.prepared, err = launcher.PrepareLocalModpack(t.Context(), file)
	if err != nil {
		t.Fatal(err)
	}
	next, inst, err := newLoaderInstallation(a.state, p.prepared.Info().Name, p.prepared.Info().Minecraft,
		p.prepared.Info().Loader, p.prepared.Info().LoaderVersion)
	if err != nil || !inst.Forge() || inst.LoaderVersion != "52.1.14" || next.Selected != a.state.Selected {
		t.Fatalf("wrong Forge instance or selection: %+v, %v", inst, err)
	}
	p.name, p.archive = p.prepared.Info().Name, filepath.Base(file)
	tt.Frame()
	tt.Advance(300 * time.Millisecond)
	picker, hasPicker := tt.Find("选择整合包文件…")
	install, hasInstall := tt.Find("安装整合包")
	if !hasPicker || !hasInstall || picker.X+picker.W >= install.X || install.Y+install.H > 530 || a.state.Selected != 1 {
		t.Fatalf("import controls don't fit or changed the launch selection: %+v %+v", picker, install)
	}
	capture(t, tt, "download-modpacks-import")
}

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
	p.loaderGame, p.loaderKind, p.loaderVersion = p.version, launcher.LoaderFabric, "0.16.14"
	p.loaderVersions = []launcher.LoaderRelease{{Version: "0.16.14"}, {Version: "0.16.13"}}
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
	next, instance, err := newLoaderInstallation(a.state, "下载测试", p.version, p.kind(), p.loaderVersion)
	if err != nil {
		t.Fatal(err)
	}
	if next.Selected != 1 || !instance.Fabric() || instance.LoaderVersion != "0.16.14" || instance.ID != 3 ||
		instance.GameDir != filepath.Join(a.state.Settings.GameRoot, "instance-3") || len(a.state.Instances) != 2 {
		t.Fatalf("installation would change the wrong configuration: %+v / %+v", next, instance)
	}
	if err := tt.Click("NeoForge"); err != nil {
		t.Fatal(err)
	}
	if p.currentLoaderCatalog() {
		t.Fatal("switching loaders retained the Fabric catalog")
	}
	p.loaderGame, p.loaderKind, p.loaderVersion = p.version, launcher.LoaderNeoForge, "21.4.120"
	p.loaderVersions = []launcher.LoaderRelease{{Version: "21.4.120"}}
	tt.Frame()
	if _, ok := tt.Find("NeoForge 版本"); !ok {
		t.Fatal("missing NeoForge catalog control")
	}
}

func TestResourceDownloadsKeepInstallTargetSeparate(t *testing.T) {
	a, tt := testDesktop(t)
	a.state.Instances[0].Loader, a.state.Instances[0].LoaderVersion = launcher.LoaderForge, "52.1.21"
	a.state.Instances[1].Loader, a.state.Instances[1].LoaderVersion = launcher.LoaderNeoForge, "21.4.120"
	tt.SetSize(820, 530)
	for _, label := range []string{"下载", "模组"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	p := a.browserFor(launcher.Mods)
	if p.target.ID != 0 {
		t.Fatal("the browser silently selected a different instance")
	}
	if _, ok := tt.Find("筛选加载器"); !ok {
		t.Fatal("standalone downloads did not expose an independent filter")
	}
	project := launcher.PackProject{ID: "sodium", Title: "Sodium", Author: "CaffeineMC",
		Description: "Minecraft 客户端渲染优化。"}
	p.project, p.searched = project, true
	p.result = launcher.PackSearch{Hits: []launcher.PackProject{
		project, {ID: "lithium", Title: "Lithium", Author: "CaffeineMC"}, {ID: "balm", Title: "Balm", Author: "BlayTheNinth"},
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
	if _, ok := tt.Find("保存到文件夹…"); !ok {
		t.Fatal("standalone downloads require an instance")
	}
	capture(t, tt, "download-mods")
	if err := tt.Click("实例"); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"建筑测试", "资源", "模组", "添加模组…"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if p.target.ID != 2 || a.state.Selected != 1 || p.project.ID != "" {
		t.Fatal("instance download did not reset the standalone catalog and capture the instance")
	}
	if err := tt.Click("管理已安装"); err != nil {
		t.Fatal(err)
	}
	if a.page != pageLibrary || a.inspected != 2 || a.content.kind() != launcher.Mods || a.state.Selected != 1 {
		t.Fatal("returning to installed mods navigated to the wrong instance")
	}
	if err := tt.Click("添加模组…"); err != nil {
		t.Fatal(err)
	}
	i := &a.state.Instances[1]
	i.Loader, i.LoaderVersion, i.Revision = launcher.LoaderQuilt, "0.30.2", i.Revision+1
	tt.Frame()
	if p.target.ID != i.ID || p.target.Loader != launcher.LoaderQuilt || p.project.ID != "" || len(p.versions) != 0 || a.state.Selected != 1 {
		t.Fatal("changing the target loader retained incompatible results or changed the launch target")
	}
	if err := tt.Click("下载"); err != nil || p.target.ID != 0 {
		t.Fatalf("the top navigation did not reopen standalone downloads: %v", err)
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
