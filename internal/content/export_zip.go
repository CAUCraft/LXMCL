package content

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

// publishInstanceZIP writes a new ZIP at the chosen path outside the source instance.
func publishInstanceZIP(ctx context.Context, gameDir, filename string, prepare func(*os.File) error) error {
	parent, name := filepath.Dir(filename), filepath.Base(filename)
	if !filepath.IsAbs(filename) || !storage.ValidFilename(name) || !strings.EqualFold(filepath.Ext(name), ".zip") {
		return i18n.Error("archive.export_target")
	}
	if err := CheckSeparateDirectory(gameDir, parent); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	out, err := os.OpenRoot(parent)
	if err != nil {
		return err
	}
	defer out.Close()
	err = storage.PublishNew(ctx, out, name, 0600, prepare)
	if errors.Is(err, fs.ErrExist) {
		return i18n.Error("archive.export_exists", err)
	}
	return err
}
