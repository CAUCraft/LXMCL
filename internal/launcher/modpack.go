package launcher

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

type ModpackInfo struct {
	Source                   ModpackSource
	Name, Version, Minecraft string
	Loader, LoaderVersion    string
	Downloads, Overrides     int
}

type PreparedModpack struct {
	info     ModpackInfo
	filename string
	sum      [sha256.Size]byte
}

func (p PreparedModpack) Info() ModpackInfo { return p.info }

func (g Game) PrepareOnlineModpack(ctx context.Context, r ModpackRelease) (PreparedModpack, error) {
	if r.Unavailable != "" {
		return PreparedModpack{}, errors.New(r.Unavailable)
	}
	if !r.asset.valid(maxModpackArchive) || r.asset.Size == 0 {
		return PreparedModpack{}, i18n.Error("modpack.online_file_unavailable")
	}
	filename, err := g.modpackAsset(ctx, r.asset, modpackHTTP)
	if err != nil {
		return PreparedModpack{}, err
	}
	return prepareModpack(ctx, filename, r.source)
}

func PrepareLocalModpack(ctx context.Context, filename string) (PreparedModpack, error) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".mrpack":
		return prepareModpack(ctx, filename, ModrinthModpacks)
	case ".zip":
		return prepareModpack(ctx, filename, CurseForgeModpacks)
	default:
		return PreparedModpack{}, i18n.Error("modpack.archive_extension_invalid")
	}
}

func openModpack(filename string) (*os.File, int64, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, 0, err
	}
	info, err := f.Stat()
	if err == nil && (!info.Mode().IsRegular() || info.Size() == 0 || info.Size() > maxModpackArchive) {
		err = i18n.Error("modpack.archive_invalid")
	}
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, info.Size(), nil
}

func modpackFingerprint(ctx context.Context, f *os.File, size int64) ([sha256.Size]byte, error) {
	var sum [sha256.Size]byte
	h := sha256.New()
	n, err := io.Copy(h, transfer.Reader(ctx, io.NewSectionReader(f, 0, size+1)))
	if err == nil && n != size {
		err = i18n.Error("modpack.archive_size_changed")
	}
	copy(sum[:], h.Sum(nil))
	return sum, err
}

func prepareModpack(ctx context.Context, filename string, source ModpackSource) (PreparedModpack, error) {
	f, size, err := openModpack(filename)
	if err != nil {
		return PreparedModpack{}, err
	}
	defer f.Close()
	sum, err := modpackFingerprint(ctx, f, size)
	if err != nil {
		return PreparedModpack{}, err
	}
	plan, err := readModpack(f, size, source)
	if err != nil {
		return PreparedModpack{}, err
	}
	return PreparedModpack{info: plan.info, filename: filename, sum: sum}, ctx.Err()
}
