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
