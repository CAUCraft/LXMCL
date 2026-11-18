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

type PackUpdate = modpack.UpdatePreview

func (g Game) PreviewPackUpdate(ctx context.Context, i Instance, pack PreparedModpack, key string) (PackUpdate, error) {
	return modpack.PreviewUpdate(ctx, pack, modpack.InstallOptions{Target: i, Cache: g.downloads(), Resolver: (catalog.Client{Transport: g.Transport}).CurseForgeResolver(key)})
}
func (g Game) ApplyPackUpdate(ctx context.Context, store Store, state State, i Instance, pack PreparedModpack, key string, preview PackUpdate, report func(Progress)) (State, error) {
	next := state.Clone()
	for n, old := range next.Instances {
		if old != i {
			continue
		}
		if !preview.Matches(pack, i) {
			return state, i18n.Error("modpack.instance_changed")
		}
		info := pack.Info()
		revised := old
		revised.PackFeed, revised.PackDigest = pack.Feed()
		revised.Version, revised.Loader, revised.LoaderVersion = info.Minecraft, info.Loader, info.LoaderVersion
		revised.Revision++
		if err := revised.Validate(); err != nil {
			return state, err
		}
		next.Instances[n] = revised
		err := modpack.ApplyUpdate(ctx, preview, modpack.InstallOptions{Target: i, Cache: g.downloads(), Resolver: (catalog.Client{Transport: g.Transport}).CurseForgeResolver(key),
			InstallGame: func(ctx context.Context, work Instance, report func(Progress)) error {
				return g.InstallWithJava(ctx, work, next.Settings.JavaPath, report)
			},
			Commit: func() error { return store.Save(next) },
		}, report)
		if err != nil {
			return state, err
		}
		return next, nil
	}
	return state, i18n.Error("modpack.instance_changed")
}
