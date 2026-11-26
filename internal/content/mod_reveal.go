package content

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/instance"
)

type modFileLocation struct {
	directory, file fs.FileInfo
}

// ModRevealPath verifies that this inventory row still identifies the same
// regular mod file at its original path. It never reads the JAR contents.
func (f File) ModRevealPath(i instance.Instance) (string, error) {
	ext, _, err := Mods.Format(i)
	if err != nil {
		return "", err
	}
	if f.location == nil || !ValidName(f.Name, ext) || !filepath.IsAbs(i.GameDir) {
		return "", fs.ErrInvalid
	}
	gameDir := filepath.Clean(i.GameDir)
	game, err := os.OpenRoot(gameDir)
	if err != nil {
		return "", err
	}
	defer game.Close()

	folder := string(Mods)
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
	if !file.Mode().IsRegular() || !os.SameFile(f.location.file, file) {
		return "", fs.ErrInvalid
	}
	current, err := game.Lstat(folder)
	if err != nil {
		return "", err
	}
	if current.Mode().Type() != fs.ModeDir || !os.SameFile(directory, current) {
		return "", fs.ErrInvalid
	}
	return filepath.Join(gameDir, name), nil
}
