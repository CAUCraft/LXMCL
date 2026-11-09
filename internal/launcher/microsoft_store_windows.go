package launcher

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

type credentialBlob struct {
	size uint32
	data *byte
}

var (
	crypt32             = syscall.NewLazyDLL(filepath.Join(os.Getenv("SystemRoot"), "System32", "crypt32.dll"))
	protectCredential   = crypt32.NewProc("CryptProtectData")
	unprotectCredential = crypt32.NewProc("CryptUnprotectData")
	freeCredential      = syscall.NewLazyDLL("kernel32.dll").NewProc("LocalFree")
)

func windowsCredentials(input []byte, protect bool) ([]byte, error) {
	if len(input) == 0 || len(input) > maxVaultSize {
		return nil, errors.New("Microsoft 凭据大小无效")
	}
	in, out := credentialBlob{uint32(len(input)), &input[0]}, credentialBlob{}
	proc := unprotectCredential
	if protect {
		proc = protectCredential
	}
	if err := proc.Find(); err != nil {
		return nil, err
	}
	const noUI = 1 // CRYPTPROTECT_UI_FORBIDDEN; default scope is the current Windows user.
	ok, _, err := proc.Call(uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, noUI, uintptr(unsafe.Pointer(&out)))
	runtime.KeepAlive(input)
	if ok == 0 {
		return nil, err
	}
	defer freeCredential.Call(uintptr(unsafe.Pointer(out.data)))
	if out.data == nil || out.size == 0 || out.size > maxVaultSize {
		return nil, errors.New("Windows 返回了无效的凭据数据")
	}
	data := unsafe.Slice(out.data, int(out.size))
	result := append([]byte(nil), data...)
	if !protect {
		clear(data)
	}
	return result, nil
}
