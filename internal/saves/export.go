package saves

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

// ErrLocalBackupMissing indicates that the source no longer exists.
var ErrLocalBackupMissing = fmt.Errorf("local backup source: %w", fs.ErrNotExist)

// ErrLocalBackupChanged indicates that a new listing is needed before export.
var ErrLocalBackupChanged = i18n.Error("saves.archive_changed")

// ExportLocalBackup copies the selected managed backup to a new ZIP without
// opening its archive contents. The entry must come from LocalBackups.
func ExportLocalBackup(ctx context.Context, dataRoot string, id int, entry BackupEntry, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if entry.Missing {
		return ErrLocalBackupMissing
	}
	if entry.info == nil || !validBackupName(entry.Name) || entry.path != filepath.Join(BackupsDir(dataRoot, id), entry.Name) {
		return ErrLocalBackupChanged
	}
	name := filepath.Base(destination)
	if !filepath.IsAbs(destination) || !validBackupName(name) {
		return i18n.Error("saves.path_invalid", destination)
	}
	source, err := openManagedBackups(dataRoot, id)
	if err != nil {
		return backupSourceError(err)
	}
	defer source.Close()
	if err := checkBackupSnapshot(source, entry); err != nil {
		return err
	}
	in, err := source.Open(entry.Name)
	if err != nil {
		return backupSourceError(err)
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return backupSourceError(err)
	}
	if !sameBackupFile(entry.info, info) {
		return ErrLocalBackupChanged
	}

	dst, err := os.OpenRoot(filepath.Dir(filepath.Clean(destination)))
	if err != nil {
		return err
	}
	defer dst.Close()
	return storage.PublishNew(ctx, dst, name, 0600, func(out *os.File) error {
		_, err := io.CopyN(out, transfer.Reader(ctx, in), entry.info.Size())
		if errors.Is(err, io.EOF) {
			return ErrLocalBackupChanged
		}
		if err != nil {
			return err
		}
		after, err := in.Stat()
		if err != nil {
			return backupSourceError(err)
		}
		if !sameBackupFile(entry.info, after) {
			return ErrLocalBackupChanged
		}
		return checkBackupSnapshot(source, entry)
	})
}

func checkBackupSnapshot(source *os.Root, entry BackupEntry) error {
	info, err := source.Lstat(entry.Name)
	if err != nil {
		return backupSourceError(err)
	}
	if !sameBackupFile(entry.info, info) {
		return ErrLocalBackupChanged
	}
	return nil
}

func sameBackupFile(expected, current fs.FileInfo) bool {
	return regularBackup(current) && os.SameFile(expected, current) &&
		expected.Size() == current.Size() && expected.ModTime().Equal(current.ModTime())
}

func backupSourceError(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return ErrLocalBackupMissing
	}
	return err
}
