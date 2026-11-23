package config

import (
	"slices"

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

// ImportServers appends eligible favorites in selection order, preserving existing entries.
func (s *State) ImportServers(rows []server.Server) int {
	saved := s.SavedServerAddresses()
	before := len(s.Servers)
	for _, row := range rows {
		entry, err := server.NewFavorite(row.Name, row.Address)
		if err != nil || saved[entry.Address] {
			continue
		}
		s.Servers = append(s.Servers, entry)
		saved[entry.Address] = true
	}
	return len(s.Servers) - before
}

// MoveServer moves a captured favorite one position in the saved list.
func (s *State) MoveServer(original server.Server, step int) bool {
	if step != -1 && step != 1 {
		return false
	}
	from := slices.Index(s.Servers, original)
	to := from + step
	if from < 0 || to < 0 || to >= len(s.Servers) {
		return false
	}
	s.Servers[from], s.Servers[to] = s.Servers[to], s.Servers[from]
	return true
}

func (s *State) RemoveServer(address string) {
	s.Servers = slices.DeleteFunc(s.Servers, func(entry server.Server) bool { return entry.Address == address })
}
