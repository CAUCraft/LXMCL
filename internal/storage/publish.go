package storage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// ValidFilename accepts one portable filename, without directory components.
func ValidFilename(name string) bool {
	return ValidPortablePath(name) && !strings.Contains(name, "/")
}

// PublishNew prepares a file in the opened directory, then publishes it without
// replacing any existing entry. prepare must finish writing and validating the
// bytes before returning; it must not close or retain the file. The final name is
// absent until preparation, Sync and Close succeed. Cancellation is checked before
// publication; a committed file remains successful if cancellation arrives later.
// A process crash may leave a .part file, which does not block a subsequent retry.
func PublishNew(ctx context.Context, root *os.Root, name string, perm fs.FileMode, prepare func(*os.File) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !ValidFilename(name) {
		return i18n.Error("storage.filename_invalid")
	}
	if _, err := root.Lstat(name); !errors.Is(err, fs.ErrNotExist) {
		if err != nil {
			return err
		}
		return &os.PathError{Op: "publish", Path: name, Err: fs.ErrExist}
	}
	return prepareAndCommit(ctx, root, perm, prepare, func(stage string, file *os.File) error {
		return publishNew(ctx, root, stage, name, file)
	})
}
