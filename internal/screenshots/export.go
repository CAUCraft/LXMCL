package screenshots

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// Export copies the original PNG bytes to a new file in the selected directory.
func Export(ctx context.Context, gameDir, name, folder string) error {
	if !validPNGName(name) || !filepath.IsAbs(folder) {
		return fs.ErrInvalid
	}
	source, err := open(gameDir)
	if err != nil {
		return err
	}
	defer source.Close()
	dst, err := os.OpenRoot(folder)
	if err != nil {
		return err
	}
	defer dst.Close()
	err = exportPNG(ctx, source, dst, name, nil)
	if errors.Is(err, fs.ErrExist) {
		return i18n.Error("screenshot.export_conflict", name)
	}
	return err
}

type BatchExport struct {
	Exported, Skipped, Failed int
}

// ExportBatch copies the listed originals. Already published files stay in the
// destination if the batch stops, and existing names count as skips.
func ExportBatch(ctx context.Context, gameDir string, entries []Entry, folder string, report func(name string, done int)) (result BatchExport, err error) {
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if len(entries) == 0 {
		return result, fs.ErrInvalid
	}
	game, err := openExportDirectory(gameDir)
	if err != nil {
		return result, err
	}
	source, err := checkedExportChild(game, Directory)
	game.Close()
	if err != nil {
		return result, err
	}
	defer source.Close()
	dst, err := openExportDirectory(folder)
	if err != nil {
		return result, err
	}
	defer dst.Close()
	var firstError error
	for n, entry := range entries {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if report != nil {
			report(entry.Name, n)
		}
		err := fs.ErrInvalid
		if validPNGName(entry.Name) && entry.info != nil {
			err = exportPNG(ctx, source, dst, entry.Name, entry.info)
		}
		switch {
		case err == nil:
			result.Exported++
		case errors.Is(err, fs.ErrExist):
			result.Skipped++
		case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
			return result, err
		default:
			result.Failed++
			if firstError == nil {
				firstError = err
			}
		}
	}
	return result, firstError
}

func validPNGName(name string) bool {
	return storage.ValidFilename(name) && strings.EqualFold(filepath.Ext(name), ".png")
}

func exportPNG(ctx context.Context, source, dst *os.Root, name string, expected fs.FileInfo) error {
	return storage.PublishNew(ctx, dst, name, 0600, func(out *os.File) error {
		before, err := source.Lstat(name)
		if err != nil {
			return err
		}
		if !before.Mode().IsRegular() || expected != nil && (!sameScreenshot(expected, before) || isExportReparsePoint(before)) {
			return i18n.Error("screenshot.changed")
		}
		matches := func(info fs.FileInfo) bool {
			return sameScreenshot(before, info) && (expected == nil || !isExportReparsePoint(info))
		}
		in, err := source.Open(name)
		if err != nil {
			return err
		}
		defer in.Close()
		opened, err := in.Stat()
		if err != nil {
			return err
		}
		current, err := source.Lstat(name)
		if err != nil {
			return err
		}
		if !matches(opened) || !matches(current) {
			return i18n.Error("screenshot.changed")
		}
		n, err := io.Copy(out, transfer.Reader(ctx, io.LimitReader(in, before.Size()+1)))
		if err != nil {
			return err
		}
		after, err := in.Stat()
		if err != nil {
			return err
		}
		current, err = source.Lstat(name)
		if err != nil {
			return err
		}
		if n != before.Size() || !matches(after) || !matches(current) {
			return i18n.Error("screenshot.changed")
		}
		return nil
	})
}

func sameScreenshot(before, current fs.FileInfo) bool {
	return current.Mode().IsRegular() && os.SameFile(before, current) &&
		before.Size() == current.Size() && before.ModTime().Equal(current.ModTime())
}

// openExportDirectory walks a selected absolute path from its volume root and
// rejects linked components. It hands callers the verified directory handle.
func openExportDirectory(path string) (*os.Root, error) {
	if !filepath.IsAbs(path) || strings.Contains("/"+filepath.ToSlash(path)+"/", "/../") {
		return nil, fs.ErrInvalid
	}
	path = filepath.Clean(path)
	base := filepath.VolumeName(path) + string(os.PathSeparator)
	root, err := os.OpenRoot(base)
	if err != nil {
		return nil, err
	}
	relative := strings.TrimPrefix(path, base)
	if relative == "" {
		return root, nil
	}
	for _, name := range strings.Split(relative, string(os.PathSeparator)) {
		next, err := checkedExportChild(root, name)
		root.Close()
		if err != nil {
			return nil, err
		}
		root = next
	}
	return root, nil
}

func checkedExportChild(parent *os.Root, name string) (*os.Root, error) {
	before, err := parent.Lstat(name)
	if err != nil {
		return nil, err
	}
	if before.Mode().Type() != fs.ModeDir || isExportReparsePoint(before) {
		return nil, i18n.Error("screenshot.export_directory_changed")
	}
	root, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	actual, err := root.Stat(".")
	if err == nil {
		var current fs.FileInfo
		current, err = parent.Lstat(name)
		if err == nil && current.Mode().Type() == fs.ModeDir && !isExportReparsePoint(current) && !isExportReparsePoint(actual) &&
			os.SameFile(before, actual) && os.SameFile(before, current) {
			return root, nil
		}
	}
	root.Close()
	if err != nil {
		return nil, err
	}
	return nil, i18n.Error("screenshot.export_directory_changed")
}
