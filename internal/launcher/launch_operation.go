package launcher

import (
	"context"
	"errors"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

type launchSteps struct {
	installed func(Instance) bool
	install   func(context.Context, Instance, string, func(Progress)) error
	authorize func(context.Context, LaunchRequest, func(Progress)) (LaunchResult, error)
	start     func(context.Context, LaunchRequest, LaunchResult, func(Progress)) (Running, error)
}

// Execute handles authentication, installation, one repair attempt and startup.
// The caller serializes state-changing operations and observes Running.Done.
func (g Game) Execute(ctx context.Context, store Store, request LaunchRequest, report func(Progress)) (LaunchResult, error) {
	return executeLaunch(ctx, request, report, launchSteps{
		installed: g.Installed, install: g.InstallWithJava,
		authorize: func(ctx context.Context, r LaunchRequest, p func(Progress)) (LaunchResult, error) {
			return authorizeLaunch(ctx, store, r, p)
		},
		start: func(ctx context.Context, r LaunchRequest, auth LaunchResult, progress func(Progress)) (Running, error) {
			if r.State.StationKey != "" {
				identity, err := stationIdentity(auth.StationSession)
				if err != nil {
					return Running{}, err
				}
				return g.startProgress(ctx, r.Instance, r.State.Settings, r.Server, identity, &auth.StationSession, progress)
			}
			var session *MinecraftSession
			if r.State.MicrosoftProfile != "" {
				session = &auth.Session
			}
			identity, err := launchIdentity(r.State.Settings.Player, session)
			if err != nil {
				return Running{}, err
			}
			return g.startProgress(ctx, r.Instance, r.State.Settings, r.Server, identity, nil, progress)
		},
	})
}

func executeLaunch(ctx context.Context, r LaunchRequest, report func(Progress), steps launchSteps) (LaunchResult, error) {
	var result LaunchResult
	err := tasks.Run(ctx, func(ctx context.Context, progress func(Progress)) error {
		i := r.Instance
		if current, ok := r.State.Find(i.ID); !ok || current != i {
			return i18n.Error("instance.settings_target_changed")
		}
		var err error
		if !r.InstallOnly {
			result, err = steps.authorize(ctx, r, progress)
			result.AuthFailed = err != nil
			if err != nil {
				return err
			}
		}
		needsInstall := r.InstallOnly || !steps.installed(i)
		if needsInstall {
			if err = steps.install(ctx, i, r.State.Settings.JavaPath, progress); err != nil {
				return err
			}
		}
		if r.InstallOnly {
			return nil
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		progress(Progress{Stage: i18n.Text("progress.start_game")})
		result.Running, err = steps.start(ctx, r, result, progress)
		if !needsInstall && errors.Is(err, ErrRepairRequired) && ctx.Err() == nil {
			progress(Progress{Stage: i18n.Text("progress.repair_game")})
			if err = steps.install(ctx, i, r.State.Settings.JavaPath, progress); err != nil {
				return err
			}
			if err = ctx.Err(); err != nil {
				return err
			}
			progress(Progress{Stage: i18n.Text("progress.start_game")})
			result.Running, err = steps.start(ctx, r, result, progress)
		}
		return err
	}, report)
	return result, err
}
