package content

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

type contentFileLocation struct {
	directory, file fs.FileInfo
}

// ModRevealPath verifies that this inventory row still identifies the same
// regular mod file at its original path. It never reads the JAR contents.
func (f File) ModRevealPath(i instance.Instance) (string, error) {
	ext, _, err := Mods.Format(i)
	if err != nil {
		return "", err
	}
	if !ValidName(f.Name, ext) {
		return "", fs.ErrInvalid
	}
	return f.revealPath(i, Mods, 0)
}

// ResourcePackRevealPath checks the exact inventory entry, including an
// optional .disabled suffix. A preview need not decode for this check.
func (f File) ResourcePackRevealPath(i instance.Instance) (string, error) {
	if !storage.ValidFilename(f.DiskName()) || !f.Directory && !ValidName(f.Name, ".zip") {
		return "", fs.ErrInvalid
	}
	var entryType fs.FileMode
	if f.Directory {
		entryType = fs.ModeDir
	}
	return f.revealPath(i, ResourcePacks, entryType)
}

func (f File) revealPath(i instance.Instance, kind Kind, entryType fs.FileMode) (string, error) {
	if f.location == nil || !filepath.IsAbs(i.GameDir) {
		return "", fs.ErrInvalid
	}
	gameDir := filepath.Clean(i.GameDir)
	game, err := os.OpenRoot(gameDir)
	if err != nil {
		return "", err
	}
	defer game.Close()
	gameIdentity, err := game.Stat(".")
	if err != nil {
		return "", err
	}

	folder := string(kind)
	directory, err := game.Lstat(folder)
	if err != nil {
		return "", err
	}
	if directory.Mode().Type() != fs.ModeDir || !os.SameFile(f.location.directory, directory) {
		return "", fs.ErrInvalid
	}
	name := filepath.Join(folder, f.DiskName())
	file, err := game.Lstat(name)
	if err != nil {
		return "", err
	}
	if file.Mode().Type() != entryType || !os.SameFile(f.location.file, file) {
		return "", fs.ErrInvalid
	}
	current, err := game.Lstat(folder)
	if err != nil {
		return "", err
	}
	if current.Mode().Type() != fs.ModeDir || !os.SameFile(directory, current) {
		return "", fs.ErrInvalid
	}
	currentGame, err := os.Stat(gameDir)
	if err != nil {
		return "", err
	}
	if !os.SameFile(gameIdentity, currentGame) {
		return "", fs.ErrInvalid
	}
	return filepath.Join(gameDir, name), nil
}
