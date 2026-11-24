//go:build !linux && !windows

package launcher

import "errors"

func availableDiskSpace(_ string) (uint64, error) {
	return 0, errors.ErrUnsupported
}
