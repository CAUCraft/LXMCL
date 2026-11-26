//go:build !windows

package screenshots

import "io/fs"

func isExportReparsePoint(fs.FileInfo) bool { return false }
