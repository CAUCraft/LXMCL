package content

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

// ErrPrismExportWorldsChanged indicates that the confirmed selection cannot be exported from the preview.
var ErrPrismExportWorldsChanged = i18n.Error("prism.export_worlds_changed")

// PrismExportWorlds is a read-only snapshot of a source instance's exportable worlds.
type PrismExportWorlds struct {
	instance instance.Instance
	saves    fs.FileInfo
	worlds   map[string]prismExportWorld
}

type prismExportWorld struct {
	directory fs.FileInfo
	files     []prismExportWorldFile
}

type prismExportWorldFile struct {
	name string
	info fs.FileInfo
}

// WorldFolders returns the exact folder names available for selection.
func (p *PrismExportWorlds) WorldFolders() []string {
	return slices.Sorted(maps.Keys(p.worlds))
}

// PreviewPrismExportWorlds lists exportable worlds without opening any file contents.
func PreviewPrismExportWorlds(ctx context.Context, i instance.Instance) (*PrismExportWorlds, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := i.Validate(); err != nil {
		return nil, err
	}
	if !i.Vanilla() {
		return nil, i18n.Error("prism.version")
	}
	p := &PrismExportWorlds{instance: i, worlds: make(map[string]prismExportWorld)}
	game, err := os.OpenRoot(i.GameDir)
	if errors.Is(err, fs.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	defer game.Close()
	root, info, err := openPrismExportDir(game, "saves")
	if errors.Is(err, fs.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	p.saves = info
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, files := 0, 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		batch, err := dir.ReadDir(min(128, storage.MaxPackEntries+1-entries))
		entries += len(batch)
		if entries > storage.MaxPackEntries {
			return nil, i18n.Error("prism.export_worlds_preview_limit")
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		for _, entry := range batch {
			name := entry.Name()
			if !entry.IsDir() || !storage.ValidFilename(name) {
				continue
			}
			world, info, openErr := openPrismExportDir(root, name)
			if openErr != nil {
				continue
			}
			snapshot, scanErr := planPrismExportWorld(ctx, world, name)
			world.Close()
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			if scanErr != nil || !prismExportDirMatches(root, name, info) {
				continue
			}
			files += len(snapshot)
			if files > storage.MaxPackEntries {
				return nil, i18n.Error("prism.export_worlds_preview_limit")
			}
			p.worlds[name] = prismExportWorld{directory: info, files: snapshot}
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}
	if !prismExportDirMatches(game, "saves", p.saves) {
		return nil, ErrPrismExportWorldsChanged
	}
	return p, nil
}

// ExportPrismSelectedWorlds exports only the accepted worlds and optional instance files.
func ExportPrismSelectedWorlds(ctx context.Context, i instance.Instance, filename string, files PrismFiles, preview *PrismExportWorlds, selected []string, report func(tasks.Progress)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if preview == nil || preview.instance != i || !files.Worlds || files.WorldFolders != nil ||
		len(selected) == 0 || len(selected) > len(preview.worlds) {
		return ErrPrismExportWorldsChanged
	}
	names := slices.Clone(selected)
	slices.Sort(names)
	for n, name := range names {
		if _, found := preview.worlds[name]; !found || (n > 0 && name == names[n-1]) {
			return ErrPrismExportWorldsChanged
		}
	}
	return exportPrismFiles(ctx, i, filename, files, func(add prismExportAdd) error {
		return preview.write(ctx, names, add)
	}, report)
}

func (p *PrismExportWorlds) write(ctx context.Context, names []string, add prismExportAdd) error {
	game, err := os.OpenRoot(p.instance.GameDir)
	if err != nil {
		return ErrPrismExportWorldsChanged
	}
	defer game.Close()
	root, info, err := openPrismExportDir(game, "saves")
	if err != nil {
		return ErrPrismExportWorldsChanged
	}
	defer root.Close()
	if !os.SameFile(p.saves, info) {
		return ErrPrismExportWorldsChanged
	}
	var writeErr error
	for _, name := range names {
		if writeErr = p.visitWorld(ctx, root, name, add); writeErr != nil {
			break
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, name := range names {
		if err := p.visitWorld(ctx, root, name, nil); err != nil {
			return err
		}
	}
	if !prismExportDirMatches(game, "saves", p.saves) {
		return ErrPrismExportWorldsChanged
	}
	return writeErr
}

func (p *PrismExportWorlds) visitWorld(ctx context.Context, root *os.Root, name string, add prismExportAdd) error {
	world, dir, err := openPrismExportDir(root, name)
	if err != nil {
		return ErrPrismExportWorldsChanged
	}
	defer world.Close()
	expected := p.worlds[name]
	if !os.SameFile(expected.directory, dir) {
		return ErrPrismExportWorldsChanged
	}
	now, err := planPrismExportWorld(ctx, world, name)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil || !slices.EqualFunc(expected.files, now, func(a, b prismExportWorldFile) bool {
		return a.name == b.name && os.SameFile(a.info, b.info) && a.info.Size() == b.info.Size() &&
			a.info.ModTime().Equal(b.info.ModTime())
	}) || !prismExportDirMatches(root, name, dir) {
		return ErrPrismExportWorldsChanged
	}
	if add != nil {
		for _, file := range now {
			if err := add(world, file.name, "saves/"+name+"/"+file.name, file.info); err != nil {
				return err
			}
		}
	}
	return nil
}

func planPrismExportWorld(ctx context.Context, root *os.Root, folder string) ([]prismExportWorldFile, error) {
	level, err := root.Lstat("level.dat")
	if err != nil {
		return nil, err
	}
	if !level.Mode().IsRegular() {
		return nil, ErrPrismExportWorldsChanged
	}
	var files []prismExportWorldFile
	var total int64
	paths := make(storage.ArchivePaths)
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			if top, _, _ := strings.Cut(name, "/"); name != "." && !prismWorldDataFolder(top) {
				return fs.SkipDir
			}
			return nil
		}
		if !prismWorldData(name) {
			return nil
		}
		if err := paths.Add("instance/.minecraft/saves/"+folder+"/"+name, false); err != nil {
			return err
		}
		info, err := root.Lstat(filepath.FromSlash(name))
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return ErrPrismExportWorldsChanged
		}
		if len(files) >= storage.MaxPackEntries-2 || info.Size() > distribution.MaxFileSize || info.Size() > storage.MaxPackBytes-total {
			return i18n.Error("modpack.archive_expanded_too_large")
		}
		total += info.Size()
		files = append(files, prismExportWorldFile{name, info})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(files, func(file prismExportWorldFile) bool {
		return file.name == "level.dat" && os.SameFile(level, file.info) && level.Size() == file.info.Size() &&
			level.ModTime().Equal(file.info.ModTime())
	}) {
		return nil, ErrPrismExportWorldsChanged
	}
	return files, nil
}

func openPrismExportDir(parent *os.Root, name string) (*os.Root, fs.FileInfo, error) {
	expected, err := parent.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if expected.Mode().Type() != fs.ModeDir {
		return nil, nil, i18n.Error("saves.path_invalid", path.Join(parent.Name(), name))
	}
	root, err := parent.OpenRoot(name)
	if err != nil {
		return nil, nil, err
	}
	current, err := root.Stat(".")
	if err != nil || !os.SameFile(expected, current) {
		root.Close()
		return nil, nil, ErrPrismExportWorldsChanged
	}
	return root, current, nil
}

func prismExportDirMatches(parent *os.Root, name string, expected fs.FileInfo) bool {
	info, err := parent.Lstat(name)
	return err == nil && info.Mode().Type() == fs.ModeDir && os.SameFile(expected, info)
}
