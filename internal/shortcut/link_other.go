//go:build !windows

package shortcut

import (
	"errors"
	"io"
)

func writeLink(io.Writer, string, string, int) error {
	return errors.New("创建启动快捷方式需要 Windows")
}
