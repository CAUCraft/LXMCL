package saves

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

// WorldFileStats describes the logical size of a world's backup-eligible files.
type WorldFileStats struct {
	FileCount int
	Bytes     uint64
}

// BackupStats checks the selected world and counts its backup-eligible files
// without opening their contents.
func (w World) BackupStats(ctx context.Context, gameDir string) (WorldFileStats, error) {
	if err := ctx.Err(); err != nil {
		return WorldFileStats{}, err
	}
	if !filepath.IsAbs(gameDir) {
		return WorldFileStats{}, i18n.Error("instance.game_dir_required")
	}
	game, err := os.OpenRoot(gameDir)
	if err != nil {
		return WorldFileStats{}, err
	}
	defer game.Close()
	saves, err := openBackupDirectory(game, "saves")
	if err != nil {
		return WorldFileStats{}, err
	}
	defer saves.Close()
	savesInfo, err := saves.Stat(".")
	if err != nil {
		return WorldFileStats{}, err
	}
	world, err := openBackupDirectory(saves, w.folder)
	if err != nil {
		return WorldFileStats{}, err
	}
	defer world.Close()
	dir, err := world.Stat(".")
	if err != nil {
		return WorldFileStats{}, err
	}
	level, err := world.Lstat("level.dat")
	if err != nil {
		return WorldFileStats{}, err
	}
	if w.directory == nil || w.level == nil || !os.SameFile(w.directory, dir) || !sameBackupFile(w.level, level) {
		return WorldFileStats{}, i18n.Error("saves.world_changed", w.folder)
	}

	observed := backupFilePlan{paths: make(storage.ArchivePaths)}
	if err := observed.readScopedWorld(ctx, world, w.folder); err != nil {
		return WorldFileStats{}, err
	}
	if !slices.ContainsFunc(observed.files, func(file backupFile) bool {
		return file.name == w.folder+"/level.dat" && sameBackupFile(w.level, file.info)
	}) {
		return WorldFileStats{}, i18n.Error("saves.world_changed", w.folder)
	}
	// Re-read the file set before publishing statistics so a changed or
	// incomplete enumeration cannot look like an authoritative count.
	current := backupFilePlan{paths: make(storage.ArchivePaths)}
	if err := current.readScopedWorld(ctx, world, w.folder); err != nil {
		return WorldFileStats{}, err
	}
	if !slices.EqualFunc(observed.files, current.files, func(before, after backupFile) bool {
		return before.name == after.name && sameBackupFile(before.info, after.info)
	}) || !sameCopyPath(game, "saves", savesInfo) || !sameCopyPath(saves, w.folder, dir) {
		return WorldFileStats{}, i18n.Error("saves.world_changed", w.folder)
	}
	if err := w.check(saves); err != nil {
		return WorldFileStats{}, err
	}
	if err := ctx.Err(); err != nil {
		return WorldFileStats{}, err
	}
	return WorldFileStats{FileCount: len(observed.files), Bytes: observed.size}, nil
}

func (p *backupFilePlan) readScopedWorld(ctx context.Context, root *os.Root, prefix string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		if !entry.IsDir() && name == "session.lock" {
			continue
		}
		archiveName := path.Join(prefix, name)
		info, err := root.Lstat(name)
		if err != nil {
			return err
		}
		if isBackupReparsePoint(info) || info.Mode().Type() != entry.Type() {
			return i18n.Error("saves.backup_file_unsupported", archiveName)
		}
		if info.IsDir() {
			if !storage.ValidPortablePath(archiveName) {
				return i18n.Error("saves.path_invalid", archiveName)
			}
			child, err := openBackupDirectory(root, name)
			if err != nil {
				return err
			}
			err = p.readScopedWorld(ctx, child, archiveName)
			child.Close()
			if err != nil {
				return err
			}
			if !sameCopyPath(root, name, info) {
				return i18n.Error("saves.backup_file_changed", archiveName)
			}
			continue
		}
		if err := p.add(archiveName, info); err != nil {
			return err
		}
	}
	return nil
}
