package storage

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// ReplaceFile prepares a file in root, then creates or replaces name after Sync
// and Close succeed. name must be a single local filename. prepare must finish
// writing and validating the bytes without closing or retaining the file.
// Preparation failure or cancellation preserves the destination. A committed
// replacement remains successful if cancellation arrives later. Callers serialize
// changes to the same destination; publication uses the platform's Rename semantics.
func ReplaceFile(ctx context.Context, root *os.Root, name string, perm fs.FileMode, prepare func(*os.File) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if name == "." || !filepath.IsLocal(name) || filepath.Base(name) != name {
		return i18n.Error("storage.filename_invalid")
	}
	return prepareAndCommit(ctx, root, perm, prepare, func(stage string, file *os.File) error {
		if err := file.Close(); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return root.Rename(stage, name)
	})
}
