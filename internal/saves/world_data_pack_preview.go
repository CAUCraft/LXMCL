package saves

import (
	"archive/zip"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/packmeta"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

var ErrDataPackMetadataMissing = errors.New("missing pack.mcmeta")

const maxDataPackArchive = 512 << 20

// Preview reads only the root metadata of a local pack captured by the list.
func (p WorldDataPacks) Preview(ctx context.Context, gameDir string, world World, pack DataPack) (packmeta.Metadata, error) {
	if err := ctx.Err(); err != nil {
		return packmeta.Metadata{}, err
	}
	snapshot, found := p.files[pack.Name]
	if !found || !storage.ValidFilename(pack.Name) || !filepath.IsAbs(gameDir) ||
		(pack.Kind != DataPackZIP && pack.Kind != DataPackFolder) {
		return packmeta.Metadata{}, i18n.Error("saves.data_pack_changed")
	}
	if snapshot.err != nil {
		return packmeta.Metadata{}, dataPackOpenError(snapshot.err)
	}
	before := snapshot.info
	game, err := os.OpenRoot(gameDir)
	if err != nil {
		return packmeta.Metadata{}, err
	}
	defer game.Close()
	root, err := openBackupDirectory(game, "saves")
	if err != nil {
		return packmeta.Metadata{}, err
	}
	defer root.Close()
	selected, err := openDataPackWorld(root, world)
	if err != nil {
		return packmeta.Metadata{}, err
	}
	defer selected.Close()
	files, err := openBackupDirectory(selected, "datapacks")
	if err != nil {
		return packmeta.Metadata{}, dataPackOpenError(err)
	}
	defer files.Close()
	dir, err := files.Stat(".")
	if err != nil {
		return packmeta.Metadata{}, err
	}
	if !os.SameFile(p.directory, dir) {
		return packmeta.Metadata{}, i18n.Error("saves.data_pack_changed")
	}
	if pack.Kind == DataPackFolder {
		folder, err := openBackupDirectory(files, pack.Name)
		if err != nil {
			return packmeta.Metadata{}, dataPackOpenError(err)
		}
		defer folder.Close()
		current, err := folder.Stat(".")
		if err != nil {
			return packmeta.Metadata{}, err
		}
		if !os.SameFile(before, current) {
			return packmeta.Metadata{}, i18n.Error("saves.data_pack_changed")
		}
		return readDataPackFolderMetadata(ctx, folder)
	}
	f, info, err := openDataPackFile(files, pack.Name, before, maxDataPackArchive)
	if err != nil {
		return packmeta.Metadata{}, dataPackOpenError(err)
	}
	defer f.Close()
	z, err := zip.NewReader(f, info.Size())
	if err != nil {
		return packmeta.Metadata{}, i18n.Error("resource.archive_invalid", err)
	}
	index, err := storage.IndexPackEntries(z.File, maxDataPackArchive)
	if err != nil {
		return packmeta.Metadata{}, err
	}
	entry := index[packmeta.Filename]
	if entry == nil {
		return packmeta.Metadata{}, ErrDataPackMetadataMissing
	}
	if entry.UncompressedSize64 > packmeta.MaxBytes {
		return packmeta.Metadata{}, i18n.Error("resource.filename_or_size_invalid")
	}
	reader, err := entry.Open()
	if err != nil {
		return packmeta.Metadata{}, err
	}
	defer reader.Close()
	return packmeta.Read(ctx, reader)
}

func dataPackOpenError(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return i18n.Error("saves.data_pack_changed")
	}
	return err
}

func readDataPackFolderMetadata(ctx context.Context, folder *os.Root) (packmeta.Metadata, error) {
	f, _, err := openDataPackFile(folder, packmeta.Filename, nil, packmeta.MaxBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return packmeta.Metadata{}, ErrDataPackMetadataMissing
	}
	if err != nil {
		return packmeta.Metadata{}, err
	}
	defer f.Close()
	return packmeta.Read(ctx, f)
}

func openDataPackFile(root *os.Root, name string, expected fs.FileInfo, limit int64) (*os.File, fs.FileInfo, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() || isBackupReparsePoint(info) || info.Size() > limit {
		return nil, nil, i18n.Error("resource.filename_or_size_invalid")
	}
	if expected != nil && !os.SameFile(expected, info) {
		return nil, nil, i18n.Error("saves.data_pack_changed")
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, nil, err
	}
	current, err := file.Stat()
	if err == nil && current.Mode().IsRegular() && !isBackupReparsePoint(current) &&
		os.SameFile(info, current) && current.Size() <= limit {
		return file, current, nil
	}
	file.Close()
	if err != nil {
		return nil, nil, err
	}
	return nil, nil, i18n.Error("saves.data_pack_changed")
}
