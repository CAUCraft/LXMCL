package launcher

import (
	"os"
	"path/filepath"
	"testing"
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
