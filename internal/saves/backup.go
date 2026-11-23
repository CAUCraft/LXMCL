package saves

import (
	"context"
	"crypto/rand"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

// Backup plans all world files before writing a ZIP in BackupsDir.
// The returned path is published only after the archive is closed and synced.
func Backup(ctx context.Context, gameDir, dataRoot string, id int) (string, error) {
	return backup(ctx, gameDir, dataRoot, id, nil)
}

// BackupWorld archives only the selected world, retaining its folder name for restore.
func BackupWorld(ctx context.Context, gameDir, dataRoot string, id int, world World) (string, error) {
	return backup(ctx, gameDir, dataRoot, id, &world)
}

func backup(ctx context.Context, gameDir, dataRoot string, id int, world *World) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	root, err := savesRoot(gameDir, false)
	if errors.Is(err, fs.ErrNotExist) {
		if world != nil {
			return "", i18n.Error("saves.world_changed", world.folder)
		}
		return "", i18n.Error("saves.no_worlds_to_back_up")
	}
	if err != nil {
		return "", err
	}
	defer root.Close()
	files, err := planBackup(ctx, root, world)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	dir := BackupsDir(dataRoot, id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	destination, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer destination.Close()
	name := time.Now().Format("20060102-150405") + "-" + rand.Text()[:6] + ".zip"
	if err := storage.PublishNew(ctx, destination, name, 0600, func(file *os.File) error {
		if err := writeBackup(ctx, root, files, file); err != nil {
			return err
		}
		if world != nil {
			return world.check(root)
		}
		return nil
	}); err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}
