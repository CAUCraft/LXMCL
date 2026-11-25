package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/CAUCraft/LXMCL/internal/screenshots"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/display"
)

func TestScreenshotSearchFromGallery(t *testing.T) {
	a, tt := testDesktop(t)
	a.state.Instances[0].Name, a.state.Instances[1].Name = "同名实例", "同名实例"
	first, second := a.state.Instances[0], a.state.Instances[1]
	const night = "青河-NIGHT.png"
	const dawn = "青河-Dawn.png"
	const woods = "林地.png"
	at := time.Date(2026, 11, 25, 11, 0, 0, 0, time.UTC)
	sources := make(map[string][]byte)
	add := func(i launcher.Instance, name string, w, h int, tone color.RGBA, modified time.Time) {
		t.Helper()
		dir := filepath.Join(i.GameDir, screenshots.Directory)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		var data bytes.Buffer
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.Draw(img, img.Bounds(), image.NewUniform(tone), image.Point{}, draw.Src)
		if err := png.Encode(&data, img); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name)
		sources[path] = data.Bytes()
		if err := os.WriteFile(path, sources[path], 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	add(first, night, 110, 110, color.RGBA{70, 136, 106, 255}, at)
	add(second, woods, 320, 180, color.RGBA{63, 121, 81, 255}, at)
	add(second, night, 240, 160, color.RGBA{68, 100, 149, 255}, at.Add(-time.Hour))
	add(second, dawn, 270, 170, color.RGBA{188, 126, 92, 255}, at.Add(-2*time.Hour))

	tt.SetSize(820, 530)
	tt.SetPreferences(ui.Preferences{})
	click := func(labels ...string) {
		t.Helper()
		for _, label := range labels {
			if err := tt.Click(label); err != nil {
				t.Fatalf("%s: %v (%v)", label, err, tt.Texts())
			}
			tt.Frame()
		}
	}
	gallery := func(i launcher.Instance) *screenshotGallery {
		t.Helper()
		click("截图")
		p := a.screenshots
		if p.instance != i || p.query != "" || p.selected != "" {
			t.Fatal("the new gallery reused a different instance's search or preview")
		}
		entries, err := screenshots.List(t.Context(), i.GameDir)
		if err != nil {
			t.Fatal(err)
		}
		p.entries = make(map[string]screenshots.Entry, len(entries))
		for _, entry := range entries {
			p.entries[entry.Name] = entry
			p.rows = append(p.rows, display.ListItem{Key: entry.Name, Label: entry.Name, Description: entry.Modified.Local().Format("2006-01-02 15:04")})
		}
		p.loading, p.folder, p.selected = false, true, entries[0].Name
		tt.Frame()
		return p
	}
	preview := func(p *screenshotGallery) {
		t.Helper()
		img, original, err := screenshots.Preview(t.Context(), p.instance.GameDir, p.selected)
		if err != nil {
			t.Fatal(err)
		}
		p.picture, p.original = ui.NewBitmap(img), original
		tt.Frame()
	}
	search := func(query string) {
		t.Helper()
		tt.Key(ui.Cmd, ui.KeyF)
		if !tt.Focused("搜索截图文件名") {
			t.Fatal("Ctrl+F did not focus screenshot search")
		}
		tt.Key(ui.Cmd, ui.KeyA)
		if query == "" {
			tt.Key(0, ui.KeyBackspace)
		} else {
			tt.Type(query)
		}
		tt.Frame()
	}
	check := func(p *screenshotGallery, want ...string) {
		t.Helper()
		var got []string
		for _, row := range p.visibleRows() {
			got = append(got, row.Key)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("visible screenshots = %v, want %v", got, want)
		}
	}

	click("实例", "1.21.4 · 原版")
	p := gallery(second)
	preview(p)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p.cancel = cancel
	search("青河")
	check(p, night, dawn)
	if ctx.Err() == nil || p.selected != "" || p.original != nil || !tt.HasText("选择截图查看预览") {
		t.Fatal("filtered-out selection retained its preview", tt.Texts())
	}
	for _, label := range []string{"复制图片", "导出原图…"} {
		if el, ok := tt.Access(label); !ok || !el.Disabled {
			t.Fatalf("hidden selection left %s available", label)
		}
	}
	if tt.HasText(woods) {
		t.Fatal("filtered screenshot stayed visible")
	}
	click(night)
	preview(p)
	search("  nIgHt ")
	check(p, night)
	if p.selected != night || !tt.HasText("240 × 160") {
		t.Fatal("matching selection lost its preview", tt.Texts())
	}
	tt.Advance(500 * time.Millisecond)
	capture(t, tt, "screenshot-filename-match-light")

	click("查看大图")
	tt.Key(0, ui.KeyRight)
	if p.selected != night || !tt.HasText("1 / 1") {
		t.Fatal("viewer left the filtered results", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	if p.viewerFrom != "" || p.query != "  nIgHt " {
		t.Fatal("leaving the viewer lost the gallery search")
	}

	search("不存在")
	check(p)
	if !tt.HasText("没有匹配的截图") {
		t.Fatal("missing search empty state", tt.Texts())
	}
	tt.SetDark(true)
	tt.Advance(500 * time.Millisecond)
	capture(t, tt, "screenshot-filename-empty-dark")
	search("")
	check(p, woods, night, dawn)
	search("不存在")
	click("清除截图搜索")
	check(p, woods, night, dawn)
	if el, ok := tt.Access("打开截图文件夹"); !ok || el.Disabled {
		t.Fatal("search affected opening the current screenshot folder")
	}

	search("dawn")
	click("1.21.1 · 原版 · 启动目标")
	other := gallery(first)
	check(other, night)
	preview(other)
	if a.screenshots == p || a.state.Selected != first.ID || !tt.HasText("110 × 110") || tt.HasText("240 × 160") {
		t.Fatal("changing instances reused the old search or preview, or switched the launch target")
	}
	click("批量导出…", "选择截图 "+night)
	if selected := other.batchSelection(other.visibleRows()); len(selected) != 1 || selected[0].Name != night || other.selected != night || other.original == nil {
		t.Fatal("batch selection disrupted the single-image preview", tt.Texts())
	}
	search("不存在")
	click("清除截图搜索")
	if len(other.batch) != 0 {
		t.Fatal("clearing search brought back a hidden batch selection", tt.Texts())
	}
	click("退出批量")
	for path, before := range sources {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("search modified a source screenshot", err)
		}
	}
}
