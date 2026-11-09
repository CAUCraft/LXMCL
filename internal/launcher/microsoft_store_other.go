//go:build !windows

package launcher

import "errors"

func windowsCredentials([]byte, bool) ([]byte, error) {
	return nil, errors.New("Windows 凭据保护仅在 Windows 可用")
}
