package config

import (
	"slices"
	"testing"
)

func TestRenameInstanceGroup(t *testing.T) {
	group := "Friends"
	var s State
	s.Settings.GameRoot = t.TempDir()
	for _, name := range []string{"First", "Second", "Third"} {
		if err := s.Create(name, "1.21.1"); err != nil {
			t.Fatal(err)
		}
	}
	s.InstanceGroups = map[int]string{1: group, 2: group, 3: "Solo"}
	s.LibraryGroup = &group
	next := s.Clone()
	for _, name := range []string{" ", "Solo", "solo", "New\nName"} {
		if err := next.RenameInstanceGroup(group, name); err == nil {
			t.Fatalf("accepted invalid group name %q", name)
		}
	}
	if err := next.RenameInstanceGroup(group, " friends "); err != nil {
		t.Fatal(err)
	}
	if *next.LibraryGroup != "friends" || next.InstanceGroups[1] != "friends" || next.InstanceGroups[2] != "friends" || next.InstanceGroups[3] != "Solo" || *s.LibraryGroup != group || s.InstanceGroups[1] != group {
		t.Fatal("unexpected group membership or filter", s.InstanceGroups, next.InstanceGroups)
	}
	if err := next.RenameInstanceGroup(group, "Stale"); err == nil {
		t.Fatal("accepted missing group")
	}
}

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
