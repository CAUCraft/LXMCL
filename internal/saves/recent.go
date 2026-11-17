package saves

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type BackupEntry struct {
	Name     string
	Modified time.Time
	Size     int64
}

// Recent returns the three newest regular ZIPs in the instance backup directory.
// Reading the list does not open any archives or create directories.
func Recent(ctx context.Context, dataRoot string, id int) ([]BackupEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := os.Open(BackupsDir(dataRoot, id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	var recent []BackupEntry
	for {
		if err := ctx.Err(); err != nil {
			return recent, err
		}
		files, readErr := dir.ReadDir(64)
		for _, file := range files {
			if !file.Type().IsRegular() || !strings.EqualFold(filepath.Ext(file.Name()), ".zip") {
				continue
			}
			info, err := file.Info()
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return recent, err
			}
			if !info.Mode().IsRegular() {
				continue
			}
			entry := BackupEntry{Name: file.Name(), Modified: info.ModTime(), Size: info.Size()}
			at, _ := slices.BinarySearchFunc(recent, entry, func(a, b BackupEntry) int {
				if order := b.Modified.Compare(a.Modified); order != 0 {
					return order
				}
				return strings.Compare(a.Name, b.Name)
			})
			if at < 3 {
				recent = slices.Insert(recent, at, entry)
				recent = recent[:min(3, len(recent))]
			}
		}
		if errors.Is(readErr, io.EOF) {
			return recent, nil
		}
		if readErr != nil {
			return recent, readErr
		}
	}
}
