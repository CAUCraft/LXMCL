//go:build !windows

package saves

import "io/fs"

func isBackupReparsePoint(info fs.FileInfo) bool {
	return info.Mode()&fs.ModeSymlink != 0
}
