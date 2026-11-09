package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
)

func TestDesktopNavigationKeepsLaunchTarget(t *testing.T) {
	a, tt := testDesktop(t)
	capture(t, tt, "home")
	if err := tt.Click("版本"); err != nil {
		t.Fatal(err)
	}
	capture(t, tt, "instances")
	if err := tt.Click("搜索实例"); err != nil {
		t.Fatal(err)
	}
	tt.Type("建筑")
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
	tt.SetTitleBar(ui.TitleBar{Height: 54, Right: 138})
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
