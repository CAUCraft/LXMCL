package config

import "testing"

func TestInstanceNotesAreLocalToTheProfile(t *testing.T) {
	store := Store{Root: t.TempDir()}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Survival", "Building"} {
		if err := state.Create(name, "1.21.1"); err != nil {
			t.Fatal(err)
		}
	}
	source := state.Instances[0]
	const note = "村庄 134, 72\n下次修桥"
	if err := state.SetInstanceNote(source.ID, source.Revision, "", note); err != nil {
		t.Fatal(err)
	}
	draft := state.Clone()
	if e := draft.SetInstanceNote(source.ID, source.Revision, "", "overwrite"); e == nil {
		t.Fatal("outdated note replaced saved text")
	}
	if e := draft.SetInstanceNote(source.ID, source.Revision+1, note, "overwrite"); e == nil {
		t.Fatal("outdated target accepted")
	}
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Load()
	if err != nil || saved.InstanceNotes[source.ID] != note || saved.Instances[0] != source || saved.Selected != 2 || len(saved.InstanceNotes) != 1 {
		t.Fatal(saved, err)
	}
	draft.Remove(source.ID)
	if len(draft.InstanceNotes) != 0 || state.InstanceNotes[source.ID] != note {
		t.Fatal("removal affected another snapshot")
	}
	if err := saved.SetInstanceNote(source.ID, source.Revision, note, ""); err != nil || len(saved.InstanceNotes) != 0 {
		t.Fatal(saved, err)
	}
}
