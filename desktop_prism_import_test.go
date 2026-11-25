package main

import (
	"path/filepath"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func TestPrismImportScreenshotOption(t *testing.T) {
	a, tt := testDesktop(t)
	a.state.InstanceGroups = map[int]string{1: "建筑"}
	tt.SetSize(820, 530)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(label, err, tt.Texts())
		}
		tt.Frame()
	}
	const option = "游戏截图（PNG）"
	click("实例")
	click("导入本地实例…")
	d := a.form.prism
	d.source = content.PrismInstance{Source: filepath.Join(t.TempDir(), "Island.zip"), Name: "Island", Version: "1.21.4",
		Worlds: true, Options: true, Servers: true, Screenshots: true,
		Launch: instance.LaunchOptions{Memory: 3584, Display: instance.DisplayResolution, Width: 1280, Height: 720}}
	d.options = true
	a.form.name = d.source.Name
	tt.Frame()
	check := func(selected bool) {
		t.Helper()
		n, ok := tt.Access(option)
		if !ok || n.Disabled || n.Checked != selected || d.screenshots != selected {
			t.Fatal("unexpected screenshot option", n, tt.Texts())
		}
		for _, label := range []string{"复制目标目录", option, "取消", "确认导入"} {
			n, ok := tt.Access(label)
			if !ok || n.Disabled || n.Bounds.Y < 42 || n.Bounds.Y+n.Bounds.H > 530 {
				t.Fatalf("clipped control %q: %+v", label, n)
			}
		}
	}
	check(false)
	capture(t, tt, "prism-import-screenshots-default-light")
	click(option)
	tt.SetDark(true)
	tt.Frame()
	check(true)
	capture(t, tt, "prism-import-screenshots-selected-dark")
	click("取消")
	click("导入本地实例…")
	if _, visible := tt.Access(option); visible || a.form.prism.screenshots {
		t.Fatal("a new import inherited the previous screenshot choice")
	}
}
