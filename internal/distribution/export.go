package distribution

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

// Export saves a verified copy in the chosen directory without overwriting files.
// The directory is opened before downloading and stays bound throughout the
// operation. report receives (0,1) before download and (1,1) only after publication.
func (c Cache) Export(ctx context.Context, a Asset, name, directory string, report func(done, total int)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !storage.ValidFilename(name) {
		return i18n.Error("storage.filename_invalid")
	}
	if !filepath.IsAbs(directory) {
		return i18n.Error("download.folder_required")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return i18n.Error("download.folder_open_failed", err)
	}
	defer root.Close()
	if _, err := root.Lstat(name); err == nil {
		return i18n.Error("download.folder_name_conflict", name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if report != nil {
		report(0, 1)
	}
	verified, err := c.Ensure(ctx, a)
	if err != nil {
		return err
	}
	source, err := os.Open(verified)
	if err != nil {
		return err
	}
	defer source.Close()
	err = storage.PublishNew(ctx, root, name, 0644, func(file *os.File) error {
		return a.CopyVerified(ctx, file, source)
	})
	if err != nil {
		return err
	}
	if report != nil {
		report(1, 1)
	}
	return nil
}
