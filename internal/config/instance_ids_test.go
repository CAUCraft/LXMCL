package config_test

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/config"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func TestLoadRetainsInstancesAndAdvancesMissingOrStaleCounter(t *testing.T) {
	for _, tc := range []struct {
		name, counter string
		want          int
	}{
		{"missing", "", 20}, {"stale", `,"next_id":1`, 20},
		{"negative", `,"next_id":-4`, 20}, {"high water mark", `,"next_id":100`, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := config.Store{Root: t.TempDir()}
			old := instance.Instance{ID: 19, Name: "existing", Version: "1.21.1", GameDir: filepath.Join(store.Root, "original"), Revision: 7}
			encoded, err := json.Marshal([]instance.Instance{old})
			if err != nil {
				t.Fatal(err)
			}
			data := []byte(fmt.Sprintf(`{"instances":%s,"selected":19%s}`, encoded, tc.counter))
			file := filepath.Join(store.Root, "launcher.json")
			if err := os.WriteFile(file, data, 0600); err != nil {
				t.Fatal(err)
			}
			state, err := store.Load()
			if err != nil || state.NextID != tc.want || state.Selected != 19 || len(state.Instances) != 1 || state.Instances[0] != old {
				t.Fatalf("migration changed identity: %+v %v", state, err)
			}
			if got, err := os.ReadFile(file); err != nil || string(got) != string(data) {
				t.Fatalf("Load rewrote source: %v", err)
			}
			if err := state.Create("new", "1.21.1"); err != nil {
				t.Fatal(err)
			}
			created := state.Instances[1]
			if created.ID != tc.want || created.GameDir == old.GameDir || created.GameDir != filepath.Join(state.Settings.GameRoot, fmt.Sprintf("instance-%d", tc.want)) {
				t.Fatalf("reused identity: %+v", created)
			}
			state.Remove(created.ID)
			if err := store.Save(state); err != nil {
				t.Fatal(err)
			}
			saved, err := store.Load()
			if err != nil || saved.NextID != tc.want+1 || saved.Instances[0] != old || saved.Selected != old.ID {
				t.Fatalf("removal reused an ID: %+v %v", saved, err)
			}
		})
	}
}

func TestCreateAndSaveRepairCounterWithoutRenumbering(t *testing.T) {
	store := config.Store{Root: t.TempDir()}
	original := config.State{Settings: instance.Settings{GameRoot: filepath.Join(store.Root, "games")},
		Instances: []instance.Instance{{ID: 4, Name: "original", GameDir: filepath.Join(store.Root, "old")}}, Selected: 4, NextID: 1}
	if err := store.Save(original); err != nil {
		t.Fatal(err)
	}
	if original.NextID != 1 {
		t.Fatal("Save mutated caller snapshot")
	}
	saved, err := store.Load()
	if err != nil || saved.NextID != 5 || saved.Instances[0] != original.Instances[0] {
		t.Fatalf("saved counter: %+v %v", saved, err)
	}
	draft := original.Clone()
	if err := draft.Create("new", "1.21.1"); err != nil {
		t.Fatal(err)
	}
	if draft.Selected != 5 || draft.NextID != 6 || original.Selected != 4 {
		t.Fatalf("direct allocation reused an ID: %+v", draft)
	}
}

func TestExhaustedCounterDoesNotOverflowOrMutateState(t *testing.T) {
	for _, high := range []bool{false, true} {
		state := config.State{Settings: instance.Settings{GameRoot: t.TempDir()}, Selected: 3, NextID: 1,
			Instances: []instance.Instance{{ID: math.MaxInt, Name: "last", GameDir: "retained", Revision: 9}}}
		if high {
			state.NextID = math.MaxInt
			state.Instances[0].ID = 3
		}
		before := state.Clone()
		if err := state.Create("new", "1.21.1"); err == nil || !reflect.DeepEqual(before, state) {
			t.Fatalf("exhaustion modified state: %+v %v", state, err)
		}
		store := config.Store{Root: t.TempDir()}
		if err := store.Save(state); err != nil {
			t.Fatal(err)
		}
		saved, err := store.Load()
		if err != nil || saved.NextID != math.MaxInt || !reflect.DeepEqual(saved.Instances, before.Instances) {
			t.Fatalf("overflow on reload: %+v %v", saved, err)
		}
	}
}
