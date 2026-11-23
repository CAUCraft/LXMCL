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
	if !storage.ValidFilename(name) || !strings.EqualFold(filepath.Ext(name), ".png") || !filepath.IsAbs(folder) {
		return fs.ErrInvalid
	}
	source, err := open(gameDir)
	if err != nil {
		return err
	}
	defer source.Close()
	before, err := source.Lstat(name)
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() {
		return fs.ErrInvalid
	}
	in, err := source.Open(name)
	if err != nil {
		return err
	}
	defer in.Close()
	dst, err := os.OpenRoot(folder)
	if err != nil {
		return err
	}
	defer dst.Close()
	err = storage.PublishNew(ctx, dst, name, 0600, func(out *os.File) error {
		n, err := io.Copy(out, transfer.Reader(ctx, in))
		if err != nil {
			return err
		}
		after, err := in.Stat()
		if err != nil {
			return err
		}
		if n != before.Size() || after.Size() != n || !before.ModTime().Equal(after.ModTime()) || !os.SameFile(before, after) {
			return i18n.Error("screenshot.changed")
		}
		return nil
	})
	if errors.Is(err, fs.ErrExist) {
		return i18n.Error("screenshot.export_conflict", name)
	}
	return err
}
