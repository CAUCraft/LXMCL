package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/saves"
)

func TestWorldSearchKeepsOnlyVisibleSelection(t *testing.T) {
	a, tt := testDesktop(t)
	i := a.state.Instances[0]
	for _, name := range []string{"Alpha", "河谷小屋", "河谷矿场"} {
		dir := filepath.Join(i.GameDir, "saves", name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "level.dat"), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	worlds, err := saves.Worlds(t.Context(), i.GameDir)
	if err != nil || len(worlds) != 3 {
		t.Fatal(worlds, err)
	}
	p := &worldBrowser{entries: worlds, visible: worlds, selected: -1}
	a.form = editor{open: true, mode: "worlds", target: i, worlds: p}
	tt.SetSize(820, 530)
	tt.Frame()
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(label, err)
		}
	}
	click("河谷矿场")
	click("搜索世界目录")
	tt.Type("河谷")
	if p.selected != 1 || p.visible[p.selected].Folder() != "河谷矿场" {
		t.Fatal("matching selection did not follow its folder", p.selected)
	}
	tt.Type("小屋")
	if p.selected != -1 || len(p.visible) != 1 || p.visible[0].Folder() != "河谷小屋" {
		t.Fatal("hidden selection was transferred to another world", p.selected)
	}
	for _, label := range []string{"备份所选世界", "进入世界"} {
		if node, ok := tt.Access(label); !ok || !node.Disabled {
			t.Fatal("hidden selection left an enabled action", label)
		}
	}
	click("Clear")
	if p.selected != -1 || len(p.visible) != 3 || p.visible[0].Folder() != "Alpha" {
		t.Fatal("clearing search did not restore the original list", p.selected)
	}
}
