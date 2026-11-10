package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
)

func TestVersionPickerKeepsLongCatalogInsidePopup(t *testing.T) {
	a, tt := testDesktop(t)
	for i := 31; i > 0; i-- {
		a.releases = append(a.releases, launcher.Version{ID: fmt.Sprintf("1.21.%02d", i)})
	}
	tt.SetSize(820, 530)
	tt.SetDark(true)
	for _, label := range []string{"实例", "建筑测试", "更换版本"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if err := tt.Click("选择游戏版本"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	search, ok := tt.Find("筛选选择游戏版本")
	if !ok || search.Y < 0 || search.Y+search.H > 530 {
		t.Fatalf("filter is outside the window: %+v", search)
	}
	first, ok := tt.Find("1.21.30")
	if !ok || first.Y < 0 || first.Y+first.H > 530 {
		t.Fatalf("first visible choices are not in the window: %+v", first)
	}
	// The last choice must be selectable even at the minimum window height,
	// without placing a long menu over the entire launcher.
	capture(t, tt, "version-picker-long")
	tt.Type("1.21.01")
	if err := tt.Click("1.21.01"); err != nil {
		t.Fatal(err)
	}
	if !a.form.open || a.form.version != "1.21.01" {
		t.Fatalf("the picker did not apply the selected version: %+v", a.form)
	}
}

func TestSkinStationSignInOpensFromAddAccount(t *testing.T) {
	a, tt := testDesktop(t)
	for _, target := range []string{"账号中心", "添加账号", "第三方皮肤站"} {
		if err := tt.Click(target); err != nil {
			t.Fatal(err)
		}
	}
	if a.stationLogin == nil || !a.stationLogin.open || a.page != pageAccount {
		t.Fatal("skin station sign-in did not open from the account menu")
	}
	capture(t, tt, "skin-station-login")
}
