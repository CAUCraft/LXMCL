package main

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"math"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
)

func TestAppearanceStaysWithBrowsedAccount(t *testing.T) {
	a, tt := testDesktop(t)
	const id = "0123456789abcdef0123456789abcdef"
	const cape = "01234567-89ab-cdef-0123-456789abcdef"
	a.microsoftAccount.Profile = launcher.MicrosoftProfile{ID: id, Name: "OnlinePlayer"}
	a.state.MicrosoftProfile = id
	atlas := image.NewRGBA(image.Rect(0, 0, 64, 32))
	for y := 1; y < 17; y++ {
		for x := 1; x < 11; x++ {
			atlas.SetRGBA(x, y, color.RGBA{R: 42, G: uint8(120 + (x+y)%4*25), B: 225, A: 255})
		}
	}
	a.appearance = &appearanceView{id: id, loaded: true, skin: clubSkin, cape: cape,
		data: launcher.Appearance{
			Skin:  launcher.ProfileTexture{URL: "https://textures.minecraft.net/texture/test", Variant: "CLASSIC"},
			Capes: []launcher.ProfileTexture{{ID: cape, Alias: "Demo Cape", State: "ACTIVE"}},
		}, capes: map[string]*image.NRGBA{cape: textureAtlas(atlas)}}
	tt.Frame()
	if err := tt.Click("账号中心"); err != nil {
		t.Fatal(err)
	}
	if a.page != pageAccount {
		t.Fatal("the account center did not open as a standalone page")
	}
	capture(t, tt, "account-3d")
	if err := tt.Click("Microsoft 账号操作"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("重新登录"); !ok {
		t.Fatal("the account menu has no reauthentication action")
	}
	tt.Key(0, ui.KeyEscape)
	if _, ok := tt.Find("重新登录"); ok || a.page != pageAccount {
		t.Fatal("Escape should dismiss the account menu before leaving the page")
	}
	if err := tt.Click("查看背面"); err != nil {
		t.Fatal(err)
	}
	capture(t, tt, "account-3d-cape")
	tt.SetSize(820, 530)
	tt.SetDark(true)
	tt.Advance(500 * time.Millisecond)
	tt.Frame()
	list, leftOK := tt.Find("添加账号")
	skin, rightOK := tt.Find("选择 PNG…")
	if !leftOK || !rightOK || list.X+list.W >= skin.X || skin.X+skin.W > 820 || skin.Y+skin.H > 530 {
		t.Fatalf("account list and preview do not fit the compact window: %+v %+v", list, skin)
	}
	a.appearance.draft, a.appearance.file = []byte("draft"), "新皮肤.png"
	a.appearance.variant, a.appearance.draftSkin = "classic", clubSkin
	tt.Frame()
	capture(t, tt, "account-3d-compact-dark")
	if _, ok := tt.Find("应用皮肤"); !ok {
		t.Fatal("local skin draft has no apply action")
	}
	if err := tt.Click("选择披风"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("不佩戴"); err != nil {
		t.Fatal(err)
	}
	if a.appearance.cape != "" || a.appearance.data.ActiveCape().ID != cape {
		t.Fatal("selecting no cape did not stay in the preview until applied")
	}
	if _, ok := tt.Find("应用披风"); !ok {
		t.Fatal("the new cape selection has no apply action")
	}
	if err := tt.Click("Player"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("孪枭 · 离线预览"); !ok || a.state.MicrosoftProfile != id {
		t.Fatal("browsing an offline skin did not preserve the active Microsoft account")
	}
	capture(t, tt, "account-3d-offline")
	if err := tt.Click("OnlinePlayer"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("新皮肤.png"); !ok {
		t.Fatal("browsing another account discarded the local skin draft")
	}
	if err := tt.Click("返回上一页"); err != nil || a.page != pageHome {
		t.Fatalf("returning from the account center: page %d, %v", a.page, err)
	}
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("账号列表"); ok {
		t.Fatal("account management still appears in general settings")
	}
}

func TestSkinAvatarCompositesHatLayer(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	face := color.RGBA{R: 255, A: 255}
	hat := color.RGBA{B: 255, A: 255}
	img.SetRGBA(8, 8, face)
	img.SetRGBA(9, 8, face)
	img.SetRGBA(41, 8, hat)
	avatar := skinHead(img)
	if avatar.Bounds() != image.Rect(0, 0, 8, 8) || avatar.RGBAAt(0, 0) != face || avatar.RGBAAt(1, 0) != hat {
		t.Fatal("avatar does not match the skin face and hat layer")
	}
}

func TestSkinModelShowsTheCapeOnlyFromBehind(t *testing.T) {
	cape := image.NewNRGBA(image.Rect(0, 0, 64, 32))
	draw.Draw(cape, cape.Bounds(), image.NewUniform(color.NRGBA{255, 0, 0, 255}), image.Point{}, draw.Src)
	model := skinModel(placeholderSkin.atlas, cape, false)
	dst := image.NewRGBA(image.Rect(0, 0, previewWidth, previewHeight))
	depth := make([]float64, previewWidth*previewHeight)
	drawSkinModel(dst, depth, model, 0, 0)
	front := dst.RGBAAt(previewWidth/2, previewHeight/2)
	drawSkinModel(dst, depth, model, math.Pi, 0)
	back := dst.RGBAAt(previewWidth/2, previewHeight/2)
	if front.B <= front.R || back.R <= 180 || back.B != 0 {
		t.Fatalf("expected the cape behind the torso: front=%v back=%v", front, back)
	}
}

func TestModelDragRotatesThePreview(t *testing.T) {
	orbit := frontOrbit()
	tt := ui.NewTesterWith(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			modelStage(c, clubSkin.atlas, nil, false, &orbit)
		})
	}, 320, 320, ui.TesterOptions{Preferences: ui.Preferences{ReduceMotion: true}})
	tt.Frame()
	start := bytes.Clone(tt.Image().Pix)
	tt.Press(145, 156)
	tt.Move(216, 171)
	tt.Release(216, 171)
	if orbit.yaw == frontYaw || orbit.pitch == frontPitch || bytes.Equal(start, tt.Image().Pix) {
		t.Fatal("dragging the model did not update the visible camera")
	}
}
func TestFlightLoadingFollowsReducedMotion(t *testing.T) {
	tester := ui.NewTesterWith(func(c *ui.Context) {
		flightLoading(c, "正在读取启动器设置…")
	}, 280, 56, ui.TesterOptions{})
	tester.Frame()
	start := bytes.Clone(tester.Image().Pix)
	tester.Advance(840 * time.Millisecond)
	if bytes.Equal(start, tester.Image().Pix) {
		t.Fatal("the plane did not move while loading")
	}
	tester.SetPreferences(ui.Preferences{ReduceMotion: true})
	still := bytes.Clone(tester.Image().Pix)
	tester.Advance(time.Second)
	if !bytes.Equal(still, tester.Image().Pix) {
		t.Fatal("the plane kept moving with reduced motion enabled")
	}
}
