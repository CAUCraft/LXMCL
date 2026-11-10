package launcher

import (
	"context"
	"errors"

	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/modpack"
)

type ModpackInfo = modpack.Info
type PreparedModpack = modpack.Prepared

func (g Game) PrepareOnlineModpack(ctx context.Context, r ModpackRelease) (PreparedModpack, error) {
	if r.Unavailable != "" {
		return PreparedModpack{}, errors.New(r.Unavailable)
	}
	if !r.asset.Valid(modpack.MaxArchiveSize) || r.asset.Size == 0 {
		return PreparedModpack{}, i18n.Error("modpack.online_file_unavailable")
	}
	filename, err := (distribution.Cache{Root: g.Cache}).Ensure(ctx, r.asset)
	if err != nil {
		return PreparedModpack{}, err
	}
	return modpack.Prepare(ctx, filename, r.source)
}

func PrepareLocalModpack(ctx context.Context, filename string) (PreparedModpack, error) {
	return modpack.PrepareLocal(ctx, filename)
}
