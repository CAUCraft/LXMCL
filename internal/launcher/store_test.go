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
