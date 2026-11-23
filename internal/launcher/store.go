package launcher

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/account"
	"github.com/CAUCraft/LXMCL/internal/config"
	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// Store combines configuration and credential storage for application callers.
// Configuration rules belong to config; encrypted credentials belong to account.
type Store struct {
	Root string
	// Accounts supplies authorization protocols. Its zero value uses system
	// networking; configure it before sharing Store with background operations.
	Accounts account.Client
}

func DefaultStore() (Store, error) {
	s, err := config.DefaultStore()
	return Store{Root: s.Root}, err
}

func (s Store) Load() (State, error)   { return (config.Store{Root: s.Root}).Load() }
func (s Store) Save(state State) error { return (config.Store{Root: s.Root}).Save(state) }

func (s Store) SaveWithNewGameRoot(state State) error {
	return (config.Store{Root: s.Root}).SaveWithNewGameRoot(state)
}

// RegisterInstance reserves a fresh game directory before recording a new profile.
// Archive imports and file copies publish their own directory before registering it.
func (s Store) RegisterInstance(state State, i Instance) error {
	if err := os.MkdirAll(filepath.Dir(i.GameDir), 0755); err != nil {
		return i18n.Error("install.instance_dir_create_failed", err)
	}
	if err := os.Mkdir(i.GameDir, 0755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return i18n.Error("instance.directory_occupied")
		}
		return i18n.Error("install.instance_dir_create_failed", err)
	}
	if err := s.saveNewInstance(state, i); err != nil {
		return errors.Join(err, os.Remove(i.GameDir))
	}
	return nil
}

// Check filesystem identity after publication. An older profile's missing directory
// may become reachable there through another spelling or a directory link.
func (s Store) saveNewInstance(state State, created Instance) error {
	dir, err := os.Stat(created.GameDir)
	if err != nil {
		return err
	}
	for _, other := range state.Instances {
		if other.ID == created.ID {
			continue
		}
		if info, err := os.Stat(other.GameDir); err == nil && os.SameFile(dir, info) {
			return i18n.Error("instance.directory_occupied")
		}
	}
	return s.Save(state)
}
