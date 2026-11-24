package launcher

import (
	"math"
	"os"
	"syscall"
)

func availableDiskSpace(directory string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(directory, &stat); err != nil {
		return 0, &os.PathError{Op: "statfs", Path: directory, Err: err}
	}
	blockSize := stat.Frsize
	if blockSize <= 0 {
		blockSize = stat.Bsize
	}
	if blockSize <= 0 {
		return 0, &os.PathError{Op: "statfs", Path: directory, Err: syscall.EINVAL}
	}
	if stat.Bavail > math.MaxUint64/uint64(blockSize) {
		return 0, &os.PathError{Op: "statfs", Path: directory, Err: syscall.EOVERFLOW}
	}
	return stat.Bavail * uint64(blockSize), nil
}
