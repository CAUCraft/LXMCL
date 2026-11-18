package launcher

import (
	"context"
	"errors"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/modpack"
)

type ModpackInfo = modpack.Info
type PreparedModpack = modpack.Prepared

func (g Game) PrepareOnlineModpack(ctx context.Context, r ModpackRelease) (PreparedModpack, error) {
	if r.Unavailable != "" {
		return PreparedModpack{}, errors.New(r.Unavailable)
	}
	a, ok := r.DownloadAsset()
	if !ok {
		return PreparedModpack{}, i18n.Error("modpack.online_file_unavailable")
	}
	filename, err := g.downloads().Ensure(ctx, a)
	if err != nil {
		return PreparedModpack{}, err
	}
	return modpack.Prepare(ctx, filename, r.Source())
}

func PrepareLocalModpack(ctx context.Context, filename string) (PreparedModpack, error) {
	return modpack.PrepareLocal(ctx, filename)
}

type PortableFiles = modpack.Selection

func ExportInstance(ctx context.Context, i Instance, folder, version string, selection PortableFiles) (string, error) {
	return modpack.Export(ctx, i, folder, version, selection)
}
func DuplicateFiles(ctx context.Context, source, target Instance, selection PortableFiles, store Store, next State) error {
	return modpack.Duplicate(ctx, source, target, selection, func() error { return store.Save(next) })
}
