package main

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
)

func TestCopyInstanceVersionFromOverview(t *testing.T) {
	a, tt := testDesktop(t)
	first, second := &a.state.Instances[0], &a.state.Instances[1]
	first.Name, second.Name = "同名实例", "同名实例"
	second.Loader, second.LoaderVersion = launcher.LoaderFabric, "0.16.14"
	legacy := *second
	legacy.ID, legacy.Name, legacy.Loader, legacy.LoaderVersion = 3, "旧加载器实例", launcher.LoaderQuilt, ""
	a.state.Instances = append(a.state.Instances, legacy)
	saved := slices.Clone(a.state.Instances)
	tt.SetSize(820, 530)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatalf("%s: %v (%v)", label, err, tt.Texts())
		}
		tt.Frame()
	}
	const copyLabel = "复制实例版本信息"
	checkCopy := func(game, loader string) {
		t.Helper()
		click(copyLabel)
		want := "LXMCL：" + launcherVersion + "\nMinecraft：" + game + "\n加载器：" + loader
		if _, ok := tt.Find("已复制实例版本信息"); tt.Clipboard() != want || !ok {
			t.Fatalf("copy = %q, success shown = %v, want %q", tt.Clipboard(), ok, want)
		}
	}
	click("实例")
	click("1.21.4 · Fabric 0.16.14")
	tt.SetPreferences(ui.Preferences{})
	for _, theme := range []string{"light", "dark"} {
		tt.SetDark(theme == "dark")
		tt.Advance(500 * time.Millisecond)
		checkCopy("1.21.4", "Fabric 0.16.14")
		tt.Advance(500 * time.Millisecond)
		capture(t, tt, "instance-version-copy-"+theme)
	}
	if a.inspected != second.ID || a.state.Selected != first.ID {
		t.Fatal("copying the second same-name instance changed the home launch target")
	}

	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	click("1.21.1 · 原版 · 启动目标")
	checkCopy("1.21.1", "原版")
	click("旧加载器实例")
	checkCopy("1.21.4", "Quilt（版本未知）")
	before := tt.Clipboard()
	tt.FailClipboard(errors.New("clipboard busy"))
	click(copyLabel)
	if _, ok := tt.Find("复制实例版本信息失败，请重试：clipboard busy"); tt.Clipboard() != before || !ok {
		t.Fatal("failed copy modified the clipboard or did not report the error")
	}
	if !slices.Equal(a.state.Instances, saved) || a.state.Selected != first.ID || a.busy {
		t.Fatal("version copying changed instance state or started a task")
	}
}

func TestInstanceVersionClipboardValidatesLegacyFields(t *testing.T) {
	const prefix = "LXMCL："
	for _, tc := range []struct {
		game, loader, version string
		wantGame, wantLoader  string
	}{
		{"1.21.4", "Quilt", "0.29.2-beta.10", "1.21.4", "Quilt 0.29.2-beta.10"},
		{"1.21.4", "Forge", "1.21.4-54.0.1", "1.21.4", "Forge 54.0.1"},
		{"1.21.1", "Fabric", "0.16.14-opaque", "1.21.1", "Fabric（版本未知）"},
		{"/private/game", "unverified-owner", "0.16.14", "未知", "未知"},
	} {
		i := launcher.Instance{Version: tc.game, Loader: tc.loader, LoaderVersion: tc.version, GameDir: "/private/game"}
		want := prefix + launcherVersion + "\nMinecraft：" + tc.wantGame + "\n加载器：" + tc.wantLoader
		if got := instanceVersionClipboard(i); got != want {
			t.Fatalf("shared version text = %q, want %q", got, want)
		}
	}
}
