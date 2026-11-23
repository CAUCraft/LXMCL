package saves

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

// BackupRecord identifies the basename recorded by a completed backup task.
type BackupRecord struct {
	Name string
	At   time.Time
}

// LocalBackups lists managed ZIPs for one instance and recorded backups that
// are missing. It does not open archives or create directories.
func LocalBackups(ctx context.Context, dataRoot string, id int, known []BackupRecord) ([]BackupEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	missing := make(map[string]time.Time, len(known))
	for _, record := range known {
		previous, exists := missing[record.Name]
		if validBackupName(record.Name) && (!exists || record.At.After(previous)) {
			missing[record.Name] = record.At
		}
	}
	var entries []BackupEntry
	root, err := openManagedBackups(dataRoot, id)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		defer root.Close()
		dir, err := root.Open(".")
		if err != nil {
			return nil, err
		}
		defer dir.Close()
		err = visitBackupFiles(ctx, dir, func(name string, info fs.FileInfo) {
			_, recorded := missing[name]
			if isBackupReparsePoint(info) || !validBackupName(name) || (!recorded && !legacyBackupName(name)) {
				return
			}
			delete(missing, name)
			entries = append(entries, BackupEntry{Name: name, Modified: info.ModTime(), Size: info.Size()})
		})
		if err != nil {
			return nil, err
		}
	}
	for name, at := range missing {
		entries = append(entries, BackupEntry{Name: name, Modified: at, Missing: true})
	}
	slices.SortFunc(entries, compareBackups)
	return entries, ctx.Err()
}

// CheckLocalBackup verifies the named regular ZIP in the managed directory
// before it is handed to the standard restore preview.
func CheckLocalBackup(dataRoot string, id int, name string) (string, error) {
	if !validBackupName(name) {
		return "", i18n.Error("saves.path_invalid", name)
	}
	root, err := openManagedBackups(dataRoot, id)
	if err != nil {
		return "", err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || isBackupReparsePoint(info) {
		return "", i18n.Error("saves.path_invalid", name)
	}
	return filepath.Join(BackupsDir(dataRoot, id), name), nil
}

func validBackupName(name string) bool {
	return filepath.Base(name) == name && strings.EqualFold(filepath.Ext(name), ".zip") && storage.ValidPortablePath(name)
}

func legacyBackupName(name string) bool {
	const stamp = "20060102-150405"
	const suffix = 6
	if len(name) != len(stamp)+1+suffix+len(".zip") || name[len(stamp)] != '-' || !strings.HasSuffix(name, ".zip") {
		return false
	}
	if _, err := time.Parse(stamp, name[:len(stamp)]); err != nil {
		return false
	}
	for _, ch := range name[len(stamp)+1 : len(stamp)+1+suffix] {
		if !(ch >= 'A' && ch <= 'Z' || ch >= '2' && ch <= '7') {
			return false
		}
	}
	return true
}

// Open each directory component from a scoped root. Following a link even
// within the launcher tree would allow one instance to show another's archives.
func openManagedBackups(dataRoot string, id int) (*os.Root, error) {
	path := BackupsDir(dataRoot, id)
	if id <= 0 || !filepath.IsAbs(dataRoot) {
		return nil, i18n.Error("saves.path_invalid", path)
	}
	root, err := os.OpenRoot(filepath.Clean(dataRoot))
	if err != nil {
		return nil, err
	}
	for _, part := range []string{"backups", filepath.Base(path)} {
		child, err := openBackupDirectory(root, part)
		root.Close()
		if err != nil {
			return nil, err
		}
		root = child
	}
	return root, nil
}

func openBackupDirectory(root *os.Root, name string) (*os.Root, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if info.Mode().Type() != fs.ModeDir || isBackupReparsePoint(info) {
		return nil, i18n.Error("saves.path_invalid", filepath.Join(root.Name(), name))
	}
	child, err := root.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	current, err := child.Stat(".")
	if err == nil && os.SameFile(info, current) && !isBackupReparsePoint(current) {
		return child, nil
	}
	child.Close()
	if err != nil {
		return nil, err
	}
	return nil, i18n.Error("saves.path_invalid", filepath.Join(root.Name(), name))
}
