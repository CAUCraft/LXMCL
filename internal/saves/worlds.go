package saves

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// World is a read-only snapshot of one directory and its level.dat.
// Keep it with the captured instance until the player confirms an action.
type World struct {
	folder           string
	directory, level fs.FileInfo
	levelName        string
	lastPlayed       time.Time
	savedVersion     string
}

func (w World) Folder() string { return w.folder }

// LevelName is the saved in-game name, or empty when unavailable.
func (w World) LevelName() string { return w.levelName }

// LastPlayed is the timestamp saved by the game, or zero when unavailable.
func (w World) LastPlayed() time.Time { return w.lastPlayed }

// SavedVersion is the version name recorded in level.dat, or empty when unknown.
func (w World) SavedVersion() string { return w.savedVersion }

// Worlds lists immediate world directories without creating an absent saves folder.
func Worlds(ctx context.Context, gameDir string) ([]World, error) {
	root, err := savesRoot(gameDir, false)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, err
	}
	var out []World
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !e.IsDir() {
			continue
		}
		world, err := readWorld(root, e.Name())
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		world.levelName, world.lastPlayed, world.savedVersion = readWorldMetadata(ctx, root, world)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out = append(out, world)
	}
	return out, nil
}

func readWorld(root *os.Root, name string) (World, error) {
	dir, err := root.Lstat(name)
	if err != nil {
		return World{}, err
	}
	if !dir.IsDir() {
		return World{}, fs.ErrNotExist
	}
	level, err := root.Lstat(filepath.Join(name, "level.dat"))
	if err != nil {
		return World{}, err
	}
	if !level.Mode().IsRegular() {
		return World{}, fs.ErrNotExist
	}
	return World{folder: name, directory: dir, level: level}, nil
}

func (w World) Check(gameDir string) error {
	root, err := savesRoot(gameDir, false)
	if err == nil {
		defer root.Close()
		return w.check(root)
	}
	if errors.Is(err, fs.ErrNotExist) {
		return i18n.Error("saves.world_changed", w.folder)
	}
	return err
}

func (w World) check(root *os.Root) error {
	if w.directory == nil || w.level == nil {
		return i18n.Error("saves.world_changed", w.folder)
	}
	now, err := readWorld(root, w.folder)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err != nil || !os.SameFile(w.directory, now.directory) || !os.SameFile(w.level, now.level) || w.level.Size() != now.level.Size() || !w.level.ModTime().Equal(now.level.ModTime()) {
		return i18n.Error("saves.world_changed", w.folder)
	}
	return nil
}
