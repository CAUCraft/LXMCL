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
	address, err := server.Address(address)
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = address
	} else if len([]rune(name)) > 80 {
		return i18n.Error("server.favorite_name_too_long")
	}
	for i, entry := range s.Servers {
		if i != index && entry.Address == address {
			return i18n.Error("server.favorite_duplicate")
		}
	}
	entry := server.Server{Name: name, Address: address}
	if index < 0 {
		s.Servers = append(s.Servers, entry)
	} else {
		s.Servers[index] = entry
	}
	return nil
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
