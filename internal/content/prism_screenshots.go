package content

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/screenshots"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/tasks"
)

type prismDirectoryScreenshots struct {
	source, game, gallery fs.FileInfo
	files                 []prismScreenshotFile
}

type prismScreenshotFile struct {
	name string
	info fs.FileInfo
}

// Use the same direct, regular PNG scope as the gallery without walking subdirectories.
func visitPrismScreenshots(ctx context.Context, root *os.Root, scanLimit int, visit func(string, fs.FileInfo) error) error {
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	count := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		batch := 128
		if scanLimit > 0 {
			batch = min(batch, scanLimit+1-count)
		}
		entries, readErr := dir.ReadDir(batch)
		count += len(entries)
		if scanLimit > 0 && count > scanLimit {
			return i18n.Error("modpack.archive_too_many_files")
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		for _, entry := range entries {
			name := entry.Name()
			if !entry.Type().IsRegular() || !strings.EqualFold(filepath.Ext(name), ".png") {
				continue
			}
			info, err := root.Lstat(name)
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return i18n.Error("instance.duplicate_source_changed")
			}
			if err := visit(name, info); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			return ctx.Err()
		}
	}
}

// Open a real child directory and bind the handle to the observed directory entry.
func openPrismScreenshotDir(parent *os.Root, name string) (*os.Root, fs.FileInfo, error) {
	info, err := parent.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if info.Mode().Type() != fs.ModeDir {
		return nil, nil, i18n.Error("instance.duplicate_file_unsupported", name)
	}
	root, err := parent.OpenRoot(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, i18n.Error("instance.duplicate_source_changed")
	}
	if err != nil {
		return nil, nil, err
	}
	if !prismExportDirMatches(root, ".", info) || !prismExportDirMatches(parent, name, info) {
		root.Close()
		return nil, nil, i18n.Error("instance.duplicate_source_changed")
	}
	return root, info, nil
}

func openPrismScreenshotSource(dir string) (*os.Root, fs.FileInfo, error) {
	dir = filepath.Clean(dir)
	parentPath, name := filepath.Dir(dir), filepath.Base(dir)
	if parentPath == dir {
		name = "."
	}
	parent, err := os.OpenRoot(parentPath)
	if err != nil {
		return nil, nil, err
	}
	defer parent.Close()
	return openPrismScreenshotDir(parent, name)
}

// Snapshot the eligible files without reading image contents. An unavailable
// optional gallery does not prevent importing other parts of the instance.
func readPrismDirectoryScreenshots(ctx context.Context, profile *os.Root, p PrismInstance) *prismDirectoryScreenshots {
	if p.GameFolder == "" {
		return nil
	}
	source, info, err := openPrismScreenshotSource(p.Source)
	if err != nil {
		return nil
	}
	defer source.Close()
	if !prismExportDirMatches(profile, ".", info) {
		return nil
	}
	game, gameInfo, err := openPrismScreenshotDir(source, p.GameFolder)
	if err != nil {
		return nil
	}
	defer game.Close()
	gallery, galleryInfo, err := openPrismScreenshotDir(game, screenshots.Directory)
	if err != nil {
		return nil
	}
	defer gallery.Close()
	pictures := &prismDirectoryScreenshots{source: info, game: gameInfo, gallery: galleryInfo}
	paths := make(storage.ArchivePaths)
	var total int64
	err = visitPrismScreenshots(ctx, gallery, storage.MaxPackEntries, func(name string, info fs.FileInfo) error {
		if err := paths.Add(screenshots.Directory+"/"+name, false); err != nil {
			return err
		}
		if info.Size() > distribution.MaxFileSize || info.Size() > storage.MaxPackBytes-total {
			return i18n.Error("modpack.archive_expanded_too_large")
		}
		total += info.Size()
		pictures.files = append(pictures.files, prismScreenshotFile{name, info})
		return nil
	})
	if err != nil || len(pictures.files) == 0 || !pictures.matches(source, p.GameFolder, game) {
		return nil
	}
	return pictures
}

func (p *prismDirectoryScreenshots) matches(source *os.Root, gameName string, game *os.Root) bool {
	return prismExportDirMatches(source, gameName, p.game) && prismExportDirMatches(game, screenshots.Directory, p.gallery)
}

func (p *prismDirectoryScreenshots) copy(ctx context.Context, source *os.Root, gameName string, game, dst *os.Root, report func(tasks.Progress)) error {
	root, info, err := openPrismScreenshotDir(game, screenshots.Directory)
	if err != nil {
		return err
	}
	defer root.Close()
	if !os.SameFile(info, p.gallery) || !p.matches(source, gameName, game) {
		return i18n.Error("prism.changed")
	}
	if err := dst.Mkdir(screenshots.Directory, 0755); err != nil {
		return err
	}
	out, err := dst.OpenRoot(screenshots.Directory)
	if err != nil {
		return err
	}
	defer out.Close()
	for i, file := range p.files {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := root.Lstat(file.name)
		if err != nil || !info.Mode().IsRegular() || !os.SameFile(info, file.info) {
			return i18n.Error("prism.changed")
		}
		if report != nil {
			report(tasks.Progress{Stage: i18n.Text("progress.copy_instance_file", screenshots.Directory+"/"+file.name), Done: i})
		}
		if err := copySetupFile(ctx, root, out, file.name, file.info); err != nil {
			return err
		}
	}
	if !p.matches(source, gameName, game) {
		return i18n.Error("prism.changed")
	}
	return ctx.Err()
}
