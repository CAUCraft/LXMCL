package config

import "testing"

func TestPinnedWorldPreferences(t *testing.T) {
	store := Store{Root: t.TempDir()}
	state, err := store.Load()
	if err != nil || len(state.PinnedWorlds) != 0 {
		t.Fatal(state.PinnedWorlds, err)
	}
	for _, name := range []string{"A", "B"} {
		if err := state.Create(name, "1.21.1"); err != nil {
			t.Fatal(err)
		}
	}
	a, b := state.Instances[0], state.Instances[1]
	const folder = "河谷小屋"
	const other = "河谷矿场"
	for _, name := range []string{folder, other} {
		if err := state.SetWorldPinned(a.ID, a.Revision, name, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Load()
	if err != nil || !saved.PinnedWorlds[a.ID][folder] || !saved.PinnedWorlds[a.ID][other] ||
		saved.PinnedWorlds[b.ID][folder] || saved.Instances[0] != a || saved.Instances[1] != b || saved.Selected != b.ID {
		t.Fatal(saved.PinnedWorlds, err)
	}
	draft := saved.Clone()
	if err := draft.SetWorldPinned(a.ID, a.Revision+1, folder, false); err == nil {
		t.Fatal("changed revision accepted")
	}
	if err := draft.SetWorldPinned(a.ID, a.Revision, "../"+folder, true); err == nil {
		t.Fatal("accepted a directory path")
	}
	if err := draft.SetWorldPinned(a.ID, a.Revision, folder, false); err != nil {
		t.Fatal(err)
	}
	copy, err := draft.Duplicate(a.ID, a.Revision, "Copy", "")
	if err != nil || draft.PinnedWorlds[copy.ID][other] || draft.PinnedWorlds[a.ID][folder] ||
		!draft.PinnedWorlds[a.ID][other] || !saved.PinnedWorlds[a.ID][folder] {
		t.Fatal("prefs escaped their instance or snapshot", draft.PinnedWorlds, err)
	}
	draft.Remove(a.ID)
	if len(draft.PinnedWorlds) != 0 || len(saved.PinnedWorlds[a.ID]) != 2 {
		t.Fatal("instance removal affected another snapshot")
	}
}
