package shortcut

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"unsafe"
)

// IShellLinkW supplies the target fields and IPersistFile saves the native .lnk.
// https://learn.microsoft.com/windows/win32/shell/links
var (
	system32       = filepath.Join(os.Getenv("SystemRoot"), "System32")
	ole32          = syscall.NewLazyDLL(filepath.Join(system32, "ole32.dll"))
	coInitialize   = ole32.NewProc("CoInitializeEx")
	coUninitialize = ole32.NewProc("CoUninitialize")
	coCreate       = ole32.NewProc("CoCreateInstance")
)

type comObject struct{ vtable *[21]uintptr }

func (o *comObject) release() {
	syscall.SyscallN(o.vtable[2], uintptr(unsafe.Pointer(o)))
}

func (o *comObject) setString(method int, value string) error {
	p, err := syscall.UTF16PtrFromString(value)
	if err != nil {
		return err
	}
	hr, _, _ := syscall.SyscallN(o.vtable[method], uintptr(unsafe.Pointer(o)), uintptr(unsafe.Pointer(p)))
	return checkResult(hr)
}

func checkResult(hr uintptr) error {
	if int32(hr) < 0 {
		return fmt.Errorf("Windows Shell 错误 0x%08X", uint32(hr))
	}
	return nil
}

func writeLink(out io.Writer, executable, profile string, id int) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const apartmentThreaded = 2
	hr, _, _ := coInitialize.Call(0, apartmentThreaded)
	if err := checkResult(hr); err != nil {
		return err
	}
	defer coUninitialize.Call()

	guid := func(data1 uint32) syscall.GUID {
		return syscall.GUID{Data1: data1, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	}
	class, shellLink, persistFile := guid(0x00021401), guid(0x000214f9), guid(0x0000010b)
	var link, persist *comObject
	const inProcServer = 1
	hr, _, _ = coCreate.Call(uintptr(unsafe.Pointer(&class)), 0, inProcServer, uintptr(unsafe.Pointer(&shellLink)), uintptr(unsafe.Pointer(&link)))
	if err := checkResult(hr); err != nil {
		return err
	}
	defer link.release()
	const setPath, setWorkingDirectory, setArguments = 20, 9, 11
	if err := link.setString(setPath, executable); err != nil {
		return err
	}
	if err := link.setString(setWorkingDirectory, filepath.Dir(executable)); err != nil {
		return err
	}
	if err := link.setString(setArguments, "--data-dir "+syscall.EscapeArg(profile)+" --launch "+strconv.Itoa(id)); err != nil {
		return err
	}

	hr, _, _ = syscall.SyscallN(link.vtable[0], uintptr(unsafe.Pointer(link)), uintptr(unsafe.Pointer(&persistFile)), uintptr(unsafe.Pointer(&persist)))
	if err := checkResult(hr); err != nil {
		return err
	}
	defer persist.release()
	dir, err := os.MkdirTemp("", "lxmcl-shortcut-")
	if err != nil {
		return err
	}
	defer os.Remove(dir)
	path := filepath.Join(dir, "instance.lnk")
	defer os.Remove(path)
	filename, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	const persistSave = 6 // IPersistFile::Save
	hr, _, _ = syscall.SyscallN(persist.vtable[persistSave], uintptr(unsafe.Pointer(persist)), uintptr(unsafe.Pointer(filename)), 1)
	if err := checkResult(hr); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	n, err := io.Copy(out, file)
	if err == nil && n == 0 {
		return io.ErrUnexpectedEOF
	}
	return err
}
