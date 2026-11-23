package main

import (
	"compress/gzip"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
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
	if node, ok := tt.Access("打开世界目录"); !ok || node.Disabled {
		t.Fatal("visible world did not enable its folder action")
	}
	click("搜索世界目录")
	tt.Type("河谷")
	if p.selected != 1 || p.visible[p.selected].Folder() != "河谷矿场" {
		t.Fatal("matching selection did not follow its folder", p.selected)
	}
	tt.Type("小屋")
	if p.selected != -1 || len(p.visible) != 1 || p.visible[0].Folder() != "河谷小屋" {
		t.Fatal("hidden selection was transferred to another world", p.selected)
	}
	for _, label := range []string{"打开世界目录", "备份所选世界", "进入世界"} {
		if node, ok := tt.Access(label); !ok || !node.Disabled {
			t.Fatal("hidden selection left an enabled action", label)
		}
	}
	click("Clear")
	if p.selected != -1 || len(p.visible) != 3 || p.visible[0].Folder() != "Alpha" {
		t.Fatal("clearing search did not restore the original list", p.selected)
	}
}

func TestWorldSortUsesSavedTimeAndKeepsFolderSelection(t *testing.T) {
	game := t.TempDir()
	for _, fixture := range []struct {
		folder string
		played uint64
	}{
		{"00 旧世界", 0},
		{"01 河谷", 1000},
		{"02 河谷", 2000},
		{"03 河谷", 2000},
		{"04 旧世界", 0},
	} {
		dir := filepath.Join(game, "saves", fixture.folder)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		file, err := os.Create(filepath.Join(dir, "level.dat"))
		if err != nil {
			t.Fatal(err)
		}
		z := gzip.NewWriter(file)
		// A named root compound with a Data compound and a LastPlayed long.
		nbt := binary.BigEndian.AppendUint64([]byte("\x0a\x00\x00\x0a\x00\x04Data\x04\x00\x0aLastPlayed"), fixture.played)
		_, writeErr := z.Write(append(nbt, 0, 0))
		gzipErr, fileErr := z.Close(), file.Close()
		if writeErr != nil || gzipErr != nil || fileErr != nil {
			t.Fatal(writeErr, gzipErr, fileErr)
		}
	}
	worlds, err := saves.Worlds(t.Context(), game)
	if err != nil || len(worlds) != 5 {
		t.Fatal(worlds, err)
	}
	p := &worldBrowser{entries: worlds, visible: worlds, selected: 1, order: sortRecent}
	check := func(selected string, order ...string) {
		t.Helper()
		p.filter()
		var got []string
		for _, w := range p.visible {
			got = append(got, w.Folder())
		}
		world, _ := p.selectedWorld()
		if !slices.Equal(got, order) || world.Folder() != selected {
			t.Fatal(got, world.Folder(), "want", order, selected)
		}
	}
	check("01 河谷", "02 河谷", "03 河谷", "01 河谷", "00 旧世界", "04 旧世界")
	p.query = "河谷"
	check("01 河谷", "02 河谷", "03 河谷", "01 河谷")
	p.query = "无匹配"
	check("")
	p.query, p.order = "", ""
	check("", "00 旧世界", "01 河谷", "02 河谷", "03 河谷", "04 旧世界")
}
