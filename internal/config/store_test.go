package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/instance"
)

func TestMigrationAndInstanceIdentity(t *testing.T) {
	root := t.TempDir()
	s := Store{Root: root}
	oldPath := filepath.Join(root, "instances.json")
	old := []byte(`{"instances":[{"id":19,"name":"旧实例","version":"1.21.1","loader":"Vanilla","game_directory":"original-path","revision":7}]}`)
	if err := os.WriteFile(oldPath, old, 0600); err != nil {
		t.Fatal(err)
	}
	state, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.Selected != 19 {
		t.Fatalf("selected = %d", state.Selected)
	}
	if err := state.Rename(19, 6, "过期草稿"); err == nil {
		t.Fatal("stale revision was accepted")
	}
	if err := state.Rename(19, 7, "保留目录"); err != nil {
		t.Fatal(err)
	}
	if err := state.Create("新实例", "1.21.1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(state); err != nil {
		t.Fatal(err)
	}
	reloaded, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	i, ok := reloaded.Find(19)
	if !ok || i.Name != "保留目录" || i.GameDir != "original-path" || i.Revision != 8 || reloaded.Selected != 20 {
		t.Fatalf("unexpected migrated state: %+v", reloaded)
	}
	reloaded.Remove(20)
	if err := s.Save(reloaded); err != nil {
		t.Fatal(err)
	}
	reloaded, err = s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Selected != 19 || len(reloaded.Instances) != 1 {
		t.Fatalf("unexpected state after removal: %+v", reloaded)
	}
	if b, err := os.ReadFile(oldPath); err != nil || string(b) != string(old) {
		t.Fatalf("source changed: %s / %v", b, err)
	}
}

func TestChangeVersionKeepsDirectoryAndSelection(t *testing.T) {
	store := Store{Root: t.TempDir()}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"主实例", "测试实例"} {
		if err := state.Create(name, "1.21.1"); err != nil {
			t.Fatal(err)
		}
	}
	target := state.Instances[0]
	if err := state.ChangeVersion(target.ID, target.Revision, "1.21.4"); err != nil {
		t.Fatal(err)
	}
	if err := state.ChangeVersion(target.ID, target.Revision, "1.21.5"); err == nil {
		t.Fatal("a stale version editor could replace the saved version")
	}
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	got, ok := saved.Find(target.ID)
	if !ok || got.Version != "1.21.4" || got.GameDir != target.GameDir || got.Revision != target.Revision+1 || saved.Selected != state.Instances[1].ID {
		t.Fatalf("version change did not preserve the instance: %+v", saved)
	}
}

func TestInstanceLaunchOverridesAndDuplicatePersist(t *testing.T) {
	store := Store{Root: t.TempDir()}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	state.Settings.JavaPath = filepath.Join(store.Root, "global-java")
	for _, name := range []string{"测试世界", "默认世界"} {
		if err := state.Create(name, "1.21.1"); err != nil {
			t.Fatal(err)
		}
	}
	source, selected := state.Instances[0], state.Selected
	opts := instance.LaunchOptions{JavaMode: instance.JavaAuto, Memory: 8192, Display: instance.DisplayResolution, Width: 1280, Height: 720}
	if err := state.ConfigureLaunch(source.ID, source.Revision, opts); err != nil {
		t.Fatal(err)
	}
	if err := state.ConfigureLaunch(source.ID, source.Revision, instance.LaunchOptions{}); err == nil {
		t.Fatal("stale draft replaced the saved overrides")
	}
	duplicate, err := state.Duplicate(source.ID, source.Revision+1, "独立副本")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	original, _ := saved.Find(source.ID)
	copied, _ := saved.Find(duplicate.ID)
	unchanged, _ := saved.Find(selected)
	if original.Launch != opts || copied.Launch != opts || copied.GameDir == original.GameDir ||
		copied.Version != original.Version || saved.Selected != selected {
		t.Fatalf("copy changed the selected instance or shared its data directory: %+v", saved)
	}
	if original.Launch.JavaExecutable(saved.Settings.JavaPath) != "" ||
		unchanged.Launch.JavaExecutable(saved.Settings.JavaPath) != saved.Settings.JavaPath ||
		original.Launch.MaxMemory(saved.Settings.Memory) != 8192 ||
		unchanged.Launch.MaxMemory(saved.Settings.Memory) != saved.Settings.Memory {
		t.Fatal("instance overrides changed the global Java or memory defaults")
	}
}
