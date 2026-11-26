package content

import (
	"archive/zip"
	"context"
	"errors"
	"image"
	"io"
	"io/fs"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/iconimage"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/packmeta"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

type ResourcePackPreview struct {
	packmeta.Metadata
	Cover                 image.Image
	MetadataErr, CoverErr error
}

// PreviewResourcePack opens only the selected local pack. The two root entries
// are independent so a missing cover or unreadable manifest can be shown separately.
func PreviewResourcePack(ctx context.Context, i instance.Instance, file File) (ResourcePackPreview, error) {
	if err := ctx.Err(); err != nil {
		return ResourcePackPreview{}, err
	}
	name := file.DiskName()
	if !storage.ValidFilename(name) || !file.Directory && !ValidName(file.Name, ".zip") {
		return ResourcePackPreview{}, i18n.Error("resource.filename_invalid")
	}
	root, err := OpenDirectory(i, ResourcePacks, false)
	if err != nil {
		return ResourcePackPreview{}, err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil {
		return ResourcePackPreview{}, err
	}
	if file.Directory && info.IsDir() {
		dir, err := root.OpenRoot(name)
		if err != nil {
			return ResourcePackPreview{}, err
		}
		defer dir.Close()
		return readPackPreview(ctx, dir.FS())
	}
	if file.Directory || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > MaxSize {
		return ResourcePackPreview{}, i18n.Error("resource.filename_or_size_invalid")
	}
	f, err := root.Open(name)
	if err != nil {
		return ResourcePackPreview{}, err
	}
	defer f.Close()
	z, err := zip.NewReader(f, info.Size())
	if err != nil {
		return ResourcePackPreview{}, i18n.Error("resource.archive_invalid", err)
	}
	if _, err = storage.IndexPackEntries(z.File, MaxSize); err != nil {
		return ResourcePackPreview{}, err
	}
	return readPackPreview(ctx, z)
}

func packEntry(ctx context.Context, pack fs.FS, name string, limit int64) ([]byte, error) {
	info, err := fs.Stat(pack, name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, i18n.Error("resource.filename_or_size_invalid")
	}
	f, err := pack.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(transfer.Reader(ctx, f), limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, i18n.Error("resource.filename_or_size_invalid")
	}
	return data, nil
}

func readPackPreview(ctx context.Context, pack fs.FS) (ResourcePackPreview, error) {
	var p ResourcePackPreview
	meta, err := packEntry(ctx, pack, packmeta.Filename, packmeta.MaxBytes)
	if err == nil {
		p.Metadata, err = packmeta.Decode(meta)
	}
	p.MetadataErr = err
	png, err := packEntry(ctx, pack, "pack.png", iconimage.MaxPreviewBytes)
	if err == nil {
		p.Cover, err = iconimage.Preview(ctx, png, "png")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		p.CoverErr = err
	}
	return p, ctx.Err()
}
