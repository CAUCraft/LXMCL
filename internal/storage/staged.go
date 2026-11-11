package storage

import (
	"context"
	"crypto/rand"
	"errors"
	"io/fs"
	"os"
)

// prepareAndCommit owns one temporary file and its cleanup. commit receives an
// open, synced file; it must close the writer and check cancellation before
// publishing. Windows PublishNew needs the open handle to prepare its rename.
func prepareAndCommit(ctx context.Context, root *os.Root, perm fs.FileMode, prepare func(*os.File) error, commit func(string, *os.File) error) (err error) {
	if err := ctx.Err(); err != nil {
		return err
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
		// Once committed, cleanup must not turn success into a failed operation.
		// The final destination may now belong to another writer; never remove it.
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
	return commit(stage, file)
}
