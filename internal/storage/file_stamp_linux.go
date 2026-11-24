package storage

import (
	"io/fs"
	"os"
	"syscall"
)

func fileStampIdentity(_ *os.File, info fs.FileInfo) (uint64, uint64, int64, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, 0, fs.ErrInvalid
	}
	return uint64(stat.Dev), uint64(stat.Ino), stat.Ctim.Nano(), nil
}
