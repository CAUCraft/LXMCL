//go:build !windows

package launcher

import "github.com/CAUCraft/LXMCL/internal/i18n"

func windowsCredentials([]byte, bool) ([]byte, error) {
	return nil, i18n.Error("credentials.windows_only")
}
