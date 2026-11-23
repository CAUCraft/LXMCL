package config

import "testing"

func TestSkippedModUpdatesAreLocalToInstanceAndVersion(t *testing.T) {
	store := Store{Root: t.TempDir()}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"A", "B"} {
		if err := state.Create(name, "1.21.1"); err != nil {
			t.Fatal(err)
		}
	}
	a, b := state.Instances[0], state.Instances[1]
	const old = "map/old"
	const newer = "map/new"
	for _, version := range []string{old, newer} {
		if err := state.SetModUpdateSkipped(a.ID, a.Revision, version, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Load()
	if err != nil || !saved.SkippedModUpdates[a.ID][old] || !saved.SkippedModUpdates[a.ID][newer] || saved.SkippedModUpdates[b.ID][old] || saved.Selected != b.ID || saved.Instances[0] != a {
		t.Fatal(saved, err)
	}
	draft := saved.Clone()
	if err := draft.SetModUpdateSkipped(a.ID, a.Revision+1, old, false); err == nil {
		t.Fatal("stale instance accepted")
	}
	if err := draft.SetModUpdateSkipped(a.ID, a.Revision, old, false); err != nil {
		t.Fatal(err)
	}
	if draft.SkippedModUpdates[a.ID][old] || !draft.SkippedModUpdates[a.ID][newer] || !saved.SkippedModUpdates[a.ID][old] {
		t.Fatal("restore changed another version or snapshot")
	}
	draft.Remove(a.ID)
	if len(draft.SkippedModUpdates) != 0 || len(saved.SkippedModUpdates[a.ID]) != 2 {
		t.Fatal("remove changed another snapshot")
	}
}
