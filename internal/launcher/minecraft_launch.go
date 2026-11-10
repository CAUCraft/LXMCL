package launcher

import (
	"context"

	"github.com/CAUCraft/LXMCL/internal/account"
	"github.com/CAUCraft/LXMCL/internal/minecraft"
)

type Running = minecraft.Running

var ErrRepairRequired = minecraft.ErrRepairRequired
var ErrJavaRequired = minecraft.ErrJavaRequired

func (g Game) Start(ctx context.Context, i Instance, settings Settings) (Running, error) {
	return g.StartOnServer(ctx, i, settings, "")
}

func (g Game) StartOnServer(ctx context.Context, i Instance, settings Settings, server string) (Running, error) {
	return g.StartWithSession(ctx, i, settings, server, nil)
}

func (g Game) StartWithSession(ctx context.Context, i Instance, settings Settings, server string, session *MinecraftSession) (Running, error) {
	identity, err := launchIdentity(settings.Player, session)
	if err != nil {
		return Running{}, err
	}
	return g.start(ctx, i, settings, server, identity, nil)
}

func (g Game) StartWithStation(ctx context.Context, i Instance, settings Settings, server string, session StationSession) (Running, error) {
	identity, err := stationIdentity(session)
	if err != nil {
		return Running{}, err
	}
	return g.start(ctx, i, settings, server, identity, &session)
}

func stationIdentity(session StationSession) (gameIdentity, error) { return session.LaunchIdentity() }

func (g Game) start(ctx context.Context, i Instance, settings Settings, server string, identity gameIdentity, station *StationSession) (Running, error) {
	return g.startProgress(ctx, i, settings, server, identity, station, nil)
}

type gameIdentity = account.LaunchIdentity

func launchIdentity(offline string, session *MinecraftSession) (gameIdentity, error) {
	if session != nil {
		return session.LaunchIdentity()
	}
	return account.OfflineIdentity(offline)
}

func (g Game) startProgress(ctx context.Context, i Instance, settings Settings, server string, identity gameIdentity, station *StationSession, report func(Progress)) (Running, error) {
	options := minecraft.LaunchOptions{Settings: settings, Server: server, Identity: identity}
	if station != nil {
		options.PrepareJVM = func(ctx context.Context) ([]string, error) { return station.LaunchArgs(ctx, g.Cache) }
	}
	return g.runtime().Start(ctx, i, options, report)
}
