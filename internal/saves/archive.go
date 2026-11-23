package saves

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"

	"github.com/CAUCraft/LXMCL/internal/transfer"
)

func writeBackup(ctx context.Context, root *os.Root, files []backupFile, dst io.Writer) (err error) {
	z := zip.NewWriter(dst)
	defer func() { err = errors.Join(err, z.Close()) }()
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := transfer.WriteZIPFile(ctx, root, z, file.name, file.name, file.info); err != nil {
			return err
		}
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
