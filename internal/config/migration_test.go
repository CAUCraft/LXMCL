package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/config"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func TestLegacySettingsRemainIntactAndCurrentConfigTakesPrecedence(t *testing.T) {
	store := config.Store{Root: t.TempDir()}
	settings := instance.Settings{GameRoot: filepath.Join(store.Root, "existing-games"), Player: "LegacyPlayer", Memory: 6144, JavaPath: filepath.Join(store.Root, "java")}
	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(store.Root, "launcher-settings.json")
	if err := os.WriteFile(legacy, data, 0600); err != nil {
		t.Fatal(err)
	}
	state, err := store.Load()
	if err != nil || state.Settings != settings {
		t.Fatalf("legacy settings changed: %+v %v", state, err)
	}
	if err := state.UseOfflinePlayer("NewPlayer"); err != nil {
		t.Fatal(err)
	}
	if err := state.AddServer("Favorite", "example.com:25565"); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil || loaded.Settings.Player != "NewPlayer" || len(loaded.Servers) != 1 || loaded.Servers[0] != state.Servers[0] {
		t.Fatalf("current selection was lost: %+v %v", loaded, err)
	}
	if got, err := os.ReadFile(legacy); err != nil || string(got) != string(data) {
		t.Fatalf("legacy source changed: %v", err)
	}
	current := filepath.Join(store.Root, "launcher.json")
	corrupt := []byte(`{"instances":[]} trailing data`)
	if err := os.WriteFile(current, corrupt, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("corrupt current file fell back to legacy configuration")
	}
	if got, err := os.ReadFile(current); err != nil || string(got) != string(corrupt) {
		t.Fatalf("failed load replaced current data: %v", err)
	}
}

func TestEditServerKeepsOrderAndRejectsStaleOrDuplicateEdits(t *testing.T) {
	store := config.Store{Root: t.TempDir()}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"one.example", "two.example", "three.example"} {
		if err := state.AddServer("", address); err != nil {
			t.Fatal(err)
		}
	}
	original := state.Servers[1]
	if err := state.EditServer(original, "  社团服  ", " [2001:db8::1]:25566 "); err != nil {
		t.Fatal(err)
	}
	want := state.Clone()
	if want.Servers[1].Address != "[2001:db8::1]:25566" || want.Servers[1].Name != "社团服" {
		t.Fatalf("edit changed order or failed to normalize: %+v", want.Servers)
	}
	edited := want.Servers[1]
	if state.EditServer(original, "stale", "new.example") == nil ||
		state.EditServer(edited, "duplicate", "ONE.example:25565") == nil ||
		state.EditServer(edited, "", "invalid.example:99999") == nil {
		t.Fatal("accepted a stale, duplicate or invalid edit")
	}
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil || !slices.Equal(loaded.Servers, want.Servers) {
		t.Fatalf("favorites changed after rejected edits and reload: %+v %v", loaded.Servers, err)
	}
}
