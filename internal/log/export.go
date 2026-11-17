package log

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/storage"
)

// ExportPreview saves the captured preview without replacing its source log.
// The caller obtains the destination through a native save dialog.
func ExportPreview(source, destination, preview string) error {
	if !filepath.IsAbs(destination) {
		return errors.New("请选择完整的保存路径")
	}
	if strings.EqualFold(filepath.Clean(source), filepath.Clean(destination)) || IsGame(destination) {
		return errors.New("请选择新的文本文件，不能覆盖游戏日志")
	}
	if src, err := os.Stat(source); err == nil {
		if dst, err := os.Stat(destination); err == nil && os.SameFile(src, dst) {
			return errors.New("请选择新的文本文件，不能覆盖游戏日志")
		}
	}
	return storage.WriteFile(destination, []byte(preview))
}
