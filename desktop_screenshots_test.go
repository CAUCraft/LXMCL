package main

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/screenshots"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/display"
)

func TestScreenshotPageKeepsInstanceSelection(t *testing.T) {
	a, tt := testDesktop(t)
	tt.SetSize(820, 530)
	click := func(s string) {
		t.Helper()
		if err := tt.Click(s); err != nil {
			t.Fatal(err)
		}
		tt.Advance(250 * time.Millisecond)
		tt.Frame()
	}
	click("实例")
	click("建筑测试")
	click("截图")
	p := a.screenshots
	if p == nil || p.instance.ID != 2 || a.state.Selected != 1 {
		t.Fatal("wrong gallery target")
	}
	p.loading = false
	tt.Frame()
	if !tt.HasText("还没有游戏截图") {
		t.Fatal(tt.Texts())
	}
	capture(t, tt, "screenshots-empty-light")
	img := image.NewRGBA(image.Rect(0, 0, 320, 180))
	for y := range 180 {
		for x := range 320 {
			c := color.RGBA{113, 172, 215, 255}
			h := 100 - ((x / 16) * 7 % 36)
			switch {
			case y > h+10:
				c = color.RGBA{73, 107, 71, 255}
			case y > h:
				c = color.RGBA{138, 169, 86, 255}
			case y > 22 && y < 30 && x > 65 && x < 130:
				c = color.RGBA{221, 236, 239, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	p.rows = []display.ListItem{{Key: "河谷.png", Label: "河谷.png", Description: "2026-11-21 14:30"}, {Key: "林地.png", Label: "林地.png", Description: "2026-11-19 11:07"}}
	p.folder, p.selected, p.picture, p.original = true, p.rows[0].Key, ui.NewBitmap(img), &screenshots.Original{Size: img.Bounds().Size()}
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		tt.Frame()
		tt.Advance(500 * time.Millisecond)
		tt.Frame()
		r, ok := tt.Find("截图预览")
		if !ok || r.W < 180 || r.H < 100 || r.X+r.W > 821 || r.Y+r.H > 531 || !tt.HasText("320 × 180") {
			t.Fatal("preview outside window", r, tt.Texts())
		}
		copyButton, ok := tt.Find("复制图片")
		if !ok || copyButton.X < 0 || copyButton.X+copyButton.W > 820 || copyButton.Y+copyButton.H > 530 {
			t.Fatal("copy button outside window", copyButton)
		}
		if dark {
			capture(t, tt, "screenshots-dark")
		} else {
			capture(t, tt, "screenshots-light")
		}
		click("查看大图")
		large, ok := tt.Find("截图预览")
		if !ok || large.W <= r.W || large.H <= r.H || large.X+large.W > 820 || large.Y+large.H > 530 || !tt.HasText("1 / 2") {
			t.Fatal("viewer bounds or position", large, tt.Texts())
		}
		iconAction, ok := tt.Find("用作实例图标…")
		if !ok || iconAction.X < 0 || iconAction.X+iconAction.W > 820 || iconAction.Y+iconAction.H > 530 {
			t.Fatal("screenshot icon action outside window", iconAction, tt.Texts())
		}
		if dark {
			capture(t, tt, "screenshot-viewer-dark")
		} else {
			capture(t, tt, "screenshot-viewer-light")
		}
		tt.Key(0, ui.KeyLeft)
		if p.selected != "河谷.png" {
			t.Fatal("moved past first screenshot")
		}
		if dark {
			tt.Key(0, ui.KeyRight)
			tt.Key(0, ui.KeyRight)
			if p.selected != "林地.png" || p.original != nil || !tt.HasText("正在读取预览…") {
				t.Fatal("wrong selection while loading last screenshot", tt.Texts())
			}
		}
		tt.Key(0, ui.KeyEscape)
		tt.Frame()
		if a.detailPage != detailScreenshots || p.selected != "河谷.png" || p.viewerFrom != "" {
			t.Fatal("Esc did not restore gallery selection")
		}
	}
	click("林地.png")
	if p.selected != "林地.png" || p.picture != nil || p.original != nil {
		t.Fatal("old preview remains selected")
	}
	click("生存世界")
	click("截图")
	if a.screenshots == p || a.screenshots.instance.ID != 1 || a.state.Selected != 1 {
		t.Fatal("previous gallery remained active")
	}
}
