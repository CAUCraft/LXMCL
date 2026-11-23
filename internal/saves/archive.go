package saves

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

func writeBackup(ctx context.Context, root *os.Root, files []backupFile, dst io.Writer) (err error) {
	z := zip.NewWriter(dst)
	defer func() { err = errors.Join(err, z.Close()) }()
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := writeBackupFile(ctx, root, z, file); err != nil {
			return err
		}
	}
	return nil
}

func writeBackupFile(ctx context.Context, root *os.Root, z *zip.Writer, file backupFile) error {
	src, err := root.Open(filepath.FromSlash(file.name))
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(file.info, info) || info.Size() != file.info.Size() || !info.ModTime().Equal(file.info.ModTime()) {
		return i18n.Error("saves.backup_file_changed", file.name)
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name, header.Method = file.name, zip.Deflate
	dst, err := z.CreateHeader(header)
	if err != nil {
		return err
	}
	n, err := io.Copy(dst, io.LimitReader(transfer.Reader(ctx, src), info.Size()+1))
	if err != nil {
		return err
	}
	after, err := src.Stat()
	if err != nil {
		return err
	}
	if n != info.Size() || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return i18n.Error("saves.backup_file_changed", file.name)
	}
	return nil
}

func extractArchive(ctx context.Context, root *os.Root, entries []restoreEntry) error {
	for _, entry := range entries {
		if err := transfer.ExtractZIPEntry(ctx, root, entry.file, entry.name); err != nil {
			return err
		}
	}
	return nil
}
