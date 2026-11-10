package content

import (
	"context"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// WriteVerified publishes a new resource after size, optional digest and archive
// checks. Callers must serialize resource changes for the same instance; enabled
// and disabled filenames cannot be checked atomically as a pair.
func WriteVerified(ctx context.Context, i instance.Instance, kind Kind, name string, in io.Reader, size int64, digest hash.Hash, expected string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ext, _, err := kind.Format(i)
	if err != nil {
		return err
	}
	if !ValidName(name, ext) || !storage.ValidFilename(name) || size <= 0 || size > MaxSize {
		return i18n.Error("resource.filename_or_size_invalid")
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
		var out io.Writer = f
		if digest != nil {
			out = io.MultiWriter(f, digest)
		}
		n, err := io.Copy(out, io.LimitReader(transfer.Reader(ctx, in), size+1))
		if err != nil {
			return err
		}
		if n != size || digest != nil && !strings.EqualFold(fmt.Sprintf("%x", digest.Sum(nil)), expected) {
			return i18n.Error("resource.checksum_failed")
		}
		if err := ValidateArchive(ctx, i, kind, f, n); err != nil {
			return err
		}
		return checkDisabledConflict(root, name)
	})
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
