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

// IShellLinkW supplies the target fields. IPersistStream serializes the native
// link in memory so storage.PublishNew can commit it without overwriting a file.
// https://learn.microsoft.com/windows/win32/shell/links
var (
	system32       = filepath.Join(os.Getenv("SystemRoot"), "System32")
	ole32          = syscall.NewLazyDLL(filepath.Join(system32, "ole32.dll"))
	shlwapi        = syscall.NewLazyDLL(filepath.Join(system32, "shlwapi.dll"))
	coInitialize   = ole32.NewProc("CoInitializeEx")
	coUninitialize = ole32.NewProc("CoUninitialize")
	coCreate       = ole32.NewProc("CoCreateInstance")
	createStream   = ole32.NewProc("CreateStreamOnHGlobal")
	streamReset    = shlwapi.NewProc("IStream_Reset")
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
	class, shellLink, persistStream := guid(0x00021401), guid(0x000214f9), guid(0x00000109)
	var link, persist, stream *comObject
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

	hr, _, _ = syscall.SyscallN(link.vtable[0], uintptr(unsafe.Pointer(link)), uintptr(unsafe.Pointer(&persistStream)), uintptr(unsafe.Pointer(&persist)))
	if err := checkResult(hr); err != nil {
		return err
	}
	defer persist.release()
	hr, _, _ = createStream.Call(0, 1, uintptr(unsafe.Pointer(&stream)))
	if err := checkResult(hr); err != nil {
		return err
	}
	defer stream.release()
	const persistSave = 6
	hr, _, _ = syscall.SyscallN(persist.vtable[persistSave], uintptr(unsafe.Pointer(persist)), uintptr(unsafe.Pointer(stream)), 1)
	if err := checkResult(hr); err != nil {
		return err
	}
	hr, _, _ = streamReset.Call(uintptr(unsafe.Pointer(stream)))
	if err := checkResult(hr); err != nil {
		return err
	}
	const read = 3 // ISequentialStream::Read
	var buf [4096]byte
	for {
		var n uint32
		hr, _, _ = syscall.SyscallN(stream.vtable[read], uintptr(unsafe.Pointer(stream)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&n)))
		if err := checkResult(hr); err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		if _, err := out.Write(buf[:n]); err != nil {
			return err
		}
	}
}
