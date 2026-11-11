package launcher

import (
	"github.com/CAUCraft/LXMCL/internal/account"
	"github.com/CAUCraft/LXMCL/internal/config"
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
