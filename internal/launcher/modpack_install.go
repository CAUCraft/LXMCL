package launcher

import (
	"context"
	"net/http"

	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/modpack"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

// InstallModpack installs and validates all files before publishing the instance.
// The caller serializes state writes and keeps the submitted state snapshot.
func (g Game) InstallModpack(ctx context.Context, store Store, next State, i Instance, pack PreparedModpack, key string, report func(Progress)) error {
	install := func(ctx context.Context, work Instance, report func(Progress)) error {
		return g.InstallWithJava(ctx, work, next.Settings.JavaPath, report)
	}
	return tasks.Run(ctx, func(ctx context.Context, progress func(Progress)) error {
		return g.installModpack(ctx, store, next, i, pack, newCurseForge(key), nil, install, progress)
	}, report)
}

func (g Game) installModpack(ctx context.Context, store Store, next State, i Instance, pack PreparedModpack, cf curseForge,
	client *http.Client, install func(context.Context, Instance, func(Progress)) error, report func(Progress)) error {
	saved, exists := next.Find(i.ID)
	if !exists || saved != i {
		return i18n.Error("modpack.instance_changed")
	}
	return modpack.Install(ctx, pack, modpack.InstallOptions{
		Target: i, Cache: distribution.Cache{Root: g.Cache, Client: client}, Resolver: cf,
		InstallGame: install, Commit: func() error { return store.Save(next) },
	}, report)
}
