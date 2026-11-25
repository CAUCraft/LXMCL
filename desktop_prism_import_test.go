package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/content"
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
	source := t.TempDir()
	for name, data := range map[string]string{
		"instance.cfg":                      "name=Island\nOverrideMemory=true\nMaxMemAlloc=3584\nOverrideWindow=true\nMinecraftWinWidth=1280\nMinecraftWinHeight=720\n",
		"mmc-pack.json":                     `{"formatVersion":1,"components":[{"uid":"net.minecraft","version":"1.21.4"}]}`,
		".minecraft/options.txt":            "music:0.5",
		".minecraft/servers.dat":            "test server list",
		".minecraft/saves/Island/level.dat": "test world",
		".minecraft/screenshots/岛屿.PNG":     "preview only reads file metadata",
	} {
		file := filepath.Join(source, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var err error
	d.source, err = content.ReadPrismInstance(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
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
	capture(t, tt, "prism-directory-screenshots-default-light")
	click(option)
	tt.SetDark(true)
	tt.Frame()
	check(true)
	capture(t, tt, "prism-directory-screenshots-selected-dark")
	click("取消")
	click("导入本地实例…")
	if _, visible := tt.Access(option); visible || a.form.prism.screenshots {
		t.Fatal("a new import inherited the previous screenshot choice")
	}
}
