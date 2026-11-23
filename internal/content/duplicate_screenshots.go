package content

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/screenshots"
)

// CopyScreenshots copies exactly the PNG files shown in the instance gallery
// into the new instance's staging directory, preserving gallery order by time.
func CopyScreenshots(ctx context.Context, gameDir string, destination *os.Root) error {
	entries, err := screenshots.List(ctx, gameDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil || len(entries) == 0 {
		return err
	}
	source, err := os.OpenRoot(gameDir)
	if err != nil {
		return err
	}
	defer source.Close()
	if err := destination.MkdirAll(screenshots.Directory, 0755); err != nil {
		return err
	}
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(screenshots.Directory, e.Name)
		info, err := source.Lstat(path)
		if err != nil {
			return err
		}
		if err := copySetupFile(ctx, source, destination, path, info); err != nil {
			return err
		}
	}
	return nil
}
