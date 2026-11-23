// Package shortcut creates instance shortcuts for the current launcher.
package shortcut

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/storage"
)

const Supported = runtime.GOOS == "windows"

// Create publishes a new .lnk only after Windows has serialized the launcher target.
func Create(ctx context.Context, filename, launcherPath, profile string, id int) error {
	if id <= 0 || !filepath.IsAbs(launcherPath) || !filepath.IsAbs(profile) || !filepath.IsAbs(filename) {
		return fs.ErrInvalid
	}
	if filepath.Ext(filename) == "" {
		filename += ".lnk"
	}
	if !strings.EqualFold(filepath.Ext(filename), ".lnk") {
		return errors.New("请选择 .lnk 快捷方式文件名")
	}
	root, err := os.OpenRoot(filepath.Dir(filename))
	if err != nil {
		return err
	}
	defer root.Close()
	return storage.PublishNew(ctx, root, filepath.Base(filename), 0600, func(file *os.File) error {
		return writeLink(file, launcherPath, profile, id)
	})
}
