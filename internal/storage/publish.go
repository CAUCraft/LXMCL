package storage

import (
	"context"
	"crypto/rand"
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
func PublishNew(ctx context.Context, root *os.Root, name string, perm fs.FileMode, prepare func(*os.File) error) (err error) {
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
	var file *os.File
	var stage string
	for {
		stage = ".lxmcl-" + rand.Text() + ".part"
		file, err = root.OpenFile(stage, os.O_CREATE|os.O_EXCL|os.O_RDWR, perm)
		if !errors.Is(err, fs.ErrExist) {
			break
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if err != nil {
		return err
	}
	defer func() {
		file.Close()
		cleanup := root.Remove(stage)
		// Once published, cleanup failure must not turn a valid file into a failed
		// download. Never delete a destination that another operation may now own.
		if err != nil && cleanup != nil && !errors.Is(cleanup, fs.ErrNotExist) {
			err = errors.Join(err, cleanup)
		}
	}()
	if err := prepare(file); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	return publishNew(ctx, root, stage, name, file)
}
