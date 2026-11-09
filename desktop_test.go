package main

import (
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/display"
)

func TestDesktopNavigationKeepsLaunchTarget(t *testing.T) {
	a, tt := testDesktop(t)
	capture(t, tt, "home")
	tt.Key(0, ui.KeyF1)
	mods, key, _ := display.HotkeyBinding(searchHotkey, display.HotkeyAuto)
	tt.Key(mods, key)
	capture(t, tt, "instances")
	tt.Type("建筑")
	if a.page != pageLibrary || a.query != "建筑" {
		t.Fatalf("global search did not focus the instance query: page %d, query %q", a.page, a.query)
	}
	if _, ok := tt.Find("生存世界"); ok {
		t.Fatal("search did not hide the unmatched instance")
	}
	if err := tt.Click("建筑测试"); err != nil {
		t.Fatal(err)
	}
	if a.inspected != 2 || a.state.Selected != 1 {
		t.Fatalf("browse target %d, launch target %d", a.inspected, a.state.Selected)
	}
	capture(t, tt, "instance-detail")
	if err := tt.Click("更多实例操作"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("重命名"); !ok {
		t.Fatal("rename action is missing from the instance menu")
	}
	capture(t, tt, "instance-menu")
	tt.Key(0, ui.KeyEscape)
	if a.inspected != 2 {
		t.Fatal("Escape should dismiss the menu before leaving the instance")
	}
	tt.Key(0, ui.KeyF2)
	if !a.form.open || a.form.target.ID != 2 {
		t.Fatalf("F2 did not open the viewed instance: %+v", a.form)
	}
	capture(t, tt, "rename")
	tt.Key(0, ui.KeyEscape)
	if a.form.open || a.inspected != 2 {
		t.Fatalf("first Escape should only close the editor; inspected = %d", a.inspected)
	}
	tt.Key(0, ui.KeyEscape)
	if a.inspected != 0 || a.state.Selected != 1 {
		t.Fatalf("second Escape should return to the list; inspected = %d, selected = %d", a.inspected, a.state.Selected)
	}
}

func TestDesktopTasksReachableDuringDownload(t *testing.T) {
	a, tt := testDesktop(t)
	a.busy, a.taskName = true, "建筑测试"
	a.task = launcher.Progress{Stage: "下载资源", Done: 8, Total: 24}
	a.history = []taskRecord{{
		Name: "生存世界", Version: "1.21.1", Action: "启动游戏", Status: "已失败",
		Detail: "请检查 Java 设置后重试。", At: time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC),
	}}
	tt.SetSize(820, 530)
	tt.SetDark(true)
	tt.Advance(500 * time.Millisecond)
	tt.Frame()
	capture(t, tt, "home-dark-compact")
	if err := tt.Click("查看任务"); err != nil {
		t.Fatal(err)
	}
	if a.page != pageLibrary || a.libraryPage != 1 || a.state.Selected != 1 {
		t.Fatalf("unexpected task navigation: page %d, tab %d, target %d", a.page, a.libraryPage, a.state.Selected)
	}
	if _, ok := tt.Find("历史记录"); !ok {
		t.Fatal("task history was not rendered")
	}
	capture(t, tt, "tasks-dark-compact")
	a.logOpen, a.logPath, a.logText = true, "launcher.log", "Minecraft is starting."
	tt.Frame()
	if _, ok := tt.Find("游戏日志"); !ok {
		t.Fatal("the log viewer did not open")
	}
	capture(t, tt, "log-dark-compact")
	tt.Key(0, ui.KeyEscape)
	if a.logOpen {
		t.Fatal("Escape did not close the log viewer")
	}
	a.busy = false
	a.task = launcher.Progress{}
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("浏览…"); !ok {
		t.Fatal("native game-directory picker has no entry point")
	}
	capture(t, tt, "settings-dark-compact")
}

func TestCompactTitleBarCentersNavigationWithWindowsControls(t *testing.T) {
	_, tt := testDesktop(t)
	for _, width := range []int{820, 1140} {
		tt.SetSize(width, 580)
		tt.Frame()
		bounds, ok := tt.Find("主导航")
		if !ok {
			t.Fatalf("navigation missing at %d DIP", width)
		}
		middle := bounds.X + bounds.W/2
		if math.Abs(float64(middle)-float64(width)/2) > 2 {
			t.Fatalf("navigation center %.1f is not at window center %.1f", middle, float64(width)/2)
		}
	}
}

func TestCompactSettingsAndLicenses(t *testing.T) {
	a, tt := testDesktop(t)
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("应用设置"); ok {
		t.Fatal("save actions should only appear when there are pending changes")
	}
	a.memory = "6144"
	tt.Frame()
	if err := tt.Click("还原更改"); err != nil {
		t.Fatal(err)
	}
	if a.memory != "4096" {
		t.Fatalf("restored memory = %q", a.memory)
	}
	tt.Key(0, ui.KeyF1)
	if err := tt.Click("查看许可证"); err != nil {
		t.Fatal(err)
	}
	if !a.licensesOpen || a.licenseFile != "LICENSE" {
		t.Fatal("licenses did not open on the project license")
	}
	capture(t, tt, "licenses")
	tt.Key(0, ui.KeyEscape)
	if a.licensesOpen {
		t.Fatal("Escape did not dismiss the licenses")
	}
}

func TestInstanceFilterAndVersionEditor(t *testing.T) {
	a, tt := testDesktop(t)
	a.releases = []launcher.Version{{ID: "1.21.4"}, {ID: "1.21.1"}}
	if err := tt.Click("版本"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("待下载"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("生存世界"); ok {
		t.Fatal("cached instance appeared in the download filter")
	}
	capture(t, tt, "instance-filter")
	if err := tt.Click("建筑测试"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("更换版本"); err != nil {
		t.Fatal(err)
	}
	if !a.form.open || a.form.target.ID != 2 || a.form.version != "1.21.4" || a.state.Selected != 1 {
		t.Fatalf("version editor bound to the wrong instance: %+v", a.form)
	}
	capture(t, tt, "instance-version")
	a.form.version = "1.20.1"
	tt.Frame()
	if err := tt.Click("保存版本"); err != nil {
		t.Fatal(err)
	}
	if !a.form.open || a.form.err == "" || a.state.Instances[1].Version != "1.21.4" {
		t.Fatal("an unsupported version was accepted")
	}
}

func TestTaskCenterFiltersAndDisablesStaleRetry(t *testing.T) {
	a, tt := testDesktop(t)
	i := a.state.Instances[1]
	a.history = []taskRecord{
		{InstanceID: i.ID, Revision: i.Revision, Name: i.Name, Version: i.Version, Action: taskVerify, Status: taskFailed, At: time.Now()},
		{Name: "已完成的任务", Version: "1.21.1", Action: taskVerify, Status: taskCompleted, At: time.Now().Add(-time.Minute)},
	}
	if err := tt.Click("任务中心"); err != nil {
		t.Fatal(err)
	}
	if a.page != pageLibrary || a.libraryPage != 1 {
		t.Fatal("title bar action did not open the task center")
	}
	if err := tt.Click("主页"); err != nil {
		t.Fatal(err)
	}
	mods, key, _ := display.HotkeyBinding(tasksHotkey, display.HotkeyAuto)
	tt.Key(mods, key)
	if a.page != pageLibrary || a.libraryPage != 1 || a.state.Selected != 1 {
		t.Fatal("task shortcut did not preserve the launch selection")
	}
	if err := tt.Click("失败 / 取消"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("已完成的任务"); ok {
		t.Fatal("completed task appeared in the failed filter")
	}
	if _, ok := tt.Find("重试"); !ok {
		t.Fatal("unchanged task target cannot be retried")
	}
	capture(t, tt, "task-filter")
	a.state.Instances[1].Revision++
	tt.Frame()
	if _, ok := tt.Find("重试"); ok {
		t.Fatal("a task for an obsolete revision can still be retried")
	}
}

func TestInstanceSettingsKeepDraftAndLaunchTargetSeparate(t *testing.T) {
	a, tt := testDesktop(t)
	tt.SetSize(820, 530)
	tt.SetDark(true)
	tt.Frame()
	for _, name := range []string{"版本", "建筑测试", "启动设置", "8 GiB"} {
		if err := tt.Click(name); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := tt.Find("应用并启动"); !ok {
		t.Fatal("pending settings have no apply-and-launch action")
	}
	if a.state.Selected != 1 || a.state.Instances[1].Launch.Memory != 0 {
		t.Fatal("editing an instance changed the launch target or applied the draft")
	}
	capture(t, tt, "instance-launch-dark")
	if err := tt.Click("游戏文件"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("打开存档目录"); !ok {
		t.Fatal("game files has no saves shortcut")
	}
	capture(t, tt, "instance-files-dark")
	if err := tt.Click("启动设置"); err != nil {
		t.Fatal(err)
	}
	if a.launchDrafts[2].fields.memory != "8192" {
		t.Fatal("switching tabs lost the instance draft")
	}
	if err := tt.Click("还原更改"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("应用并启动"); ok {
		t.Fatal("reverting the draft left an apply-and-launch action")
	}
	tt.Key(0, ui.KeyEscape)
	if a.detailPage != detailOverview || a.inspected != 2 {
		t.Fatal("Escape did not return to the instance overview")
	}
	if err := tt.Click("更多实例操作"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("复制实例配置…"); err != nil {
		t.Fatal(err)
	}
	if a.form.target.ID != 2 || a.form.mode != "duplicate" || a.state.Selected != 1 {
		t.Fatal("copy dialog was not bound to the viewed instance")
	}
}

func testDesktop(t *testing.T) (*desktop, *ui.Tester) {
	t.Helper()
	logo, err := ui.DecodeImage(logoPNG)
	if err != nil {
		t.Fatal(err)
	}
	hero, err := ui.DecodeImage(homePNG)
	if err != nil {
		t.Fatal(err)
	}
	a := &desktop{
		logo: logo, hero: hero, ready: true, installed: map[int]bool{1: true},
		state: launcher.State{Selected: 1, Settings: launcher.Settings{Player: "Player", Memory: 4096, GameRoot: t.TempDir()},
			Instances: []launcher.Instance{
				{ID: 1, Name: "生存世界", Version: "1.21.1", Loader: "Vanilla", GameDir: t.TempDir()},
				{ID: 2, Name: "建筑测试", Version: "1.21.4", Loader: "Vanilla", GameDir: t.TempDir()},
			}},
	}
	a.draft, a.memory = a.state.Settings, "4096"
	tt := ui.NewTesterWith(a.view, 1140, 720, ui.TesterOptions{Preferences: ui.Preferences{ReduceMotion: true}})
	tt.SetTitleBar(ui.TitleBar{Height: titleBarHeight, Right: 138})
	tt.Frame()
	return a, tt
}

func capture(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("LXMCL_UI_CAPTURE")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
