package launcher

import (
	"context"

	"github.com/CAUCraft/LXMCL/internal/server"
)

type Server = server.Server

type ServerStatus = server.Status

func ServerAddress(address string) (string, error) { return server.Address(address) }

func PingServer(ctx context.Context, address string) (ServerStatus, error) {
	return server.Ping(ctx, address)
}
