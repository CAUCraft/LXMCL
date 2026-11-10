package main

import (
	"bytes"
	"image"
	"image/color"
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
		}, capes: map[string]*ui.Bitmap{cape: renderCape(atlas)}}
	tt.Frame()
	if err := tt.Click("账号设置"); err != nil {
		t.Fatal(err)
	}
	capture(t, tt, "account-appearance")
	tt.SetSize(820, 530)
	tt.SetDark(true)
	tt.Advance(500 * time.Millisecond)
	tt.Frame()
	list, leftOK := tt.Find("添加离线账号")
	skin, rightOK := tt.Find("选择 PNG…")
	if !leftOK || !rightOK || list.X+list.W >= skin.X || skin.X+skin.W > 820 || skin.Y+skin.H > 530 {
		t.Fatalf("account list and preview do not fit the compact window: %+v %+v", list, skin)
	}
	a.appearance.draft, a.appearance.file = []byte("draft"), "新皮肤.png"
	a.appearance.variant, a.appearance.classic = "classic", clubSkin
	tt.Frame()
	capture(t, tt, "account-appearance-compact-dark")
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
	if err := tt.Click("Player"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("孪枭 · 皮肤预览"); !ok || a.state.MicrosoftProfile != id {
		t.Fatal("browsing an offline skin did not preserve the active Microsoft account")
	}
	if err := tt.Click("账号设置"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("新皮肤.png"); !ok {
		t.Fatal("browsing another account discarded the local skin draft")
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
	front := skinSide(img, false, false)
	if front.RGBAAt(4, 0) != face || front.RGBAAt(5, 0) != hat {
		t.Fatal("the face and avatar disagree")
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
