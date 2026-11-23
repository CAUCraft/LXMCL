package config

import (
	"slices"
	"testing"
)

func TestInstanceGroupsSurviveReloadWithoutChangingLaunchTargets(t *testing.T) {
	store := Store{Root: t.TempDir()}
	s, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Survival", "Building"} {
		if err := s.Create(name, "1.21.1"); err != nil {
			t.Fatal(err)
		}
	}
	instances := slices.Clone(s.Instances)
	first, second := instances[0], instances[1]
	if err := s.SetInstanceGroup(first.ID, first.Revision, "", "  Friends  "); err != nil {
		t.Fatal(err)
	}
	if err := s.SetInstanceGroup(second.ID, second.Revision, "", "friends"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.GroupNames(), []string{"Friends"}) {
		t.Fatal(s.InstanceGroups)
	}
	group := "Friends"
	s.LibraryGroup = &group
	if err := store.Save(s); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Load()
	if err != nil || saved.LibraryGroup == nil || *saved.LibraryGroup != group || saved.InstanceGroups[second.ID] != group || saved.Selected != second.ID || !slices.Equal(saved.Instances, instances) {
		t.Fatal(saved, err)
	}
	draft := saved.Clone()
	if err := draft.SetInstanceGroup(first.ID, first.Revision+1, group, "New"); err == nil {
		t.Fatal("accepted a stale revision")
	}
	if err := draft.SetInstanceGroup(first.ID, first.Revision, "", "New"); err == nil {
		t.Fatal("accepted a stale group editor")
	}
	if err := draft.SetInstanceGroup(first.ID, first.Revision, group, "A\nB"); err == nil {
		t.Fatal("accepted multiline group")
	}
	if err := draft.SetInstanceGroup(first.ID, first.Revision, group, ""); err != nil {
		t.Fatal(err)
	}
	if draft.LibraryGroup == nil {
		t.Fatal("filter lost with another member present")
	}
	draft.Remove(second.ID)
	if draft.LibraryGroup != nil || len(draft.InstanceGroups) != 0 || len(saved.InstanceGroups) != 2 {
		t.Fatal("last-member cleanup or clone isolation failed")
	}
}
