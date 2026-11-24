package storage

import (
	"io/fs"
	"os"
	"runtime"
	"syscall"
)

func fileStampIdentity(file *os.File, _ fs.FileInfo) (uint64, uint64, int64, error) {
	var info syscall.ByHandleFileInformation
	err := syscall.GetFileInformationByHandle(syscall.Handle(file.Fd()), &info)
	runtime.KeepAlive(file)
	return uint64(info.VolumeSerialNumber), (uint64(info.FileIndexHigh) << 32) | uint64(info.FileIndexLow), info.CreationTime.Nanoseconds(), err
}
