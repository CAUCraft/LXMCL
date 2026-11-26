package config

import (
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/server"
)

func (s *State) AddServer(name, address string) error {
	return s.saveServer(-1, name, address)
}

func (s *State) EditServer(original server.Server, name, address string) error {
	index := slices.Index(s.Servers, original)
	if index < 0 {
		return i18n.Error("server.favorite_changed")
	}
	return s.saveServer(index, name, address)
}

func (s *State) saveServer(index int, name, address string) error {
	entry, err := server.NewFavorite(name, address)
	if err != nil {
		return err
	}
	if s.serverAddressesExcept(index)[entry.Address] {
		return i18n.Error("server.favorite_duplicate")
	}
	if index < 0 {
		s.Servers = append(s.Servers, entry)
	} else {
		s.Servers[index] = entry
	}
	return nil
}

// SavedServerAddresses includes equivalent spellings from older configurations.
func (s *State) SavedServerAddresses() map[string]bool {
	return s.serverAddressesExcept(-1)
}

func (s *State) serverAddressesExcept(index int) map[string]bool {
	saved := make(map[string]bool, len(s.Servers))
	for i, entry := range s.Servers {
		if i == index {
			continue
		}
		if address, err := server.Address(entry.Address); err == nil {
			saved[address] = true
		}
	}
	return saved
}

type ServerImportEntry struct {
	Server server.Server
	Add    bool
}

// PlanServerImport keeps the input order and marks the first eligible occurrence.
func (s *State) PlanServerImport(rows []server.Server) []ServerImportEntry {
	saved := s.SavedServerAddresses()
	plan := make([]ServerImportEntry, 0, len(rows))
	for _, row := range rows {
		entry, err := server.NewFavorite(row.Name, row.Address)
		add := err == nil && !saved[entry.Address]
		if add {
			saved[entry.Address] = true
		}
		plan = append(plan, ServerImportEntry{Server: entry, Add: add})
	}
	return plan
}

// ImportServers appends eligible favorites in selection order, preserving existing entries.
func (s *State) ImportServers(rows []server.Server) int {
	before := len(s.Servers)
	for _, row := range s.PlanServerImport(rows) {
		if row.Add {
			s.Servers = append(s.Servers, row.Server)
		}
	}
	return len(s.Servers) - before
}

// MatchingServers returns favorites in their saved order.
func (s *State) MatchingServers(query string) []server.Server {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return s.Servers
	}
	var visible []server.Server
	for _, entry := range s.Servers {
		if entry.MatchesNameOrAddress(query) {
			visible = append(visible, entry)
		}
	}
	return visible
}

// MoveServer swaps adjacent matches while leaving hidden favorites in place.
func (s *State) MoveServer(original server.Server, step int, query string) bool {
	if step != -1 && step != 1 {
		return false
	}
	visible := s.MatchingServers(query)
	from := slices.Index(visible, original)
	to := from + step
	if from < 0 || to < 0 || to >= len(visible) {
		return false
	}
	i, j := slices.Index(s.Servers, original), slices.Index(s.Servers, visible[to])
	s.Servers[i], s.Servers[j] = s.Servers[j], s.Servers[i]
	return true
}

func (s *State) RemoveServer(address string) {
	s.Servers = slices.DeleteFunc(s.Servers, func(entry server.Server) bool { return entry.Address == address })
}
