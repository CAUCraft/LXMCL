package saves

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/nbt"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

type DataPackKind int

const (
	DataPackNonFile DataPackKind = iota
	DataPackZIP
	DataPackFolder
	DataPackUnsupported
	DataPackMissing
)

type DataPack struct {
	Name string // The local filename or the exact saved ID, including file/.
	Kind DataPackKind
}

type WorldDataPacks struct {
	Local    []DataPack
	Enabled  []DataPack
	Disabled []DataPack
	Saved    bool
	SavedErr error

	directory fs.FileInfo
	files     map[string]dataPackFile
}

type dataPackFile struct {
	info fs.FileInfo
	err  error
}

// ReadWorldDataPacks inventories immediate local files and the last saved
// selection. It never opens a pack or creates a directory.
func ReadWorldDataPacks(ctx context.Context, gameDir string, world World) (WorldDataPacks, error) {
	if err := ctx.Err(); err != nil {
		return WorldDataPacks{}, err
	}
	root, err := savesRoot(gameDir, false)
	if errors.Is(err, fs.ErrNotExist) {
		return WorldDataPacks{}, i18n.Error("saves.world_changed", world.folder)
	}
	if err != nil {
		return WorldDataPacks{}, err
	}
	defer root.Close()
	selected, err := openDataPackWorld(root, world)
	if err != nil {
		return WorldDataPacks{}, err
	}
	defer selected.Close()
	result, err := worldDataPackFiles(ctx, selected)
	if err != nil {
		return WorldDataPacks{}, err
	}
	files := make(map[string]DataPackKind, len(result.Local))
	for _, pack := range result.Local {
		files[pack.Name] = pack.Kind
	}
	var enabled, disabled []DataPack
	found := false
	result.SavedErr = readWorldData(ctx, root, world, func(r *nbt.Reader, tag byte, key string) {
		if key != "DataPacks" {
			r.Skip(tag)
			return
		}
		if tag != 10 || found {
			r.Invalidate()
			return
		}
		found = true
		r.Compound(func(tag byte, key string) {
			switch {
			case key == "Enabled" && tag == 9 && enabled == nil:
				enabled = savedDataPackList(ctx, r, files)
			case key == "Disabled" && tag == 9 && disabled == nil:
				disabled = savedDataPackList(ctx, r, files)
			case key == "Enabled" || key == "Disabled":
				r.Invalidate()
			default:
				r.Skip(tag)
			}
		})
	})
	if err := ctx.Err(); err != nil {
		return WorldDataPacks{}, err
	}
	if err := world.Check(gameDir); err != nil {
		return WorldDataPacks{}, err
	}
	if result.SavedErr == nil {
		result.Enabled, result.Disabled = enabled, disabled
		result.Saved = enabled != nil || disabled != nil
	}
	return result, nil
}

func openDataPackWorld(root *os.Root, world World) (*os.Root, error) {
	if err := world.check(root); err != nil {
		return nil, err
	}
	selected, err := openBackupDirectory(root, world.folder)
	if err != nil {
		return nil, err
	}
	info, err := selected.Stat(".")
	if err == nil && os.SameFile(world.directory, info) {
		return selected, nil
	}
	selected.Close()
	if err != nil {
		return nil, err
	}
	return nil, i18n.Error("saves.world_changed", world.folder)
}

func worldDataPackFiles(ctx context.Context, world *os.Root) (WorldDataPacks, error) {
	root, err := openBackupDirectory(world, "datapacks")
	if errors.Is(err, fs.ErrNotExist) {
		return WorldDataPacks{}, nil
	}
	if err != nil {
		return WorldDataPacks{}, err
	}
	defer root.Close()
	dir, err := root.Open(".")
	if err != nil {
		return WorldDataPacks{}, err
	}
	defer dir.Close()
	info, err := dir.Stat()
	if err != nil {
		return WorldDataPacks{}, err
	}
	packs := WorldDataPacks{directory: info, files: make(map[string]dataPackFile)}
	for {
		if err := ctx.Err(); err != nil {
			return WorldDataPacks{}, err
		}
		entries, err := dir.ReadDir(128)
		for _, entry := range entries {
			info, err := entry.Info()
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return WorldDataPacks{}, err
			}
			kind, err := dataPackFileKind(root, info)
			if err != nil {
				return WorldDataPacks{}, err
			}
			packs.Local = append(packs.Local, DataPack{Name: entry.Name(), Kind: kind})
			if kind == DataPackFolder || kind == DataPackZIP {
				// Root.Lstat captures the file identity from a handle on Windows.
				snapshot, err := root.Lstat(entry.Name())
				packs.files[entry.Name()] = dataPackFile{info: snapshot, err: err}
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return WorldDataPacks{}, err
		}
	}
	slices.SortFunc(packs.Local, func(a, b DataPack) int { return strings.Compare(a.Name, b.Name) })
	return packs, nil
}

func dataPackFileKind(root *os.Root, info fs.FileInfo) (DataPackKind, error) {
	if isBackupReparsePoint(info) {
		return DataPackUnsupported, nil
	}
	if info.Mode().IsRegular() && strings.EqualFold(filepath.Ext(info.Name()), ".zip") {
		return DataPackZIP, nil
	}
	if info.Mode().Type() != fs.ModeDir {
		return DataPackUnsupported, nil
	}
	folder, err := openBackupDirectory(root, info.Name())
	if errors.Is(err, fs.ErrNotExist) {
		return DataPackUnsupported, nil
	}
	if err != nil {
		return DataPackUnsupported, err
	}
	defer folder.Close()
	meta, err := folder.Lstat("pack.mcmeta")
	if errors.Is(err, fs.ErrNotExist) {
		return DataPackUnsupported, nil
	}
	if err != nil {
		return DataPackUnsupported, err
	}
	if meta.Mode().IsRegular() && !isBackupReparsePoint(meta) {
		return DataPackFolder, nil
	}
	return DataPackUnsupported, nil
}

func savedDataPackList(ctx context.Context, r *nbt.Reader, files map[string]DataPackKind) []DataPack {
	tag, count := r.Byte(), r.Count()
	if count > 65536 || tag != 8 && (tag != 0 || count != 0) {
		r.Invalidate()
		return nil
	}
	packs := make([]DataPack, 0, count)
	for range count {
		if r.Err() != nil || ctx.Err() != nil {
			return nil
		}
		id, valid := r.JavaString()
		if !valid || strings.TrimSpace(id) == "" || strings.ContainsFunc(id, unicode.IsControl) {
			r.Invalidate()
			return nil
		}
		kind := DataPackNonFile
		if name, local := strings.CutPrefix(id, "file/"); local {
			if current, exists := files[name]; exists {
				kind = current
			} else if storage.ValidFilename(name) {
				kind = DataPackMissing
			} else {
				kind = DataPackUnsupported
			}
		}
		packs = append(packs, DataPack{Name: id, Kind: kind})
	}
	return packs
}
