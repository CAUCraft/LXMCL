package transfer

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// WriteZIPFile writes a regular file whose identity, size and timestamp match the snapshot.
func WriteZIPFile(ctx context.Context, root *os.Root, z *zip.Writer, source, name string, expected fs.FileInfo) error {
	src, err := root.Open(filepath.FromSlash(source))
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(expected, info) || info.Size() != expected.Size() || !info.ModTime().Equal(expected.ModTime()) {
		return i18n.Error("saves.backup_file_changed", source)
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name, header.Method = name, zip.Deflate
	dst, err := z.CreateHeader(header)
	if err != nil {
		return err
	}
	n, err := io.Copy(dst, io.LimitReader(Reader(ctx, src), info.Size()+1))
	if err != nil {
		return err
	}
	after, err := src.Stat()
	if err != nil {
		return err
	}
	if n != info.Size() || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return i18n.Error("saves.backup_file_changed", source)
	}
	return nil
}

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
