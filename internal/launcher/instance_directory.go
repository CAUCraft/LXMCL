package launcher

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// CheckNewInstanceDirectory reports whether the destination already contains an
// entry. A missing destination is not reserved or tested for write permission.
func CheckNewInstanceDirectory(directory string) (occupied bool, err error) {
	if !filepath.IsAbs(directory) {
		return false, i18n.Error("instance.game_root_required")
	}
	_, err = os.Lstat(directory)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, i18n.Error("settings.game_root_unusable", err)
	}
	if _, err := nearestInstanceDirectory(filepath.Dir(directory)); err != nil {
		return false, i18n.Error("settings.game_root_unusable", err)
	}
	return false, nil
}

// AvailableDiskSpace reports bytes available to the current user on the
// filesystem where the absolute directory is, or would be, located.
func AvailableDiskSpace(directory string) (uint64, error) {
	if !filepath.IsAbs(directory) {
		return 0, i18n.Error("instance.game_root_required")
	}
	ancestor, err := nearestInstanceDirectory(directory)
	if err != nil {
		return 0, err
	}
	return availableDiskSpace(ancestor)
}

// Distinguish absent directories from an existing non-directory or dangling
// link in the path. Registration separately performs its exclusive Mkdir.
func nearestInstanceDirectory(parent string) (string, error) {
	for {
		info, err := os.Lstat(parent)
		if err == nil {
			if !info.IsDir() {
				info, err = os.Stat(parent)
				if err != nil {
					return "", err
				}
			}
			if !info.IsDir() {
				return "", &os.PathError{Op: "stat", Path: parent, Err: syscall.ENOTDIR}
			}
			return parent, nil
		}
		next := filepath.Dir(parent)
		if !errors.Is(err, os.ErrNotExist) || next == parent {
			return "", err
		}
		parent = next
	}
}
