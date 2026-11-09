package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
)

func TestDesktopNavigationKeepsLaunchTarget(t *testing.T) {
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
		state: launcher.State{Selected: 1, Settings: launcher.Settings{Player: "Player"},
			Instances: []launcher.Instance{
				{ID: 1, Name: "生存世界", Version: "1.21.1", Loader: "Vanilla", GameDir: t.TempDir()},
				{ID: 2, Name: "建筑测试", Version: "1.21.4", Loader: "Vanilla", GameDir: t.TempDir()},
			}},
	}
	tt := ui.NewTesterWith(a.view, 1140, 720, ui.TesterOptions{Preferences: ui.Preferences{ReduceMotion: true}})
	tt.SetTitleBar(ui.TitleBar{Height: 54, Right: 138})
	tt.Frame()
	capture(t, tt, "home")
	if err := tt.Click("版本"); err != nil {
		t.Fatal(err)
	}
	capture(t, tt, "instances")
	// Browse the second instance while the first remains the home launch target.
	a.query = "建筑"
	tt.Frame()
	if err := tt.Click("管理"); err != nil {
		t.Fatal(err)
	}
	if a.inspected != 2 || a.state.Selected != 1 {
		t.Fatalf("browse target %d, launch target %d", a.inspected, a.state.Selected)
	}
	tt.Key(0, ui.KeyF2)
	if !a.form.open || a.form.target.ID != 2 {
		t.Fatalf("F2 did not open the viewed instance: %+v", a.form)
	}
	capture(t, tt, "rename")
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
