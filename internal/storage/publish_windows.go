package storage

import (
	"context"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	publishKernel32    = syscall.NewLazyDLL("kernel32.dll")
	reopenPublishFile  = publishKernel32.NewProc("ReOpenFile")
	publishNTDLL       = syscall.NewLazyDLL("ntdll.dll")
	renamePublishFile  = publishNTDLL.NewProc("NtSetInformationFile")
	publishStatusError = publishNTDLL.NewProc("RtlNtStatusToDosError")
)

// FILE_RENAME_INFORMATION, with ReplaceIfExists=false. The native API preserves
// the directory handle instead of resolving the destination against a pathname.
// https://learn.microsoft.com/windows-hardware/drivers/ddi/ntifs/ns-ntifs-_file_rename_information
type renameInformation struct {
	replaceIfExists byte
	directory       syscall.Handle
	nameBytes       uint32
	name            [256]uint16
}

func publishNew(ctx context.Context, root *os.Root, stage, name string, file *os.File) error {
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	encoded, err := syscall.UTF16FromString(name)
	if err != nil {
		return err
	}
	info := renameInformation{directory: syscall.Handle(dir.Fd()), nameBytes: uint32((len(encoded) - 1) * 2)}
	if len(encoded) > len(info.name) {
		return syscall.ENAMETOOLONG
	}
	copy(info.name[:], encoded)
	// Reopen the exact staged file with DELETE access before closing the writer.
	// The destination stays bound to the opened directory, even if it is moved.
	const deleteAccess = 0x00010000
	handle, _, err := reopenPublishFile.Call(file.Fd(), deleteAccess,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, 0)
	runtime.KeepAlive(file)
	if syscall.Handle(handle) == syscall.InvalidHandle {
		return err
	}
	defer syscall.CloseHandle(syscall.Handle(handle))
	if err := file.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	const fileRenameInformation = 10
	// IO_STATUS_BLOCK contains a status/pointer union followed by ULONG_PTR.
	var statusBlock [2]uintptr
	status, _, _ := renamePublishFile.Call(handle, uintptr(unsafe.Pointer(&statusBlock)),
		uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), fileRenameInformation)
	runtime.KeepAlive(dir)
	if int32(status) < 0 {
		code, _, _ := publishStatusError.Call(status)
		return &os.LinkError{Op: "publish", Old: stage, New: name, Err: syscall.Errno(code)}
	}
	return nil
}
