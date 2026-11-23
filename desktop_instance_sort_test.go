package main

import (
	"slices"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/playtime"
)

func TestLibrarySortPreservesTargetsAndCombinesFilters(t *testing.T) {
	a, _ := testDesktop(t)
	a.state.Instances[0].Name = "Beta"
	a.state.Instances[1].Name = "alpha"
	a.state.Instances[0].Loader = "" // Older vanilla instances have no loader field.
	a.state.Instances[1].Loader = "fAbRiC"
	if err := a.state.Create("Gamma", "1.21.1"); err != nil {
		t.Fatal(err)
	}
	a.state.Instances[2].Loader = "Custom Fabric"
	a.state.Selected = 1
	original := slices.Clone(a.state.Instances)
	now := time.Now()
	a.playtimes = map[int]playtime.Entry{1: {LastPlayed: now, SessionMillis: 8}, 2: {LastPlayed: now.Add(time.Hour), SessionMillis: 3}}
	check := func(mode string, ids ...int) {
		t.Helper()
		a.state.LibrarySort = mode
		var got []int
		for _, i := range a.libraryInstances() {
			got = append(got, i.ID)
		}
		if !slices.Equal(got, ids) {
			t.Fatal(mode, got, ids)
		}
	}
	check("", 1, 2, 3)
	check(sortName, 2, 1, 3)
	check(sortRecent, 2, 1, 3)
	check(sortDuration, 1, 2, 3)
	a.state.PinnedInstances = map[int]bool{2: true, 3: true}
	check("", 2, 3, 1)
	check(sortName, 2, 3, 1)
	check(sortRecent, 2, 3, 1)
	check(sortDuration, 2, 3, 1)
	delete(a.state.PinnedInstances, 2)
	check(sortName, 3, 2, 1)
	check(sortDuration, 3, 1, 2)
	group := "g"
	a.state.InstanceGroups = map[int]string{1: group, 2: group}
	a.libraryGroup = &group
	a.query = "a"
	a.instanceFilter = 2
	check(sortRecent, 2)
	a.libraryLoader = "Fabric"
	check(sortRecent, 2)
	a.libraryLoader = "Vanilla"
	check(sortRecent)
	a.instanceFilter = 0
	check(sortRecent, 1)
	a.libraryLoader, a.libraryGroup = unknownLibraryLoader, nil
	check(sortName, 3)
	a.libraryLoader = ""
	check(sortName, 3, 2, 1)
	if a.state.Selected != 1 || !slices.Equal(a.state.Instances, original) {
		t.Fatal("presentation reordered stored instances or changed the home target")
	}
}
