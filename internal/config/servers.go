package config

import (
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/server"
)

func (s *State) AddServer(name, address string) error {
	address, err := server.Address(address)
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = address
	}
	if len([]rune(name)) > 80 {
		return i18n.Error("server.favorite_name_too_long")
	}
	if slices.ContainsFunc(s.Servers, func(server server.Server) bool { return server.Address == address }) {
		return i18n.Error("server.favorite_duplicate")
	}
	s.Servers = append(s.Servers, server.Server{Name: name, Address: address})
	return nil
}

func (s *State) RemoveServer(address string) {
	s.Servers = slices.DeleteFunc(s.Servers, func(server server.Server) bool { return server.Address == address })
}
