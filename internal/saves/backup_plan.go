package saves

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

type backupFile struct {
	name string
	info fs.FileInfo
}

// VisitWorldFiles visits the same recognized worlds and stable file snapshots as a backup.
func VisitWorldFiles(ctx context.Context, gameDir string, visit func(*os.Root, string, fs.FileInfo) error) error {
	root, err := savesRoot(gameDir, false)
	if err != nil {
		return err
	}
	defer root.Close()
	files, err := planBackup(ctx, root, nil)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := visit(root, f.name, f.info); err != nil {
			return err
		}
	}
	return nil
}

// planBackup validates the entire file list before creating any output.
func planBackup(ctx context.Context, root *os.Root, selected *World) ([]backupFile, error) {
	var entries []fs.DirEntry
	if selected != nil {
		if err := selected.check(root); err != nil {
			return nil, err
		}
		entries = []fs.DirEntry{fs.FileInfoToDirEntry(selected.directory)}
	} else {
		var err error
		entries, err = fs.ReadDir(root.FS(), ".")
		if err != nil {
			return nil, err
		}
	}
	var files []backupFile
	var size uint64
	paths := make(storage.ArchivePaths)
	for _, world := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !world.IsDir() {
			continue
		}
		levelPath := filepath.Join(world.Name(), "level.dat")
		level, err := root.Lstat(levelPath)
		if errors.Is(err, fs.ErrNotExist) {
			_, oldErr := root.Lstat(levelPath + "_old")
			if errors.Is(oldErr, fs.ErrNotExist) {
				continue
			}
			if oldErr != nil {
				return nil, oldErr
			}
			return nil, i18n.Error("saves.backup_world_needs_repair", world.Name())
		}
		if err != nil {
			return nil, err
		}
		if !level.Mode().IsRegular() {
			return nil, i18n.Error("saves.backup_file_unsupported", levelPath)
		}
		levelFound := false
		if err := fs.WalkDir(root.FS(), world.Name(), func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() || entry.Name() == "session.lock" {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return i18n.Error("saves.backup_file_unsupported", name)
			}
			if len(files) >= maxSaveEntries {
				return i18n.Error("saves.backup_too_many_files")
			}
			if uint64(info.Size()) > maxSaveBytes-size {
				return i18n.Error("saves.backup_too_large")
			}
			if err := paths.Add(name, false); err != nil {
				return err
			}
			size += uint64(info.Size())
			files = append(files, backupFile{name, info})
			levelFound = levelFound || name == world.Name()+"/level.dat"
			return nil
		}); err != nil {
			return nil, err
		}
		if !levelFound {
			return nil, i18n.Error("saves.backup_file_changed", world.Name()+"/level.dat")
		}
	}
	if len(files) == 0 {
		return nil, i18n.Error("saves.no_worlds_to_back_up")
	}
	return files, nil
}
