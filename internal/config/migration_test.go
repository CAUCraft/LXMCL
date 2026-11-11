package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
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
