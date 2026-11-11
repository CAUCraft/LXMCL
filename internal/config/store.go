package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

// Store reads and atomically replaces launcher configuration. Callers serialize
// writes; credentials and task history are owned by their respective modules.
type Store struct{ Root string }

func DefaultStore() (Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return Store{}, err
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		dir = local
	}
	return Store{Root: filepath.Join(dir, "org.caucraft.lxmcl")}, nil
}

func (s Store) Load() (State, error) {
	state := State{
		Settings: instance.Settings{GameRoot: filepath.Join(s.Root, "games"), Player: "Player", Memory: 4096},
		NextID:   1,
	}
	found, err := storage.ReadJSON(filepath.Join(s.Root, "launcher.json"), &state)
	if found || err != nil {
		if err == nil {
			state.NextID = state.nextInstanceID()
		}
		return state, err
	}
	// Import the last Rust release on first use. The original files stay in place.
	if _, err = storage.ReadJSON(filepath.Join(s.Root, "launcher-settings.json"), &state.Settings); err != nil {
		return state, err
	}
	var old struct{ Instances []instance.Instance }
	if _, err = storage.ReadJSON(filepath.Join(s.Root, "instances.json"), &old); err != nil {
		return state, err
	}
	state.Instances = old.Instances
	state.NextID = state.nextInstanceID()
	if len(state.Instances) > 0 {
		state.Selected = state.Instances[0].ID
	}
	return state, nil
}

func (s Store) Save(state State) error {
	state.NextID = state.nextInstanceID()
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return storage.WriteFile(filepath.Join(s.Root, "launcher.json"), b)
}
