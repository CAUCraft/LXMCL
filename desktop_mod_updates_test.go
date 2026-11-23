package main

import (
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/launcher"
)

func TestSkippedCandidateStaysOutOfBatchUpdates(t *testing.T) {
	a, tt := testDesktop(t)
	i := &a.state.Instances[1]
	i.Loader, i.LoaderVersion = "Fabric", "0.16.14"
	file := launcher.ContentFile{Name: "map.jar", Enabled: true}
	m := launcher.InstalledMod{File: file}
	m.Title = "Atlas"
	m.Current = launcher.PackVersion{ID: "first", ProjectID: "map", Number: "1.0"}
	m.Update = launcher.PackVersion{ID: "second", ProjectID: "map", Number: "2.0"}
	m.Unconfirmed = []launcher.ModDependency{{ProjectID: "render"}}
	a.content = contentView{id: i.ID, revision: i.Revision, tab: 1, files: []launcher.ContentFile{file}, mods: map[string]launcher.InstalledMod{file.Name: m}, selected: map[string]bool{file.Name: true}}
	if err := a.state.SetModUpdateSkipped(i.ID, i.Revision, m.UpdateKey(), true); err != nil {
		t.Fatal(err)
	}
	a.inspect(*i)
	a.detailPage = detailContent
	tt.SetSize(820, 530)
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		tt.Advance(300 * time.Millisecond)
		tt.Frame()
		r, ok := tt.Find("恢复提示")
		if !ok || r.X < 0 || r.X+r.W > 820 || r.Y < 0 || r.Y+r.H > 530 || !tt.HasText("已识别 1/1 · 可更新 0") || tt.HasText("更新选中 (1)") || tt.HasText("核对前置 (1)") {
			t.Fatal(tt.Texts())
		}
	}
	other := m
	other.File.Name = "other.jar"
	other.Title = "Lantern"
	other.Current.ProjectID, other.Update.ProjectID = "lamp", "lamp"
	a.content.files = append(a.content.files, other.File)
	a.content.mods[other.File.Name] = other
	a.content.selected[other.File.Name] = true
	a.content.listing = nil
	tt.Frame()
	if !tt.HasText("已识别 2/2 · 可更新 1") || !tt.HasText("更新选中 (1)") {
		t.Fatal("a different project was skipped", tt.Texts())
	}
	m.Update.ID = "third" // The upstream display number is reused.
	a.content.mods[file.Name] = m
	tt.Frame()
	if !tt.HasText("跳过此版本") || !tt.HasText("Atlas · 1.0 → 2.0") || !tt.HasText("更新选中 (2)") || !tt.HasText("核对前置 (1)") {
		t.Fatal(tt.Texts())
	}
}
