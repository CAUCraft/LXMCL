package screenshots

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// RevealPath checks that the original preview still names the same PNG.
// It checks file identity and metadata without reading the image contents.
func (o *Original) RevealPath(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if o.info == nil || !validPNGName(o.name) || !filepath.IsAbs(o.gameDir) {
		return "", fs.ErrInvalid
	}
	gameDir := filepath.Clean(o.gameDir)
	game, err := os.OpenRoot(gameDir)
	if err != nil {
		return "", err
	}
	defer game.Close()
	gameIdentity, err := game.Stat(".")
	if err != nil {
		return "", err
	}
	folder, err := checkedExportChild(game, Directory)
	if err != nil {
		return "", err
	}
	defer folder.Close()
	folderIdentity, err := folder.Stat(".")
	if err != nil {
		return "", err
	}
	current, err := folder.Lstat(o.name)
	if err != nil {
		return "", err
	}
	if isExportReparsePoint(current) || !sameScreenshot(o.info, current) {
		return "", i18n.Error("screenshot.changed")
	}
	liveFolder, err := game.Lstat(Directory)
	if err != nil {
		return "", err
	}
	liveGame, err := os.Stat(gameDir)
	if err != nil {
		return "", err
	}
	if liveFolder.Mode().Type() != fs.ModeDir || isExportReparsePoint(liveFolder) ||
		!os.SameFile(folderIdentity, liveFolder) || !os.SameFile(gameIdentity, liveGame) {
		return "", i18n.Error("screenshot.changed")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return filepath.Join(gameDir, Directory, o.name), nil
}
