package saves

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

type backupFile struct {
	name string
	info fs.FileInfo
}

// planBackup validates the entire file list before creating any output.
func planBackup(ctx context.Context, root *os.Root) ([]backupFile, error) {
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, err
	}
	var files []backupFile
	var size uint64
	paths := make(archivePaths)
	for _, world := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !world.IsDir() {
			continue
		}
		level, err := root.Lstat(filepath.Join(world.Name(), "level.dat"))
		if err != nil || !level.Mode().IsRegular() {
			continue
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
			if err := paths.add(name, false); err != nil {
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
