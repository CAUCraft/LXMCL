package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var getDiskFreeSpaceEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

func availableDiskSpace(directory string) (uint64, error) {
	name := filepath.Clean(directory)
	const (
		extended    = "\\\\?\\"
		device      = "\\\\.\\"
		unc         = "\\\\"
		extendedUNC = extended + "UNC\\"
	)
	// Preserve ordinary path semantics except where the API needs a long path.
	isUNC := strings.HasPrefix(name, unc) && !strings.HasPrefix(name, extended) && !strings.HasPrefix(name, device)
	switch {
	case len(name) < 248, strings.HasPrefix(name, extended), strings.HasPrefix(name, device):
	case isUNC:
		name = extendedUNC + strings.TrimPrefix(name, unc)
	default:
		name = extended + name
	}
	// The API requires a trailing separator for UNC paths.
	if (isUNC || len(name) >= len(extendedUNC) && strings.EqualFold(name[:len(extendedUNC)], extendedUNC)) && !strings.HasSuffix(name, "\\") {
		name += "\\"
	}
	path, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	var available uint64
	result, _, err := getDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(path)), uintptr(unsafe.Pointer(&available)), 0, 0)
	if result == 0 {
		return 0, &os.PathError{Op: "GetDiskFreeSpaceEx", Path: directory, Err: err}
	}
	return available, nil
}
