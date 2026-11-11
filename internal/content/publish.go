package content

import (
	"context"
	"errors"
	"io/fs"
	"os"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

// prepare writes exactly size verified bytes. Local imports and remote downloads
// share the archive checks, disabled-file protection and publication lifecycle.
func publishResource(ctx context.Context, i instance.Instance, kind Kind, name string, size int64, prepare func(*os.File) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateResource(i, kind, name, size); err != nil {
		return err
	}
	root, err := OpenDirectory(i, kind, true)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := checkDisabledConflict(root, name); err != nil {
		return err
	}
	return storage.PublishNew(ctx, root, name, 0644, func(f *os.File) error {
		if err := prepare(f); err != nil {
			return err
		}
		if err := ValidateArchive(ctx, i, kind, f, size); err != nil {
			return err
		}
		return checkDisabledConflict(root, name)
	})
}

func validateResource(i instance.Instance, kind Kind, name string, size int64) error {
	ext, _, err := kind.Format(i)
	if err != nil {
		return err
	}
	if !ValidName(name, ext) || !storage.ValidFilename(name) || size <= 0 || size > MaxSize {
		return i18n.Error("resource.filename_or_size_invalid")
	}
	return nil
}

func checkDisabledConflict(root *os.Root, name string) error {
	_, err := root.Lstat(name + ".disabled")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return i18n.Error("resource.disabled_conflict")
}
