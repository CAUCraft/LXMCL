package launcher

import (
	"context"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/server"
)

type Server = server.Server

type ServerStatus = server.Status

func ServerAddress(address string) (string, error) { return server.Address(address) }

func PingServer(ctx context.Context, address string) (ServerStatus, error) {
	return server.Ping(ctx, address)
}

func (s *State) AddServer(name, address string) error {
	address, err := ServerAddress(address)
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
	if slices.ContainsFunc(s.Servers, func(server Server) bool { return server.Address == address }) {
		return i18n.Error("server.favorite_duplicate")
	}
	s.Servers = append(s.Servers, Server{Name: name, Address: address})
	return nil
}

func (s *State) RemoveServer(address string) {
	s.Servers = slices.DeleteFunc(s.Servers, func(server Server) bool { return server.Address == address })
}
