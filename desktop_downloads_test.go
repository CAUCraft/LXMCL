package main

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	p.prepared = prepareDesktopTestPack(t, "flight.mrpack", "modrinth.index.json", `{"formatVersion":1,"game":"minecraft","name":"飞行测试包","versionId":"1.0","dependencies":{"minecraft":"1.21.1","forge":"52.1.14"}}`)
	next, inst, err := newLoaderInstallation(a.state, p.prepared.Info().Name, p.prepared.Info().Minecraft,
		p.prepared.Info().Loader, p.prepared.Info().LoaderVersion)
	if err != nil || !inst.Forge() || inst.LoaderVersion != "52.1.14" || next.Selected != a.state.Selected {
		t.Fatalf("wrong Forge instance or selection: %+v, %v", inst, err)
	}
	p.name, p.archive = p.prepared.Info().Name, "flight.mrpack"
	tt.Frame()
	tt.Advance(300 * time.Millisecond)
	picker, hasPicker := tt.Find("选择整合包文件…")
	install, hasInstall := tt.Find("安装整合包")
	if !hasPicker || !hasInstall || picker.X+picker.W >= install.X || install.Y+install.H > 530 || a.state.Selected != 1 {
		t.Fatalf("import controls don't fit or changed the launch selection: %+v %+v", picker, install)
	}
	capture(t, tt, "download-modpacks-import")
}

func prepareDesktopTestPack(t *testing.T, filename, entryName, manifest string) launcher.PreparedModpack {
	t.Helper()
	file := filepath.Join(t.TempDir(), filename)
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	entry, err := w.Create(entryName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = entry.Write([]byte(manifest)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	pack, err := launcher.PrepareLocalModpack(t.Context(), file)
	if err != nil {
		t.Fatal(err)
	}
	return pack
}

func TestRejectedKeyCanBeReplacedFromModpackHistory(t *testing.T) {
	a, tt := testDesktop(t)
	tt.SetSize(820, 530)
	p := &a.modpacks
	a.page, a.downloadPage, p.mode = pageDownloads, downloadModpacks, modpackLocal
	p.prepared = prepareDesktopTestPack(t, "history.zip", "manifest.json",
		`{"manifestType":"minecraftModpack","manifestVersion":1,"name":"History","version":"1","minecraft":{"version":"1.21.1"},"files":[{"projectID":30,"fileID":40,"required":true}]}`)
	i := a.state.Instances[1]
	row := taskRecord{ID: "history-retry", InstanceID: i.ID, Revision: i.Revision, Version: i.Version, Status: taskCanceled, Pack: launcher.PackUpdateReference(i, p.prepared)}
	p.retry, p.target, p.key = &packRetry{row: row}, fmt.Sprint(i.ID), "old-test-key"
	denial := fmt.Errorf("metadata: %w", launcher.ErrCurseForgeAccessDenied)
	p.setInstallError(denial, p.key)
	tt.Frame()
	if _, ok := tt.Find("预览实例更新"); ok {
		t.Fatal("rejected key can still submit the preview")
	}
	if err := tt.Click("更换 API Key…"); err != nil || !p.keyOpen {
		t.Fatal("key editor not reachable", err)
	}
	p.keyDraft = "replacement-test-key"
	tt.Frame()
	if err := tt.Click("应用"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	if n, ok := tt.Access("预览实例更新"); !ok || n.Disabled || p.installErr != nil || p.key != "replacement-test-key" ||
		p.retry == nil || p.retry.row.ID != row.ID || p.target != fmt.Sprint(i.ID) || p.update != nil || a.busy {
		t.Fatal("replacing the key must return to the original target for an explicit preview")
	}
	p.setInstallError(denial, "old-test-key")
	if p.installErr != nil {
		t.Fatal("a late denial for the old key invalidated its replacement")
	}
}

func TestCompactModpackRetryKeepsActionsAboveFailureNotice(t *testing.T) {
	a, tt := testDesktop(t)
	tt.SetSize(820, 530)
	i := a.state.Instances[1]
	path := filepath.Join(i.GameDir, strings.Repeat("归档副本及迁移记录", 6), strings.Repeat("整合包备份", 8), strings.Repeat("整合包备份", 8), "客户端更新.mrpack")
	row := taskRecord{InstanceID: i.ID, Revision: i.Revision, Name: i.Name, Version: i.Version, Status: taskFailed}
	row.Pack.Archive, row.Pack.GameDir = path, i.GameDir
	row.Pack.SHA256, row.Pack.Source = strings.Repeat("a", 64), string(launcher.ModrinthModpacks)
	a.page, a.downloadPage, a.modpacks.mode = pageDownloads, downloadModpacks, modpackLocal
	a.modpacks.retry = &packRetry{row: row, path: path}
	detail := "open " + path + ": file not found"
	a.modpacks.prepareErr = detail
	a.notice, a.noticeError = "更新整合包 · 已失败："+detail, true
	tt.Frame()

	if err := tt.Click("重新读取"); err != nil {
		t.Fatal(err)
	}
	locate, ok := tt.Find("定位原整合包…")
	notice, hasNotice := tt.Find("复制通知详情")
	if !ok || !hasNotice || locate.Y+locate.H > notice.Y {
		t.Fatalf("relocate action overlaps the failure notice: %+v / %+v", locate, notice)
	}
	if err := tt.Click("复制整合包读取错误"); err != nil || tt.Clipboard() != detail {
		t.Fatalf("could not copy the complete error: %v", err)
	}
}

func TestInstallExistingInstancesKeepsDraftAndLibraryFilter(t *testing.T) {
	a, tt := testDesktop(t)
	a.state.Instances[0].Version, a.state.Instances[1].Version = "1.21.11", "1.21.1"
	a.state.Instances[1].Loader, a.state.Instances[1].LoaderVersion = launcher.LoaderFabric, "0.16.14"
	a.state.InstanceGroups = map[int]string{1: "朋友"}
	a.libraryVersion, a.state.LibraryVersion = "1.21.11", "1.21.11"
	a.releases = []launcher.Version{{ID: "1.21.11"}, {ID: "1.21.1"}}
	a.openInstall("1.21.1", false)
	p := &a.install
	p.name, p.group, p.query = "新草稿", "朋友", "1.21.1"
	tt.SetSize(820, 530)
	tt.Frame()

	if err := tt.Click("查看已有实例"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("建筑测试") || tt.HasText("生存世界") {
		t.Fatal("scoped list mixed versions or applied the saved library filter", tt.Texts())
	}
	if err := tt.Click("建筑测试"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.inspected != 2 || a.state.Selected != 1 || a.libraryVersion != "1.21.11" || a.state.LibraryVersion != "1.21.11" {
		t.Fatal("browsing changed the launch target or the library filter")
	}
	if err := tt.Click("返回安装"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.page != pageDownloads || a.downloadPage != downloadGame || p.existing != nil ||
		p.version != "1.21.1" || p.name != "新草稿" || p.group != "朋友" || p.query != "1.21.1" {
		t.Fatal("returning did not preserve the install draft")
	}
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
	create, cOK := tt.Find("仅创建实例")
	install, iOK := tt.Find("安装实例")
	if !vOK || !cOK || !iOK || version.X+version.W >= create.X || create.X+create.W > install.X ||
		create.Y+create.H > 530 || install.Y+install.H > 530 {
		t.Fatalf("the version list and creation actions do not fit in the compact window: %+v / %+v / %+v", version, create, install)
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
	if err := tt.Click("搜索游戏版本"); err != nil {
		t.Fatal(err)
	}
	tt.Type("1.7.10")
	for _, label := range []string{"仅创建实例", "安装实例"} {
		if _, ok := tt.Find(label); ok {
			t.Fatalf("a hidden version still offers %s", label)
		}
	}
	capture(t, tt, "install-search-empty")
	a.catalogErr = "连接超时"
	tt.Frame()
	if err := tt.Click("复制版本列表错误"); err != nil || tt.Clipboard() != a.catalogErr {
		t.Fatal("empty search hid the failed catalog refresh")
	}
	capture(t, tt, "install-search-refresh-error")
	a.catalogErr = ""
	if err := tt.Click("清除搜索"); err != nil || p.version != "1.21.4" {
		t.Fatalf("clearing search lost the visible selection: %v", err)
	}
	a.state.Instances[1].Loader, a.state.Instances[1].LoaderVersion = launcher.LoaderForge, "54.0.1"
	a.inspect(a.state.Instances[1])
	tt.Frame()
	if err := tt.Click("加载器"); err != nil {
		t.Fatal(err)
	}
	edit := a.form.loaders
	if edit == nil || edit.kind() != launcher.LoaderForge {
		t.Fatal("did not open the existing loader family")
	}
	edit.loaderGame, edit.loaderKind, edit.loaderVersion = edit.version, edit.kind(), "54.0.2"
	edit.loaderVersions = []launcher.LoaderRelease{{Version: edit.loaderVersion}}
	tt.Frame()
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	capture(t, tt, "edit-loader-compact")
	if err := tt.Click("取消"); err != nil || a.form.open || a.state.Instances[1].LoaderVersion != "54.0.1" ||
		a.state.Selected != 1 || p.loaderVersion != "21.4.120" {
		t.Fatalf("cancel changed the instance or independent install draft: %v", err)
	}
}

func TestResourceDownloadsKeepInstallTargetSeparate(t *testing.T) {
	a, tt := testDesktop(t)
	a.state.Instances[0].Loader, a.state.Instances[0].LoaderVersion = launcher.LoaderForge, "52.1.21"
	a.state.Instances[1].Loader, a.state.Instances[1].LoaderVersion = launcher.LoaderNeoForge, "21.4.120"
	a.catalogErr = "无法读取官方版本列表"
	tt.SetSize(820, 530)
	for _, label := range []string{"下载", "模组"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := tt.Find("重试游戏版本列表"); !ok {
		t.Fatal("unavailable version filter cannot be retried from resource search")
	}
	capture(t, tt, "resource-version-filter-retry")
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
	p.err = "GET https://api.modrinth.com/v2/search?facets=" + strings.Repeat("%2Cversions%3A1.21.1", 30) + ": context deadline exceeded"
	tt.Frame()
	if b, ok := tt.Find("重试搜索"); !ok || b.W == 0 || b.H == 0 || b.Y+b.H > 530 {
		t.Fatal("long diagnostics hid the catalog retry action")
	}
	if err := tt.Click("复制搜索错误"); err != nil || tt.Clipboard() != p.err {
		t.Fatal("full catalog diagnostic unavailable")
	}
	capture(t, tt, "catalog-search-recovery")
	p.err = ""
	saved := p.result
	p.page, p.result = 1, launcher.PackSearch{Total: 19}
	tt.Frame()
	if err := tt.Click("上一页"); err != nil {
		t.Fatalf("an empty later page hid the return action: %v", err)
	}
	p.page, p.result = 0, saved
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
	for _, label := range []string{"模组下载来源", "CurseForge", "设置 API Key…"} {
		if err := tt.Click(label); err != nil {
			t.Fatalf("open CurseForge key settings via %s: %v", label, err)
		}
	}
	if p.target.ID != 2 || a.state.Selected != 1 || !a.modpacks.keyOpen {
		t.Fatal("source change lost target or key prompt")
	}
	a.modpacks.keyDraft = "test-key"
	if err := tt.Click("应用"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("搜索排序"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("热门"); err != nil || p.order != "popularity" {
		t.Fatalf("select CurseForge popularity: %v", err)
	}
	project = launcher.PackProject{ID: "248787", Title: "AppleSkin", Author: "squeek502", Description: "在 HUD 中显示食物的饱和度与回复效果。"}
	p.searched, p.project, p.result = true, project, launcher.PackSearch{Hits: []launcher.PackProject{project}, Total: 1}
	const modFile = "appleskin-neoforge-mc1.21.3-3.0.8.jar"
	p.version, p.versions = "7517384", []launcher.PackVersion{{ID: "7517384", ProjectID: project.ID, Number: modFile, Type: "release", Files: []launcher.PackFile{{Filename: modFile, Size: 79857}}}}
	tt.Advance(300 * time.Millisecond)
	capture(t, tt, "curseforge-mod-compact")
	tt.SetDark(true)
	tt.Advance(300 * time.Millisecond)
	capture(t, tt, "curseforge-mod-dark")
	if _, ok := tt.Find("加入队列"); !ok {
		t.Fatal("CurseForge queue action missing")
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

func TestResourcePrerequisiteNavigationKeepsSelectedInstance(t *testing.T) {
	a, tt := testDesktop(t)
	tt.SetSize(820, 530)
	i := &a.state.Instances[1]
	i.Loader, i.LoaderVersion = launcher.LoaderNeoForge, "21.4.120"
	a.openResourceDownloads(*i, launcher.Mods)
	p := a.browserFor(launcher.Mods)
	dep := launcher.ModDependency{ProjectID: "balm", VersionID: "pinnedBalm", Type: "required"}
	project := launcher.PackProject{ID: "waystones", Title: "Waystones"}
	p.project, p.searched = project, true
	p.result = launcher.PackSearch{Hits: []launcher.PackProject{project}, Total: 1}
	p.versions = []launcher.PackVersion{
		{ID: "withLibrary", Number: "需要前置的版本", Files: []launcher.PackFile{{Filename: "waystones.jar"}}, Dependencies: []launcher.ModDependency{dep, {ProjectID: "optional", Type: "optional"}}},
		{ID: "noLibrary", Number: "独立版本", Files: []launcher.PackFile{{Filename: "standalone.jar"}}},
	}
	p.version = "withLibrary"
	p.dependencyNames = &dependencyNames{version: p.version, projects: map[string]launcher.PackProject{"balm": {ID: "balm", Title: "Balm"}}}
	tt.Frame()
	if err := tt.Click("查看前置 (1)"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	capture(t, tt, "download-prerequisites")
	if err := tt.Click("Balm"); err != nil {
		t.Fatal(err)
	}
	if p.dependency == nil || *p.dependency != dep || p.project.ID != "balm" || p.target.ID != i.ID || a.state.Selected != 1 || a.busy {
		t.Fatal("opening a prerequisite did not preserve the target, or attempted to install files")
	}
	if err := tt.Click("返回上一级"); err != nil {
		t.Fatal(err)
	}
	if p.project.ID != project.ID || p.version != "withLibrary" || p.dependency != nil {
		t.Fatal("going back did not restore the original project and version")
	}
	if err := tt.Click("独立版本"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("查看前置 (1)"); ok {
		t.Fatal("switching to a different version retained its predecessor's dependencies")
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
