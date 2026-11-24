//go:build !linux && !windows

package storage

import (
	"io/fs"
	"os"
)

func fileStampIdentity(_ *os.File, _ fs.FileInfo) (uint64, uint64, int64, error) {
	return 0, 0, 0, fs.ErrInvalid
}
