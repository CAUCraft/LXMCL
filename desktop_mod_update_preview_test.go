package main

import (
	"slices"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo/ui"
)

func TestModUpdatePreview(t *testing.T) {
	a, tt := testDesktop(t)
	for n := range a.state.Instances {
		i := &a.state.Instances[n]
		i.Name, i.Loader, i.LoaderVersion = "同名地图实例", "Fabric", "0.16.14"
	}
	target := a.state.Instances[1]
	target.Revision = 7
	a.state.Instances[1] = target
	const bridge = "map-bridge.jar"
	const engine = "map-engine.jar"
	const hidden = "local-hidden.jar"
	const extra = "map-extra.jar"
	const skipped = "map-skipped.jar"
	var files []launcher.ContentFile
	mods := make(map[string]launcher.InstalledMod)
	for _, name := range []string{bridge, engine, extra, skipped, hidden} {
		file := launcher.ContentFile{Name: name, Enabled: name != engine}
		mod := launcher.InstalledMod{File: file, Digest: name}
		mod.Current = launcher.PackVersion{ID: "installed", ProjectID: name, Number: "1.0"}
		mod.Update = launcher.PackVersion{ID: "candidate", ProjectID: name, Number: "2.0"}
		if name == bridge {
			dep := launcher.ModDependency{ProjectID: "render", Type: "required"}
			mod.Update.Dependencies, mod.Unconfirmed = []launcher.ModDependency{dep}, []launcher.ModDependency{dep}
		}
		if name == engine {
			mod.Current.Number = ""
		}
		files = append(files, file)
		mods[file.DiskName()] = mod
	}
	if err := a.state.SetModUpdateSkipped(target.ID, target.Revision, mods[skipped].UpdateKey(), true); err != nil {
		t.Fatal(err)
	}
	a.inspect(target)
	a.detailPage = detailContent
	a.content = contentView{id: target.ID, revision: target.Revision, tab: 1, files: files, mods: mods}
	tt.SetSize(820, 530)
	tt.Frame()
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(label, err, tt.Texts())
		}
		tt.Frame()
	}
	click("搜索本地资源")
	tt.Type("MAP")
	tt.Frame()
	click("选择资源 " + bridge)
	click("选择资源 " + engine)
	const open = "更新选中 (2)"
	const confirm = "确认更新 (2)"
	const backup = "更新前备份所选原模组到 ZIP"
	click(open)
	if p := a.content.modPreview; p == nil || p.target != target || p.backup ||
		!slices.EqualFunc(p.selected, []string{bridge, engine + ".disabled"}, func(mod launcher.InstalledMod, name string) bool { return mod.DiskName() == name }) {
		t.Fatal("preview scope", a.content.modPreview, tt.Texts())
	}
	for _, text := range []string{"目标实例 · #2 · 同名地图实例", "1.0 → 2.0", "未知 → 2.0", "必需前置 · 1 项未确认", "必需前置 · 未知", engine + ".disabled"} {
		if !tt.HasText(text) {
			t.Fatal("missing preview detail", text, tt.Texts())
		}
	}
	for _, name := range []string{hidden, extra, skipped} {
		if tt.HasText(name) {
			t.Fatal("preview included an unselected mod", name)
		}
	}
	click(backup)
	if !a.content.modPreview.backup || a.busy || a.pickerBusy {
		t.Fatal("selecting the optional backup started work", tt.Texts())
	}
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		for _, text := range []string{"取消", confirm, backup, "目标实例 · #2 · 同名地图实例"} {
			r, ok := tt.Find(text)
			if !ok || r.X < 0 || r.Y < 42 || r.X+r.W > 820.5 || r.Y+r.H > 530.5 {
				t.Fatal("clipped preview control", text, r, tt.Texts())
			}
		}
		name := "mod-update-preview-light"
		if dark {
			name = "mod-update-preview-dark"
		}
		capture(t, tt, name)
	}
	click("取消")
	if a.content.modPreview != nil || a.content.query != "MAP" || !tt.HasText(open) {
		t.Fatal("cancel did not preserve filtered selection", tt.Texts())
	}
	click(open)
	if a.content.modPreview.backup {
		t.Fatal("a new preview inherited the previous backup choice")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.content.modPreview != nil || !tt.HasText(open) {
		t.Fatal("escape did not return to the selected mods", tt.Texts())
	}
	click(open)
	newMod := mods[bridge]
	newMod.Update.ID = "different-candidate" // The display number is still 2.0.
	mods[bridge] = newMod
	tt.Frame()
	if a.content.modPreview != nil || tt.HasText(confirm) || !tt.HasText("更新预览已失效") || !tt.HasText(open) {
		t.Fatal("replaced candidate retained its old confirmation", tt.Texts())
	}
	click(open)
	if err := a.state.SetModUpdateSkipped(target.ID, target.Revision, newMod.UpdateKey(), true); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.content.modPreview != nil || tt.HasText(confirm) || !tt.HasText("更新选中 (1)") {
		t.Fatal("new skip did not invalidate the batch", tt.Texts())
	}
	click("更新选中 (1)")
	a.state.Instances[1].Revision++
	tt.Frame()
	if a.content.modPreview != nil || tt.HasText("确认更新") || a.busy || len(a.history) != 0 || a.state.Selected != 1 {
		t.Fatal("a stale preview started work or changed the launch target", tt.Texts())
	}
}
