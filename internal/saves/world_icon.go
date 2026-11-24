package saves

import (
	"context"
	"image"
	"io"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/iconimage"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// Icon decodes icon.png from the captured world for an optional preview.
func (w World) Icon(ctx context.Context, gameDir string) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(gameDir) {
		return nil, i18n.Error("instance.game_dir_required")
	}
	game, err := os.OpenRoot(gameDir)
	if err != nil {
		return nil, err
	}
	defer game.Close()
	saves, err := openBackupDirectory(game, "saves")
	if err != nil {
		return nil, err
	}
	defer saves.Close()
	savesInfo, err := saves.Stat(".")
	if err != nil {
		return nil, err
	}
	if err := w.check(saves); err != nil {
		return nil, err
	}
	world, err := openBackupDirectory(saves, w.folder)
	if err != nil {
		return nil, err
	}
	defer world.Close()
	directory, err := world.Stat(".")
	if err != nil {
		return nil, err
	}
	if !os.SameFile(w.directory, directory) {
		return nil, i18n.Error("saves.world_changed", w.folder)
	}
	const filename = "icon.png"
	expected, err := world.Lstat(filename)
	if err != nil {
		return nil, err
	}
	if !regularBackup(expected) || expected.Size() > iconimage.MaxPreviewBytes {
		return nil, i18n.Error("icon.read_failed")
	}
	file, err := world.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !sameBackupFile(expected, opened) {
		return nil, i18n.Error("icon.read_failed")
	}
	data, err := io.ReadAll(io.LimitReader(transfer.Reader(ctx, file), iconimage.MaxPreviewBytes+1))
	if err != nil {
		return nil, err
	}
	icon, err := iconimage.Preview(ctx, data, "png")
	if err != nil {
		return nil, err
	}
	current, err := world.Lstat(filename)
	if err != nil {
		return nil, err
	}
	if !sameBackupFile(expected, current) {
		return nil, i18n.Error("icon.read_failed")
	}
	if !sameCopyPath(game, "saves", savesInfo) {
		return nil, i18n.Error("saves.world_changed", w.folder)
	}
	if err := w.check(saves); err != nil {
		return nil, err
	}
	return icon, nil
}
