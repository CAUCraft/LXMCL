package transfer

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"path"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// ExtractZIPEntry writes one validated entry into a private staging directory.
func ExtractZIPEntry(ctx context.Context, root *os.Root, entry *zip.File, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if entry.FileInfo().IsDir() {
		return root.MkdirAll(name, 0755)
	}
	if err := root.MkdirAll(path.Dir(name), 0755); err != nil {
		return err
	}
	src, err := entry.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	n, err := io.Copy(dst, io.LimitReader(Reader(ctx, src), int64(entry.UncompressedSize64)+1))
	if err == nil && uint64(n) != entry.UncompressedSize64 {
		err = i18n.Error("saves.archive_size_mismatch")
	}
	return errors.Join(err, dst.Close())
}
