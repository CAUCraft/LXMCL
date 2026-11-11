package launcher

import (
	"context"

	"github.com/CAUCraft/LXMCL/internal/catalog"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/modpack"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

// InstallModpack installs and validates all files before publishing the instance.
// The caller serializes state writes and keeps the submitted state snapshot.
func (g Game) InstallModpack(ctx context.Context, store Store, next State, i Instance, pack PreparedModpack, key string, report func(Progress)) error {
	return tasks.Run(ctx, func(ctx context.Context, progress func(Progress)) error {
		saved, exists := next.Find(i.ID)
		if !exists || saved != i {
			return i18n.Error("modpack.instance_changed")
		}
		return modpack.Install(ctx, pack, modpack.InstallOptions{
			Target: i, Cache: g.downloads(), Resolver: (catalog.Client{Transport: g.Transport}).CurseForgeResolver(key),
			InstallGame: func(ctx context.Context, work Instance, report func(Progress)) error {
				return g.InstallWithJava(ctx, work, next.Settings.JavaPath, report)
			},
			Commit: func() error { return store.Save(next) },
		}, progress)
	}, report)
}
